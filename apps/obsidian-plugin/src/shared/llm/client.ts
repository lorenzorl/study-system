/**
 * Streaming OpenRouter chat-completions client.
 *
 * Pure transport: it knows nothing about chats, prompts or the vault. The
 * caller supplies the credential, the base URL and the messages, and receives
 * the streamed text. `fetch` is injected so tests never touch the network.
 */
import { LlmError, describeLlmError } from "./errors"
import { extractDelta, parseSseChunk } from "./sse"

export interface LlmMessage {
  role: "system" | "user" | "assistant"
  content: string
}

export interface ChatRequest {
  apiKey: string
  baseUrl: string
  model: string
  messages: LlmMessage[]
  temperature?: number
  signal?: AbortSignal
  onDelta?: (text: string) => void
}

export interface ChatResult {
  content: string
  deltas: number
}

/** The SSE payload that closes the stream; it contributes nothing. */
const DONE_PAYLOAD = "[DONE]"

/** How much of a non-2xx body is read before mapping the error. */
const MAX_ERROR_BODY = 4000

const UNREACHABLE_MESSAGE =
  "No se pudo alcanzar al proveedor del modelo. Revisá tu conexión e intentá de nuevo."
const CANCELLED_MESSAGE = "La respuesta se canceló."
const EMPTY_STREAM_MESSAGE = "El proveedor no envió un stream de respuesta."

/** Internal signal that the stream reached `[DONE]`; never escapes this module. */
class StreamClosed extends Error {}

function abortError(): LlmError {
  const error = new LlmError(CANCELLED_MESSAGE, 0)

  // Distinguishable by callers that test `error.name === "AbortError"`.
  error.name = "AbortError"

  return error
}

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError"
}

function trimTrailingSlashes(baseUrl: string): string {
  return baseUrl.trim().replace(/\/+$/, "")
}

async function cancelReader(reader: ReadableStreamDefaultReader<Uint8Array>): Promise<void> {
  try {
    await reader.cancel()
  } catch {
    // The stream is already finished, released or errored; nothing to cancel.
  }
}

/**
 * POST one streaming chat completion.
 *
 * The key travels only in the `Authorization` header. Non-2xx responses become
 * an `LlmError` carrying the provider status; a transport failure or an abort
 * becomes an `LlmError` with `status: 0`.
 */
export async function streamChatCompletion(
  req: ChatRequest,
  fetchImpl: typeof fetch = fetch,
): Promise<ChatResult> {
  if (req.signal?.aborted) throw abortError()

  const response = await requestCompletion(req, fetchImpl)

  if (!response.ok) {
    const body = await readBoundedBody(response)

    throw new LlmError(describeLlmError(response.status, body), response.status)
  }

  if (response.body === null) {
    throw new LlmError(EMPTY_STREAM_MESSAGE, response.status)
  }

  return readStream(response.body, req.onDelta)
}

async function requestCompletion(
  req: ChatRequest,
  fetchImpl: typeof fetch,
): Promise<Response> {
  const url = `${trimTrailingSlashes(req.baseUrl)}/chat/completions`
  const payload: Record<string, unknown> = {
    model: req.model,
    messages: req.messages,
    stream: true,
  }

  if (req.temperature !== undefined) payload.temperature = req.temperature

  try {
    return await fetchImpl(url, {
      method: "POST",
      headers: {
        // The only place the key appears.
        Authorization: `Bearer ${req.apiKey}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
      signal: req.signal,
    })
  } catch (error) {
    if (req.signal?.aborted || isAbortError(error)) throw abortError()

    throw new LlmError(UNREACHABLE_MESSAGE, 0)
  }
}

/** Read at most `limit` characters of an error body, never the whole payload. */
async function readBoundedBody(response: Response, limit = MAX_ERROR_BODY): Promise<string> {
  if (response.body === null) {
    try {
      return (await response.text()).slice(0, limit)
    } catch {
      return ""
    }
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let text = ""

  try {
    while (text.length < limit) {
      const { value, done } = await reader.read()

      if (done) break

      text += decoder.decode(value, { stream: true })
    }

    text += decoder.decode()
  } catch {
    // A body that fails mid-read still maps through the status message.
  } finally {
    await cancelReader(reader)
  }

  return text.slice(0, limit)
}

async function readStream(
  body: ReadableStream<Uint8Array>,
  onDelta?: (text: string) => void,
): Promise<ChatResult> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ""
  let content = ""
  let deltas = 0
  let closed = false

  const handlePayload = (payload: string): void => {
    if (payload === DONE_PAYLOAD) throw new StreamClosed()

    const delta = extractDelta(payload)

    if (delta === "") return

    content += delta
    deltas += 1
    onDelta?.(delta)
  }

  try {
    while (true) {
      const { value, done } = await reader.read()

      if (done) break

      buffer = parseSseChunk(buffer + decoder.decode(value, { stream: true }), handlePayload)
    }
  } catch (error) {
    if (!(error instanceof StreamClosed)) throw error

    closed = true
  } finally {
    await cancelReader(reader)
  }

  // A stream that ended without `[DONE]` may still hold a final line without a
  // trailing newline; flushing it avoids losing the last delta.
  if (!closed) {
    buffer += decoder.decode()

    if (buffer !== "") {
      try {
        parseSseChunk(`${buffer}\n`, handlePayload)
      } catch (error) {
        if (!(error instanceof StreamClosed)) throw error
      }
    }
  }

  return { content, deltas }
}
