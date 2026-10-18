/**
 * The orchestration order of `useSendMessage`.
 *
 * The network is mocked at the module boundary (`@/shared/llm`,
 * `@/entities/search-result`) so this test can assert the sequence the store
 * performs and what it hands each collaborator. The prompt content itself is
 * covered by `prompt.test.ts`; here we check that the right inputs reach it.
 */
import { beforeEach, describe, expect, it, vi } from "vitest"
import { createPinia, setActivePinia } from "pinia"

const mocks = vi.hoisted(() => {
  const state = {
    note: null as null | {
      chat: {
        path: string
        title: string
        prompt: string
        settings: {
          model: string | null
          temperature: number | null
          topK: number | null
        }
      }
      turns: Array<{ role: "user" | "assistant"; content: string; sources: string[] }>
    },
    calls: [] as string[],
  }

  const appendTurn = vi.fn(
    async (turn: { role: "user" | "assistant"; content: string; sources: string[] }) => {
      state.calls.push(`append:${turn.role}`)
      state.note?.turns.push(turn)
    },
  )

  return {
    state,
    appendTurn,
    searchCorpus: vi.fn(),
    streamChatCompletion: vi.fn(),
    getSettings: vi.fn(),
  }
})

vi.mock("@/shared/llm", () => ({
  streamChatCompletion: mocks.streamChatCompletion,
}))

vi.mock("@/shared/config", () => ({
  getSettings: mocks.getSettings,
}))

vi.mock("@/entities/search-result", () => ({
  searchCorpus: mocks.searchCorpus,
}))

vi.mock("@/entities/chat", () => ({
  useChatsStore: () => ({
    note: mocks.state.note,
    appendTurn: mocks.appendTurn,
  }),
}))

import { INTERRUPTED_MARKER, useSendMessage } from "./useSendMessage"

const SEARCH_RESULT = {
  chunk_id: "c1",
  document_id: "d1",
  document_title: "Domain-Driven Design",
  page_number: 42,
  snippet: "[El] modelo del [dominio]",
  text: "El modelo del dominio es la raíz del lenguaje ubicuo.",
  stage: "hybrid" as const,
  bm25: 1,
  similarity: 0.5,
}

function settings(overrides: Record<string, unknown> = {}) {
  return {
    apiBase: "http://localhost:8080",
    llmBaseUrl: "https://openrouter.ai/api/v1",
    llmApiKey: "sk-test",
    llmModel: "global-model",
    llmTemperature: 0.2,
    retrievalTopK: 6,
    historyTurns: 8,
    defaultChatPrompt: "",
    ...overrides,
  }
}

function makeNote(overrides: Partial<NonNullable<typeof mocks.state.note>> = {}) {
  return {
    chat: {
      path: "Study Dashboard/Chats/test.md",
      title: "test",
      prompt: "Sos un tutor.",
      settings: { model: null, temperature: null, topK: null },
    },
    turns: [] as NonNullable<typeof mocks.state.note>["turns"],
    ...overrides,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  mocks.state.calls.length = 0
  mocks.state.note = makeNote()
  mocks.getSettings.mockReturnValue(settings())
  mocks.searchCorpus.mockImplementation(async () => {
    mocks.state.calls.push("search")
    return [SEARCH_RESULT]
  })
  mocks.streamChatCompletion.mockImplementation(
    async (req: { onDelta?: (delta: string) => void }) => {
      mocks.state.calls.push("stream")
      req.onDelta?.("hola ")
      req.onDelta?.("mundo")
      return { content: "hola mundo", deltas: 2 }
    },
  )
})

