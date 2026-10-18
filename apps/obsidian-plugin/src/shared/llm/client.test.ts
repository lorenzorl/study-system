import { describe, expect, it } from "vitest"
import { streamChatCompletion, type ChatRequest } from "./client"
import { LlmError, describeLlmError } from "./errors"

const API_KEY = "sk-test-secret-key-123"
const BASE_URL = "https://openrouter.ai/api/v1"
const MODEL = "openai/gpt-4o-mini"

/** Encode SSE text chunks as a `ReadableStream`, as a network body would arrive. */
function encodeChunks(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder()

  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk))
      }
      controller.close()
    },
  })
}

function sseResponse(chunks: string[], status = 200): Response {
  return new Response(encodeChunks(chunks), {
    status,
    headers: { "Content-Type": "text/event-stream" },
  })
}

function jsonErrorResponse(status: number, message: string): Response {
  return new Response(JSON.stringify({ error: { message } }), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

interface CapturedRequest {
  url: string
  init?: RequestInit
}

/** A fake `fetch` that returns one response and records the request it received. */
function captureFetch(response: Response): {
  fetchImpl: typeof fetch
  captured: CapturedRequest
} {
  const captured: CapturedRequest = { url: "" }

  const fetchImpl = (async (input: RequestInfo | URL, init?: RequestInit) => {
    captured.url = typeof input === "string" ? input : input.toString()
    captured.init = init
    return response
  }) as typeof fetch

  return { fetchImpl, captured }
}

function baseRequest(overrides: Partial<ChatRequest> = {}): ChatRequest {
  return {
    apiKey: API_KEY,
    baseUrl: BASE_URL,
    model: MODEL,
    messages: [
      { role: "system", content: "Sos un tutor." },
      { role: "user", content: "¿Qué es una derivada?" },
    ],
    ...overrides,
  }
}

async function captureFailure(operation: Promise<unknown>): Promise<unknown> {
  try {
    await operation
  } catch (error) {
    return error
  }

  throw new Error("expected the operation to reject")
}

describe("streamChatCompletion transport", () => {
  it("sends the key only in the Authorization header", async () => {
    const { fetchImpl, captured } = captureFetch(sseResponse(["data: [DONE]\n\n"]))

    await streamChatCompletion(baseRequest(), fetchImpl)

    const headers = new Headers(captured.init?.headers)
    expect(headers.get("Authorization")).toBe(`Bearer ${API_KEY}`)
    expect(headers.get("Content-Type")).toContain("application/json")
    expect(captured.url).not.toContain(API_KEY)
    expect(String(captured.init?.body ?? "")).not.toContain(API_KEY)
  })

  it("POSTs the model, the messages and stream:true to /chat/completions", async () => {
    const { fetchImpl, captured } = captureFetch(sseResponse(["data: [DONE]\n\n"]))

    await streamChatCompletion(baseRequest({ baseUrl: `${BASE_URL}///` }), fetchImpl)

    expect(captured.url).toBe(`${BASE_URL}/chat/completions`)
    expect(captured.init?.method).toBe("POST")

    const body = JSON.parse(String(captured.init?.body))
    expect(body.stream).toBe(true)
    expect(body.model).toBe(MODEL)
    expect(body.messages).toEqual([
      { role: "system", content: "Sos un tutor." },
      { role: "user", content: "¿Qué es una derivada?" },
    ])
    expect("temperature" in body).toBe(false)
  })

  it("includes the temperature when the request sets one", async () => {
    const { fetchImpl, captured } = captureFetch(sseResponse(["data: [DONE]\n\n"]))

    await streamChatCompletion(baseRequest({ temperature: 0.2 }), fetchImpl)

    expect(JSON.parse(String(captured.init?.body)).temperature).toBe(0.2)
  })

  it("delivers deltas in order and concatenates them into the content", async () => {
    const { fetchImpl } = captureFetch(
      sseResponse([
        'data: {"choices":[{"delta":{"content":"Las "}}]}\n\n',
        'data: {"choices":[{"delta":{"content":"derivadas "}}]}\n',
        '\ndata: {"choices":[{"delta":{"content":"miden cambios."}}]}\n\n',
        "data: [DONE]\n\n",
      ]),
    )
    const received: string[] = []

    const result = await streamChatCompletion(
      baseRequest({ onDelta: (text) => received.push(text) }),
      fetchImpl,
    )

    expect(received).toEqual(["Las ", "derivadas ", "miden cambios."])
    expect(result.content).toBe("Las derivadas miden cambios.")
    expect(result.deltas).toBe(3)
  })

  it("keeps a delta whose payload is split across two network chunks", async () => {
    const { fetchImpl } = captureFetch(
      sseResponse(['data: {"choices":[{"delta":{"cont', 'ent":"hola"}}]}\n\n', "data: [DONE]\n\n"]),
    )

    const result = await streamChatCompletion(baseRequest(), fetchImpl)

    expect(result.content).toBe("hola")
    expect(result.deltas).toBe(1)
  })

  it("treats a final usage-only chunk as no delta and not an error", async () => {
    const { fetchImpl } = captureFetch(
      sseResponse([
        'data: {"choices":[{"delta":{"role":"assistant","content":""}}]}\n\n',
        'data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"total_tokens":5}}\n\n',
        "data: [DONE]\n\n",
      ]),
    )

    const result = await streamChatCompletion(baseRequest(), fetchImpl)

    expect(result.content).toBe("")
    expect(result.deltas).toBe(0)
  })
})

describe("streamChatCompletion error mapping", () => {
  it("throws an LlmError with the provider status and no key on a 401", async () => {
    const { fetchImpl } = captureFetch(jsonErrorResponse(401, "No auth credentials found"))

    const error = await captureFailure(streamChatCompletion(baseRequest(), fetchImpl))

    expect(error).toBeInstanceOf(LlmError)
    expect((error as LlmError).status).toBe(401)
    expect((error as Error).message).toContain("API key")
    expect((error as Error).message).not.toContain(API_KEY)
  })

  it("gives a 429 and a 500 their own messages", async () => {
    const { fetchImpl: tooMany } = captureFetch(jsonErrorResponse(429, "Rate limit exceeded"))
    const { fetchImpl: broken } = captureFetch(jsonErrorResponse(500, "Internal error"))

    const error429 = await captureFailure(streamChatCompletion(baseRequest(), tooMany))
    const error500 = await captureFailure(streamChatCompletion(baseRequest(), broken))

    expect((error429 as LlmError).status).toBe(429)
    expect((error429 as Error).message).toContain("demasiadas solicitudes")
    expect((error500 as LlmError).status).toBe(500)
    expect((error500 as Error).message).toContain("proveedor falló")
    expect((error429 as Error).message).not.toBe((error500 as Error).message)
  })

  it("throws an LlmError with status 0 when fetch rejects", async () => {
    const fetchImpl = (async () => {
      throw new Error("network down")
    }) as typeof fetch

    const error = await captureFailure(streamChatCompletion(baseRequest(), fetchImpl))

    expect(error).toBeInstanceOf(LlmError)
    expect((error as LlmError).status).toBe(0)
    expect((error as Error).message.toLowerCase()).toContain("proveedor")
    expect((error as Error).message).not.toContain(API_KEY)
  })

  it("rejects an already-aborted signal with a distinguishable cancellation error", async () => {
    const controller = new AbortController()
    controller.abort()
    const { fetchImpl } = captureFetch(sseResponse(["data: [DONE]\n\n"]))

    const error = await captureFailure(
      streamChatCompletion(baseRequest({ signal: controller.signal }), fetchImpl),
    )

    expect(error).toBeInstanceOf(LlmError)
    expect((error as LlmError).status).toBe(0)
    expect((error as Error).name).toBe("AbortError")
    expect((error as Error).message.toLowerCase()).toContain("cancel")
  })

  it("throws when the provider sends no stream", async () => {
    const { fetchImpl } = captureFetch(new Response(null, { status: 200 }))

    const error = await captureFailure(streamChatCompletion(baseRequest(), fetchImpl))

    expect(error).toBeInstanceOf(LlmError)
    expect((error as Error).message.toLowerCase()).toContain("stream")
  })

  it("throws on a malformed stream payload instead of skipping it", async () => {
    const { fetchImpl } = captureFetch(
      sseResponse(['data: {"choices":[\n\n', "data: [DONE]\n\n"]),
    )

    const error = await captureFailure(streamChatCompletion(baseRequest(), fetchImpl))

    expect(error).toBeInstanceOf(LlmError)
    expect((error as Error).message).toMatch(/malformad/i)
  })
})

describe("describeLlmError", () => {
  it("distinguishes the provider statuses with Spanish messages", () => {
    expect(describeLlmError(401, "")).toContain("API key")
    expect(describeLlmError(402, "")).toContain("crédito")
    expect(describeLlmError(403, "")).toContain("permiso")
    expect(describeLlmError(404, "")).toContain("modelo")
    expect(describeLlmError(429, "")).toContain("demasiadas solicitudes")
    expect(describeLlmError(503, "")).toContain("proveedor falló")
    expect(describeLlmError(418, "")).toContain("418")
  })

  it("appends the provider message truncated to one line", () => {
    const body = JSON.stringify({
      error: { message: "Model not found\nSecond line of detail" },
    })

    const message = describeLlmError(404, body)

    expect(message).toContain("Model not found")
    expect(message).not.toContain("\n")
  })

  it("returns the plain status message when the body carries no provider message", () => {
    expect(describeLlmError(500, "not json")).toBe("El proveedor falló (HTTP 500).")
  })
})
