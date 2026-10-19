/**
 * Streaming OpenRouter chat-completions client.
 *
 * Pure transport: it knows nothing about chats, prompts or the vault. The
 * caller supplies the credential, the base URL and the messages, and receives
 * the streamed text. `fetch` is injected so tests never touch the network.
 */
import { LlmError, describeLlmError } from "./errors"
import { extractDelta, extractToolCallFragments, parseSseChunk } from "./sse"

/** A tool call the model asked for, in the shape the provider expects back. */
export interface ToolCall {
  id: string
  type: "function"
  function: {
    name: string
    /** The raw JSON string the model streamed; the caller parses it. */
    arguments: string
  }
}

/** A tool offered to the model, in the provider's wire shape. */
export interface ToolSpec {
  type: "function"
  function: {
    name: string
    description: string
    /** A JSON Schema for the tool arguments. */
    parameters: Record<string, unknown>
  }
}

export interface LlmMessage {
  role: "system" | "user" | "assistant" | "tool"
  content: string
  /** Assistant turn: the calls the model requested, echoed back on the next request. */
  tool_calls?: ToolCall[]
  /** Tool turn: the id of the call this message answers. */
  tool_call_id?: string
}

export interface ChatRequest {
  apiKey: string
  baseUrl: string
  model: string
  messages: LlmMessage[]
  /** Tools offered to the model; omitted entirely when the chat has none. */
  tools?: ToolSpec[]
  temperature?: number
  signal?: AbortSignal
  onDelta?: (text: string) => void
}

export interface ChatResult {
  content: string
  deltas: number
  /** The assembled tool calls; empty when the stream carried none. */
  toolCalls: ToolCall[]
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
  if (req.tools !== undefined) payload.tools = req.tools

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

/** A tool call being assembled from its streamed fragments. */
interface PendingToolCall {
  id: string
  name: string
  arguments: string
}

/**
 * Fold the accumulated fragments into the assembled calls, ordered by index.
 *
 * The provider opens each call with its `index`, `id` and function `name`, then
 * streams the raw JSON `arguments` in pieces that share the index; the pieces
 * are concatenated in arrival order.
 */
function assembleToolCalls(pending: Map<number, PendingToolCall>): ToolCall[] {
  return [...pending.entries()]
    .sort(([a], [b]) => a - b)
    .map(([, call]) => ({
      id: call.id,
      type: "function" as const,
      function: { name: call.name, arguments: call.arguments },
    }))
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
  const pendingToolCalls = new Map<number, PendingToolCall>()

  const handlePayload = (payload: string): void => {
    if (payload === DONE_PAYLOAD) throw new StreamClosed()

    const delta = extractDelta(payload)

    for (const fragment of extractToolCallFragments(payload)) {
      const call = pendingToolCalls.get(fragment.index) ?? { id: "", name: "", arguments: "" }

      if (fragment.id !== undefined) call.id = fragment.id
      if (fragment.name !== undefined) call.name = fragment.name
      if (fragment.arguments !== undefined) call.arguments += fragment.arguments

      pendingToolCalls.set(fragment.index, call)
    }

    // A tool-call chunk carries no content and is not an empty answer, so the
    // stream must keep reading instead of stopping at the first one.
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

  return { content, deltas, toolCalls: assembleToolCalls(pendingToolCalls) }
}