describe("useSendMessage.send", () => {
  it("ignores a blank question without touching anything", async () => {
    const store = useSendMessage()

    await store.send("   ")

    expect(mocks.state.calls).toEqual([])
    expect(mocks.getSettings).not.toHaveBeenCalled()
    expect(store.error).toBeNull()
    expect(store.sending).toBe(false)
  })

  it("reports a missing API key in Spanish and calls nothing", async () => {
    mocks.getSettings.mockReturnValue(settings({ llmApiKey: "" }))
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(store.error).toContain("API key")
    expect(store.error).toContain("ajustes")
    expect(mocks.state.calls).toEqual([])
    expect(mocks.searchCorpus).not.toHaveBeenCalled()
    expect(store.sending).toBe(false)
  })

  it("reports a missing open chat instead of streaming into nowhere", async () => {
    mocks.state.note = null
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(store.error).toContain("chat abierto")
    expect(mocks.state.calls).toEqual([])
  })

  it("persists the question, streams, then appends the answer without touching the corpus", async () => {
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.state.calls).toEqual([
      "append:user",
      "stream",
      "append:assistant",
    ])
    expect(mocks.searchCorpus).not.toHaveBeenCalled()
    expect(store.error).toBeNull()
    expect(store.sending).toBe(false)
  })

  it("answers with the corpus down: a chat without material never queries it", async () => {
    // The corpus is unreachable on purpose. A chat without attachments must not
    // depend on the backend at all, so the failure has to be unreachable too.
    mocks.searchCorpus.mockImplementation(async () => {
      mocks.state.calls.push("search")
      throw new Error("El corpus no responde.")
    })
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.state.calls).toEqual(["append:user", "stream", "append:assistant"])
    expect(store.error).toBeNull()
    expect(store.sending).toBe(false)
  })

  it("does not append an assistant turn when the stream fails", async () => {
    mocks.streamChatCompletion.mockImplementation(async () => {
      mocks.state.calls.push("stream")
      throw new Error("El proveedor falló (HTTP 500).")
    })
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.state.calls).toEqual(["append:user", "stream"])
    expect(store.error).toContain("HTTP 500")
    expect(store.sending).toBe(false)
  })

  it("accumulates the streamed deltas in answer and appends the turn with no sources", async () => {
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(store.answer).toBe("hola mundo")

    const appended = mocks.appendTurn.mock.calls.at(-1)?.[0]

    expect(appended).toEqual({
      role: "assistant",
      content: "hola mundo",
      sources: [],
    })
  })

  it("sends the bare question with the no-material frame and no context block", async () => {
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    const request = mocks.streamChatCompletion.mock.calls[0][0] as {
      messages: Array<{ role: string; content: string }>
    }

    expect(request.messages.at(-1)?.content).toBe("¿Qué es el lenguaje ubicuo?")
    expect(request.messages[0]?.content).toContain("no tiene material adjunto")
  })

  it("excludes the question just appended from the history", async () => {
    mocks.state.note = makeNote({
      turns: [
        { role: "assistant", content: "respuesta anterior", sources: ["Refactoring, p. 7"] },
        { role: "user", content: "pregunta anterior", sources: [] },
      ],
    })
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    const request = mocks.streamChatCompletion.mock.calls[0][0] as {
      messages: Array<{ role: string; content: string }>
    }
    const userMessages = request.messages.filter((message) => message.role === "user")

    expect(userMessages.map((message) => message.content)).toEqual([
      "pregunta anterior",
      expect.stringContaining("¿Qué es el lenguaje ubicuo?"),
    ])
  })

  it("uses the chat's model and temperature overrides when set", async () => {
    mocks.state.note = makeNote({
      chat: {
        ...makeNote().chat,
        settings: { model: "chat-model", temperature: 0.9, topK: null },
      },
    })
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    const request = mocks.streamChatCompletion.mock.calls[0][0] as {
      model: string
      temperature: number
    }

    expect(request.model).toBe("chat-model")
    expect(request.temperature).toBe(0.9)
  })

  it("falls back to the global model and temperature when the chat does not override", async () => {
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    const request = mocks.streamChatCompletion.mock.calls[0][0] as {
      model: string
      temperature: number
    }

    expect(request.model).toBe("global-model")
    expect(request.temperature).toBe(0.2)
  })

  it("clears a previous answer when a new send starts", async () => {
    const store = useSendMessage()

    await store.send("primera")
    store.answer = "residuo"

    await store.send("segunda")

    expect(store.answer).toBe("hola mundo")
  })

  it("ignores a second question while the first one is still in flight", async () => {
    // Both streams wait on the same gate, so without a reentrancy guard the two
    // sends would interleave their appends and their deltas.
    let openGate: () => void = () => {}
    const gate = new Promise<void>((resolve) => {
      openGate = resolve
    })

    mocks.streamChatCompletion.mockImplementation(
      async (req: { onDelta?: (delta: string) => void }) => {
        mocks.state.calls.push("stream")
        await gate
        req.onDelta?.("hola")
        return { content: "hola", deltas: 1 }
      },
    )
    const store = useSendMessage()

    const first = store.send("primera")

    await vi.waitFor(() => {
      expect(mocks.state.calls).toContain("stream")
    })

    const second = store.send("segunda")

    openGate()
    await Promise.all([first, second])

    expect(mocks.state.calls).toEqual([
      "append:user",
      "stream",
      "append:assistant",
    ])
    expect(store.answer).toBe("hola")
  })

  it("keeps a partial answer, marked as interrupted, when the stream fails", async () => {
    mocks.streamChatCompletion.mockImplementation(
      async (req: { onDelta?: (delta: string) => void }) => {
        mocks.state.calls.push("stream")
        req.onDelta?.("Un agregado es")
        throw new Error("el stream se cortó")
      },
    )
    const store = useSendMessage()

    await store.send("¿Qué es el agregado?")

    expect(store.error).toContain("el stream se cortó")
    expect(store.answer).toContain("Un agregado es")
    expect(mocks.state.note?.turns.at(-1)).toEqual({
      role: "assistant",
      content: `Un agregado es\n\n${INTERRUPTED_MARKER}`,
      sources: [],
    })
  })

  it("saves no turn when the stream failed before a single delta", async () => {
    mocks.streamChatCompletion.mockImplementation(async () => {
      mocks.state.calls.push("stream")
      throw new Error("sin red")
    })
    const store = useSendMessage()

    await store.send("¿Qué es el agregado?")

    expect(mocks.state.calls).not.toContain("append:assistant")
    expect(store.error).toContain("sin red")
  })
})

describe("useSendMessage.cancel", () => {
  it("aborts the stream in flight without reporting an error", async () => {
    mocks.streamChatCompletion.mockImplementation(
      (req: { signal?: AbortSignal }) => {
        mocks.state.calls.push("stream")

        return new Promise((_resolve, reject) => {
          req.signal?.addEventListener("abort", () => {
            const error = new Error("La respuesta se canceló.")

            error.name = "AbortError"
            reject(error)
          })
        })
      },
    )
    const store = useSendMessage()

    const pending = store.send("¿Qué es el lenguaje ubicuo?")

    await vi.waitFor(() => {
      expect(mocks.state.calls).toContain("stream")
    })

    store.cancel()
    await pending

    expect(store.error).toBeNull()
    expect(store.sending).toBe(false)
    expect(mocks.state.calls).not.toContain("append:assistant")
  })

  it("saves what arrived before the cancel, marked as interrupted", async () => {
    mocks.streamChatCompletion.mockImplementation(
      (req: { signal?: AbortSignal; onDelta?: (delta: string) => void }) => {
        mocks.state.calls.push("stream")
        req.onDelta?.("Un agregado")

        return new Promise((_resolve, reject) => {
          req.signal?.addEventListener("abort", () => {
            const error = new Error("La respuesta se canceló.")

            error.name = "AbortError"
            reject(error)
          })
        })
      },
    )
    const store = useSendMessage()

    const pending = store.send("¿Qué es el agregado?")

    await vi.waitFor(() => {
      expect(store.answer).toContain("Un agregado")
    })

    store.cancel()
    await pending

    expect(store.error).toBeNull()
    expect(mocks.state.note?.turns.at(-1)).toEqual({
      role: "assistant",
      content: `Un agregado\n\n${INTERRUPTED_MARKER}`,
      sources: [],
    })
  })

  it("is a no-op when nothing is in flight", () => {
    const store = useSendMessage()

    expect(() => store.cancel()).not.toThrow()
  })
})

describe("useSendMessage.clearError", () => {
  it("clears the error", async () => {
    mocks.getSettings.mockReturnValue(settings({ llmApiKey: "" }))
    const store = useSendMessage()

    await store.send("una pregunta")
    store.clearError()

    expect(store.error).toBeNull()
  })
})
