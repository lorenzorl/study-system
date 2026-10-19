/**
 * The orchestration order of `useSendMessage`.
 *
 * The network is mocked at the module boundary (`@/shared/llm`,
 * `@/entities/search-result`, `@/entities/document`) so this test can assert the
 * sequence the store performs and what it hands each collaborator. The prompt
 * content itself is covered by `prompt.test.ts`; here we check that the right
 * inputs reach it.
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
      turns: Array<{
        role: "user" | "assistant"
        content: string
        sources: string[]
        consulted: boolean
      }>
      attachments: Array<
        { kind: "corpus"; documentId: string; title: string } | { kind: "vault"; path: string }
      >
    },
    calls: [] as string[],
    /** Set by the cases that assert what happens when the note refuses a turn. */
    writeFails: false,
  }

  const appendTurn = vi.fn(
    async (turn: {
      role: "user" | "assistant"
      content: string
      sources: string[]
      consulted: boolean
    }) => {
      if (state.writeFails) return false
      state.calls.push(`append:${turn.role}`)
      state.note?.turns.push(turn)
      return true
    },
  )

  return {
    state,
    appendTurn,
    searchCorpus: vi.fn(),
    listDocuments: vi.fn(),
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

// The preflight's probe of the corpus. Reachable by default: the pre-existing
// corpus cases are about what the model is offered, not about reachability.
vi.mock("@/entities/document", () => ({
  listDocuments: mocks.listDocuments,
}))

vi.mock("@/entities/chat", () => ({
  useChatsStore: () => ({
    note: mocks.state.note,
    appendTurn: mocks.appendTurn,
  }),
}))

import { INTERRUPTED_MARKER, useSendMessage } from "./useSendMessage"
import type { ToolCall } from "@/shared/llm"
import { NO_MATERIAL_FRAME, buildContext, buildSystemMessage } from "./prompt"
import { MAX_ATTACHMENT_CHARS, setVaultTextReader } from "./vault-text"

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

const CORPUS_ATTACHMENT = {
  kind: "corpus" as const,
  documentId: "doc-ddd",
  title: "Domain-Driven Design",
}

const VAULT_PATH = "Notas/Tesis.md"
const VAULT_TEXT = [
  "El estado es efímero y se descarta al recargar.",
  "",
  "Sin páginas, porque una nota del vault no tiene.",
].join("\n")

const VAULT_ATTACHMENT = { kind: "vault" as const, path: VAULT_PATH }

/** The vault as the send path sees it: a map a test can edit or empty. */
let vaultFiles: Record<string, string>

const SEARCH_TOOL_CALL: ToolCall = {
  id: "call-1",
  type: "function",
  function: { name: "search_material", arguments: JSON.stringify({ query: "lenguaje ubicuo" }) },
}

/** A scripted transport: one entry per request, in order. */
function scriptStream(responses: Array<{ content?: string; toolCalls?: ToolCall[] }>) {
  let index = 0

  mocks.streamChatCompletion.mockImplementation(
    async (req: { onDelta?: (delta: string) => void }) => {
      mocks.state.calls.push("stream")
      const response = responses[Math.min(index, responses.length - 1)]

      index += 1

      const content = response.content ?? ""
      if (content !== "") req.onDelta?.(content)

      return { content, deltas: content === "" ? 0 : 1, toolCalls: response.toolCalls ?? [] }
    },
  )
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
    attachments: [] as NonNullable<typeof mocks.state.note>["attachments"],
    ...overrides,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  mocks.state.calls.length = 0
  mocks.state.writeFails = false
  mocks.state.note = makeNote()
  mocks.getSettings.mockReturnValue(settings())
  vaultFiles = { [VAULT_PATH]: VAULT_TEXT }
  setVaultTextReader(async (path) => {
    const text = vaultFiles[path]

    if (text === undefined) throw new Error(`No se encontró la nota "${path}".`)

    return text
  })
  mocks.listDocuments.mockImplementation(async () => [])
  mocks.searchCorpus.mockImplementation(async () => {
    mocks.state.calls.push("search")
    return [SEARCH_RESULT]
  })
  mocks.streamChatCompletion.mockImplementation(
    async (req: { onDelta?: (delta: string) => void }) => {
      mocks.state.calls.push("stream")
      req.onDelta?.("hola ")
      req.onDelta?.("mundo")
      return { content: "hola mundo", deltas: 2, toolCalls: [] }
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
      consulted: true,
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

  it("sends today's exact request, byte for byte, when the chat has no attachments", async () => {
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    // `signal` and `onDelta` are functions and cannot be compared as bytes; they
    // ride along and are the only things left out. Everything the provider reads
    // is pinned here, including the absence of a `tools` key.
    const { signal: _signal, onDelta: _onDelta, ...payload } = mocks.streamChatCompletion.mock
      .calls[0][0] as Record<string, unknown>

    expect(JSON.stringify(payload)).toBe(
      JSON.stringify({
        apiKey: "sk-test",
        baseUrl: "https://openrouter.ai/api/v1",
        model: "global-model",
        temperature: 0.2,
        messages: [
          { role: "system", content: `${NO_MATERIAL_FRAME}\n\nSos un tutor.` },
          { role: "user", content: "¿Qué es el lenguaje ubicuo?" },
        ],
      }),
    )
  })

  it("excludes the question just appended from the history", async () => {
    mocks.state.note = makeNote({
      turns: [
        { role: "assistant", content: "respuesta anterior", sources: ["Refactoring, p. 7"], consulted: true },
        { role: "user", content: "pregunta anterior", sources: [], consulted: true },
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
        return { content: "hola", deltas: 1, toolCalls: [] }
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
      consulted: true,
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

  it("answers false for every guard that stops the send before anything is persisted", async () => {
    const store = useSendMessage()

    expect(await store.send("   ")).toBe(false)

    mocks.getSettings.mockReturnValue(settings({ llmApiKey: "" }))

    expect(await store.send("una pregunta")).toBe(false)

    mocks.getSettings.mockReturnValue(settings())
    mocks.state.note = null

    expect(await store.send("una pregunta")).toBe(false)
  })

  it("answers true when the send proceeded", async () => {
    const store = useSendMessage()

    expect(await store.send("¿Qué es el lenguaje ubicuo?")).toBe(true)
    expect(mocks.appendTurn.mock.calls.map(([turn]) => turn.role)).toEqual([
      "user",
      "assistant",
    ])
  })

  it("answers false for a second question while the first one is still in flight", async () => {
    let openGate: () => void = () => {}
    const gate = new Promise<void>((resolve) => {
      openGate = resolve
    })

    mocks.streamChatCompletion.mockImplementation(async () => {
      mocks.state.calls.push("stream")
      await gate
      return { content: "", deltas: 0, toolCalls: [] }
    })
    const store = useSendMessage()

    const first = store.send("primera")

    await vi.waitFor(() => {
      expect(mocks.state.calls).toContain("stream")
    })

    // The second question was never asked, so its text must come back to the
    // composer instead of vanishing behind the guard.
    expect(await store.send("segunda")).toBe(false)

    openGate()
    expect(await first).toBe(true)
  })
})

describe("useSendMessage.send with a corpus attachment", () => {
  it("declares the search tool and the searchable frame when the chat has a corpus attachment", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    const request = mocks.streamChatCompletion.mock.calls[0][0] as {
      tools?: Array<{ function: { name: string } }>
      messages: Array<{ role: string; content: string }>
    }

    expect(request.tools?.map((tool) => tool.function.name)).toEqual(["search_material"])
    expect(request.messages[0].content).toContain("Buscá ese material con la herramienta")
    // The instruction lives in the frame, so the message stays the bare question.
    expect(request.messages.at(-1)?.content).toBe("¿Qué es el lenguaje ubicuo?")
  })

  it("asks, searches, returns the passages and then persists the answer with its sources", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    scriptStream([
      { toolCalls: [SEARCH_TOOL_CALL] },
      { content: "El modelo del dominio es la raíz del lenguaje ubicuo." },
    ])
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.state.calls).toEqual([
      "append:user",
      "stream",
      "search",
      "stream",
      "append:assistant",
    ])
    expect(mocks.searchCorpus).toHaveBeenCalledWith("lenguaje ubicuo", 6, "doc-ddd")

    const first = mocks.streamChatCompletion.mock.calls[0][0] as {
      messages: Array<{ role: string }>
    }
    const second = mocks.streamChatCompletion.mock.calls[1][0] as {
      messages: Array<{
        role: string
        content: string
        tool_calls?: ToolCall[]
        tool_call_id?: string
      }>
    }

    // The first request carried only the conversation: no tool exchange yet.
    expect(first.messages.map((message) => message.role)).toEqual(["system", "user"])
    // The assistant turn goes back exactly as it arrived, and one tool message
    // per call answers it.
    expect(second.messages.at(-2)).toEqual({
      role: "assistant",
      content: "",
      tool_calls: [SEARCH_TOOL_CALL],
    })
    expect(second.messages.at(-2)?.tool_calls?.[0]).toBe(SEARCH_TOOL_CALL)
    expect(second.messages.at(-1)?.role).toBe("tool")
    expect(second.messages.at(-1)?.tool_call_id).toBe("call-1")
    expect(second.messages.at(-1)?.content).toBe(
      buildContext([
        {
          kind: "corpus",
          title: "Domain-Driven Design",
          page: 42,
          text: SEARCH_RESULT.text,
        },
      ]),
    )

    expect(mocks.appendTurn.mock.calls.at(-1)?.[0]).toEqual({
      role: "assistant",
      content: "El modelo del dominio es la raíz del lenguaje ubicuo.",
      sources: ["Domain-Driven Design, p. 42"],
      consulted: true,
    })
    expect(store.error).toBeNull()
    expect(store.sending).toBe(false)
  })

  it("narrows the search to the attached document the model named", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    const narrowed: ToolCall = {
      ...SEARCH_TOOL_CALL,
      function: {
        name: "search_material",
        arguments: JSON.stringify({ query: "ubicuo", resource_id: "doc-ddd" }),
      },
    }

    scriptStream([{ toolCalls: [narrowed] }, { content: "listo" }])
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.searchCorpus).toHaveBeenCalledWith("ubicuo", 6, "doc-ddd")
  })

  it("uses the chat's top k as the search limit when it overrides the global one", async () => {
    mocks.state.note = makeNote({
      chat: { ...makeNote().chat, settings: { model: null, temperature: null, topK: 3 } },
      attachments: [CORPUS_ATTACHMENT],
    })
    scriptStream([{ toolCalls: [SEARCH_TOOL_CALL] }, { content: "listo" }])
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.searchCorpus).toHaveBeenCalledWith("lenguaje ubicuo", 3, "doc-ddd")
  })

  it("refuses a resource id that is not attached and never searches for it", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    const stray: ToolCall = {
      ...SEARCH_TOOL_CALL,
      function: {
        name: "search_material",
        arguments: JSON.stringify({ query: "secreto", resource_id: "doc-ajeno" }),
      },
    }

    scriptStream([{ toolCalls: [stray] }, { content: "no tengo ese material" }])
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.searchCorpus).not.toHaveBeenCalled()

    const second = mocks.streamChatCompletion.mock.calls[1][0] as {
      messages: Array<{ role: string; content: string; tool_call_id?: string }>
    }
    const toolMessage = second.messages.at(-1)

    expect(toolMessage?.role).toBe("tool")
    expect(toolMessage?.tool_call_id).toBe("call-1")
    expect(toolMessage?.content).toContain("doc-ajeno")
    expect(toolMessage?.content).toContain("no está adjunto")
    expect(mocks.appendTurn.mock.calls.at(-1)?.[0]).toEqual({
      role: "assistant",
      content: "no tengo ese material",
      sources: [],
      consulted: false,
    })
  })

  it("stops after three tool iterations and asks once for the final answer without tools", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    // A model that would search forever: the cap is what ends it.
    scriptStream([
      { toolCalls: [SEARCH_TOOL_CALL] },
      { toolCalls: [SEARCH_TOOL_CALL] },
      { toolCalls: [SEARCH_TOOL_CALL] },
      { toolCalls: [SEARCH_TOOL_CALL] },
      { toolCalls: [SEARCH_TOOL_CALL] },
    ])
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.streamChatCompletion).toHaveBeenCalledTimes(4)
    expect(mocks.searchCorpus).toHaveBeenCalledTimes(3)

    const finalAsk = mocks.streamChatCompletion.mock.calls[3][0] as { tools?: unknown }

    expect("tools" in finalAsk).toBe(false)
    expect(store.error).toBeNull()
    expect(store.sending).toBe(false)
  })

  it("surfaces a failing search as the existing Spanish error and keeps the question", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    mocks.searchCorpus.mockImplementation(async () => {
      throw new Error("No se pudo conectar al servidor. ¿Está corriendo en localhost:8080?")
    })
    scriptStream([{ toolCalls: [SEARCH_TOOL_CALL] }, { content: "nunca llega" }])
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(store.error).toContain("No se pudo conectar al servidor")
    // The question was persisted before the search, so it is not lost.
    expect(mocks.state.calls).toEqual(["append:user", "stream"])
    expect(mocks.appendTurn.mock.calls[0][0]).toEqual({
      role: "user",
      content: "¿Qué es el lenguaje ubicuo?",
      sources: [],
      consulted: true,
    })
    expect(store.sending).toBe(false)
  })

  it("names the model and says it cannot search when the provider rejects the tool", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    mocks.streamChatCompletion.mockImplementation(async () => {
      mocks.state.calls.push("stream")
      throw Object.assign(new Error("El proveedor devolvió un error inesperado (HTTP 400). Detalle: tools is not supported"), {
        status: 400,
      })
    })
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(store.error).toContain("global-model")
    expect(store.error).toContain("no puede buscar el material adjunto")
    // What the provider said, not a guessed capability list.
    expect(store.error).toContain("tools is not supported")
    expect(store.sending).toBe(false)
  })

  it("keeps the plain provider error when the chat declares no tool", async () => {
    mocks.streamChatCompletion.mockImplementation(async () => {
      mocks.state.calls.push("stream")
      throw Object.assign(new Error("El proveedor devolvió un error inesperado (HTTP 400)."), {
        status: 400,
      })
    })
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(store.error).toContain("HTTP 400")
    expect(store.error).not.toContain("no puede buscar el material adjunto")
  })

  it("does not write the tool exchange to the note", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    scriptStream([{ toolCalls: [SEARCH_TOOL_CALL] }, { content: "El modelo del dominio." }])
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    const appended = mocks.appendTurn.mock.calls.map(([turn]) => turn)

    // Two turns only: the machinery of one answer is not part of the conversation.
    expect(appended.map((turn) => turn.role)).toEqual(["user", "assistant"])
    expect(JSON.stringify(appended)).not.toContain("tool_call")
    expect(JSON.stringify(appended)).not.toContain("search_material")
  })
})

describe("useSendMessage.preflight", () => {
  const UNREACHABLE = async () => {
    throw new Error("No se pudo conectar al servidor. ¿Está corriendo en localhost:8080?")
  }

  // S6: the block happens before the first model call, and before the question is
  // written, so there is nothing a retry could duplicate.
  it("blocks a corpus chat before the first model call when the server is down", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    mocks.listDocuments.mockImplementation(UNREACHABLE)
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.streamChatCompletion).not.toHaveBeenCalled()
    expect(mocks.appendTurn).not.toHaveBeenCalled()
    expect(store.error).toContain(`«${CORPUS_ATTACHMENT.title}»`)
    expect(store.error).toContain("el servidor del corpus no responde")
    expect(store.sending).toBe(false)
  })

  it("sends when the corpus answers", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.listDocuments).toHaveBeenCalledTimes(1)
    expect(mocks.streamChatCompletion).toHaveBeenCalledTimes(1)
    expect(store.error).toBeNull()
    expect(mocks.appendTurn.mock.calls.map(([turn]) => turn.role)).toEqual([
      "user",
      "assistant",
    ])
  })

  it("never probes the corpus for a vault-only chat, even with the server down", async () => {
    mocks.state.note = makeNote({ attachments: [VAULT_ATTACHMENT] })
    mocks.listDocuments.mockImplementation(UNREACHABLE)
    const store = useSendMessage()

    await store.send("¿Qué es el estado?")

    expect(mocks.listDocuments).not.toHaveBeenCalled()
    expect(mocks.streamChatCompletion).toHaveBeenCalledTimes(1)
    expect(store.error).toBeNull()
  })

  it("never probes the corpus for a chat with no attachments, even with the server down", async () => {
    mocks.listDocuments.mockImplementation(UNREACHABLE)
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.listDocuments).not.toHaveBeenCalled()
    expect(mocks.streamChatCompletion).toHaveBeenCalledTimes(1)
    expect(store.error).toBeNull()
  })

  it("answers false and leaves nothing in flight or persisted when the corpus is unreachable", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    mocks.listDocuments.mockImplementation(UNREACHABLE)
    const store = useSendMessage()

    const persisted = await store.send("¿Qué es el lenguaje ubicuo?")

    // Nothing was asked, so the text goes back to the composer: the user starts
    // the server and presses send again, and no question is duplicated.
    expect(persisted).toBe(false)
    expect(store.sending).toBe(false)
    expect(store.answer).toBe("")
    expect(mocks.appendTurn).not.toHaveBeenCalled()
    expect(mocks.streamChatCompletion).not.toHaveBeenCalled()
  })

  it("recovers on the next send after a block, without duplicating the question", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    mocks.listDocuments.mockImplementationOnce(UNREACHABLE)
    const store = useSendMessage()

    expect(await store.send("primera")).toBe(false)
    expect(store.error).toContain("el servidor del corpus no responde")
    expect(mocks.state.note?.turns).toHaveLength(0)

    // The same question, asked again once the server is up: exactly one user turn.
    expect(await store.send("primera")).toBe(true)
    expect(store.error).toBeNull()
    expect(mocks.listDocuments).toHaveBeenCalledTimes(2)
    expect(mocks.state.note?.turns.map((turn) => `${turn.role}:${turn.content}`)).toEqual([
      "user:primera",
      "assistant:hola mundo",
    ])
  })

  it("names only the corpus title in the error when the chat also has a vault file", async () => {
    mocks.state.note = makeNote({ attachments: [VAULT_ATTACHMENT, CORPUS_ATTACHMENT] })
    mocks.listDocuments.mockImplementation(UNREACHABLE)
    const store = useSendMessage()

    expect(await store.send("¿Qué es el estado?")).toBe(false)
    expect(store.error).toContain(`«${CORPUS_ATTACHMENT.title}»`)
    expect(store.error).not.toContain(VAULT_PATH)
    // The vault file is in the message and needs no server, so only the corpus
    // half is named, and the model is never reached with a material claim it
    // cannot honour.
    expect(store.error).toContain("el servidor del corpus no responde")
    expect(mocks.streamChatCompletion).not.toHaveBeenCalled()
    expect(mocks.appendTurn).not.toHaveBeenCalled()
  })
})

describe("useSendMessage.consulted", () => {
  /** The assistant turn the store persisted last. */
  function lastAnswer() {
    return mocks.appendTurn.mock.calls.at(-1)?.[0]
  }

  // The third state of the frame: the chat has material behind the tool and no
  // search ran, so the answer is a memory answer and the note has to say so.
  it("records consulted false when the model answers without searching the corpus", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.searchCorpus).not.toHaveBeenCalled()
    expect(lastAnswer()).toEqual({
      role: "assistant",
      content: "hola mundo",
      sources: [],
      consulted: false,
    })
  })

  it("records consulted true when one tool call actually searched", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    scriptStream([{ toolCalls: [SEARCH_TOOL_CALL] }, { content: "El modelo del dominio." }])
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.searchCorpus).toHaveBeenCalledTimes(1)
    expect(lastAnswer()?.consulted).toBe(true)
  })

  // A call the client refuses for scope read no material: it ran nothing, so it
  // cannot turn a memory answer into a founded one.
  it("does not count a call refused for scope as a consultation", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    const stray: ToolCall = {
      ...SEARCH_TOOL_CALL,
      function: {
        name: "search_material",
        arguments: JSON.stringify({ query: "secreto", resource_id: "doc-ajeno" }),
      },
    }

    scriptStream([{ toolCalls: [stray] }, { content: "no tengo ese material" }])
    const store = useSendMessage()

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.searchCorpus).not.toHaveBeenCalled()
    expect(lastAnswer()).toEqual({
      role: "assistant",
      content: "no tengo ese material",
      sources: [],
      consulted: false,
    })
  })

  // The flag is one per answer and reaches both places that write a turn, so a
  // cut short answer is not silently promoted to a founded one.
  it("records consulted false on the partial answer of an unconsulted chat", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    mocks.streamChatCompletion.mockImplementation(
      async (req: { onDelta?: (delta: string) => void }) => {
        mocks.state.calls.push("stream")
        req.onDelta?.("Un agregado es")
        throw new Error("el stream se cortó")
      },
    )
    const store = useSendMessage()

    await store.send("¿Qué es el agregado?")

    expect(lastAnswer()).toEqual({
      role: "assistant",
      content: `Un agregado es\n\n${INTERRUPTED_MARKER}`,
      sources: [],
      consulted: false,
    })
  })
})

// The pre-existing cases of a vault-only chat, unchanged by the flag: a vault
// file is in the message, so there is nothing behind the tool to consult.
describe("useSendMessage.send with a vault attachment", () => {
  it("records consulted true: a vault file is in the message, not behind the tool", async () => {
    mocks.state.note = makeNote({ attachments: [VAULT_ATTACHMENT] })
    const store = useSendMessage()

    await store.send("¿Qué es el estado?")

    expect(mocks.appendTurn.mock.calls.at(-1)?.[0]?.consulted).toBe(true)
  })
  it("sends the file's text whole, with the citation frame, and declares no tools", async () => {
    mocks.state.note = makeNote({ attachments: [VAULT_ATTACHMENT] })
    const store = useSendMessage()

    await store.send("¿Qué es el estado?")

    const request = mocks.streamChatCompletion.mock.calls[0][0] as {
      tools?: unknown
      messages: Array<{ role: string; content: string }>
    }

    // One round trip and no server: a vault file needs neither the tool nor the
    // backend, which is S7 and S1 in the same request.
    expect("tools" in request).toBe(false)
    expect(mocks.state.calls).toEqual(["append:user", "stream", "append:assistant"])
    expect(request.messages.map((message) => message.role)).toEqual(["system", "user"])
    // The chat does have material, so the no-material frame would be a lie and
    // the citation frame is what its answer is held to.
    expect(request.messages[0].content).toBe(buildSystemMessage("Sos un tutor."))
    expect(request.messages[0].content).not.toContain("no tiene material adjunto")
    // The material travels in the message, whole, as a vault block.
    expect(request.messages.at(-1)?.content).toBe(
      `${buildContext([{ kind: "vault", path: VAULT_PATH, text: VAULT_TEXT }])}\n\n¿Qué es el estado?`,
    )
    expect(request.messages.at(-1)?.content).toContain(VAULT_TEXT)
    // No page number is invented in the material either.
    expect(request.messages.at(-1)?.content).not.toContain("p. ")
  })

  it("cites the file by its path when it persists the answer", async () => {
    mocks.state.note = makeNote({ attachments: [VAULT_ATTACHMENT] })
    const store = useSendMessage()

    await store.send("¿Qué es el estado?")

    const appended = mocks.appendTurn.mock.calls.at(-1)?.[0]

    expect(appended).toEqual({
      role: "assistant",
      content: "hola mundo",
      sources: [VAULT_PATH],
      consulted: true,
    })
    expect(JSON.stringify(appended)).not.toContain("p. ")
  })

  it("sends the vault text and declares the search tool when the chat has both classes", async () => {
    mocks.state.note = makeNote({ attachments: [VAULT_ATTACHMENT, CORPUS_ATTACHMENT] })
    scriptStream([{ toolCalls: [SEARCH_TOOL_CALL] }, { content: "El modelo del dominio." }])
    const store = useSendMessage()

    await store.send("¿Qué es el estado?")

    const first = mocks.streamChatCompletion.mock.calls[0][0] as {
      tools?: Array<{ function: { name: string } }>
      messages: Array<{ role: string; content: string }>
    }

    // Both halves of the request: the vault text is in the message, and the tool
    // is declared for the corpus half.
    expect(first.tools?.map((tool) => tool.function.name)).toEqual(["search_material"])
    expect(first.messages.at(-1)?.content).toContain(VAULT_TEXT)
    expect(first.messages.at(-1)?.content).toContain("¿Qué es el estado?")
    expect(mocks.state.calls).toEqual([
      "append:user",
      "stream",
      "search",
      "stream",
      "append:assistant",
    ])
    // Cited in block order: the vault file first, then what the tool returned.
    expect(mocks.appendTurn.mock.calls.at(-1)?.[0]).toEqual({
      role: "assistant",
      content: "El modelo del dominio.",
      sources: [VAULT_PATH, "Domain-Driven Design, p. 42"],
      consulted: true,
    })
  })

  it("refuses the send and keeps the question when a file cannot be read", async () => {
    mocks.state.note = makeNote({ attachments: [VAULT_ATTACHMENT] })
    // The note was moved or deleted after it was attached.
    vaultFiles = {}
    const store = useSendMessage()

    const consumed = await store.send("¿Qué es el estado?")

    expect(store.error).toContain("No se pudo leer")
    expect(store.error).toContain(VAULT_PATH)
    expect(store.error).toContain("volvé a intentar")
    // The question was consumed even though no answer came: it is in the note, so
    // the caller must not hand the text back -- a retry would ask it twice.
    expect(consumed).toBe(true)
    // Nothing is sent: an answer without the material the chat claims would be
    // an answer that pretends to have read it.
    expect(mocks.streamChatCompletion).not.toHaveBeenCalled()
    // The question is still a fact, so it stays in the note.
    expect(mocks.appendTurn.mock.calls.map(([turn]) => turn.role)).toEqual(["user"])
    expect(mocks.appendTurn.mock.calls[0][0]).toEqual({
      role: "user",
      content: "¿Qué es el estado?",
      sources: [],
      consulted: true,
    })
    expect(store.sending).toBe(false)
  })

  it("refuses the send when a file grew past the cap after it was attached", async () => {
    mocks.state.note = makeNote({ attachments: [VAULT_ATTACHMENT] })
    vaultFiles = { [VAULT_PATH]: "x".repeat(MAX_ATTACHMENT_CHARS + 1) }
    const store = useSendMessage()

    await store.send("¿Qué es el estado?")

    expect(store.error).toContain(VAULT_PATH)
    expect(store.error).toContain(String(MAX_ATTACHMENT_CHARS))
    expect(store.error).toContain("no se envió")
    expect(mocks.streamChatCompletion).not.toHaveBeenCalled()
    expect(mocks.appendTurn.mock.calls.map(([turn]) => turn.role)).toEqual(["user"])
    expect(store.sending).toBe(false)
  })

  it("refuses instead of crashing when the vault reader was never installed", async () => {
    mocks.state.note = makeNote({ attachments: [VAULT_ATTACHMENT] })
    setVaultTextReader(null)
    const store = useSendMessage()

    await store.send("¿Qué es el estado?")

    expect(store.error).toContain(VAULT_PATH)
    expect(store.error).toContain("lector del vault no está disponible")
    expect(mocks.streamChatCompletion).not.toHaveBeenCalled()
    expect(store.sending).toBe(false)
  })
})

// The send path's other promise: it answers whether the question was consumed, and
// the note is what decides that -- not the caller, and not the fact that it tried.
describe("useSendMessage when the note refuses the turn", () => {
  it("answers that nothing was consumed, and spends no model call", async () => {
    // The note could not be written: no paths, a read-only vault, a failed write.
    mocks.state.writeFails = true
    const store = useSendMessage()

    const consumed = await store.send("¿Qué es el agregado?")

    expect(consumed).toBe(false)
    // Nothing is asked of the model: its answer would have nowhere to live.
    expect(mocks.streamChatCompletion).not.toHaveBeenCalled()
    // The question was attempted once and the assistant never answered.
    expect(mocks.appendTurn.mock.calls.map(([turn]) => turn.role)).toEqual(["user"])
    expect(store.sending).toBe(false)
  })
})

describe("useSendMessage.cancel during the preflight", () => {
  /** A server that accepted the connection and never answers. */
  function hangingProbe(rejectWith: () => Error) {
    return (signal?: AbortSignal) =>
      new Promise((_resolve, reject) => {
        signal?.addEventListener("abort", () => reject(rejectWith()))
      })
  }

  it("ends the send without an error when the probe is cancelled", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    mocks.listDocuments.mockImplementation(
      hangingProbe(() => {
        const error = new Error("La respuesta se canceló.")

        error.name = "AbortError"
        return error
      }),
    )
    const store = useSendMessage()

    const pending = store.send("¿Qué es el lenguaje ubicuo?")

    await vi.waitFor(() => {
      expect(mocks.listDocuments).toHaveBeenCalledTimes(1)
    })

    store.cancel()

    // Cancelling is the user's own decision: no message, nothing written, and
    // the composer back to idle instead of wedged behind a stopped probe.
    expect(await pending).toBe(false)
    expect(store.error).toBeNull()
    expect(store.sending).toBe(false)
    expect(mocks.appendTurn).not.toHaveBeenCalled()
    expect(mocks.streamChatCompletion).not.toHaveBeenCalled()
  })

  it("stays quiet even when the client reported the cancel as a network failure", async () => {
    // `shared/api/client.ts` maps every fetch failure, an abort included, to its
    // own `NetworkError`. The signal is what tells the two apart, which is why the
    // probe decides from it and not from the thrown value.
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    mocks.listDocuments.mockImplementation(
      hangingProbe(() => Object.assign(new Error("no responde"), { name: "NetworkError" })),
    )
    const store = useSendMessage()

    const pending = store.send("¿Qué es el lenguaje ubicuo?")

    await vi.waitFor(() => {
      expect(mocks.listDocuments).toHaveBeenCalledTimes(1)
    })

    store.cancel()
    await pending

    expect(store.error).toBeNull()
    expect(store.sending).toBe(false)
    expect(mocks.appendTurn).not.toHaveBeenCalled()
  })

})

describe("useSendMessage.cancel", () => {
  it("is a no-op when nothing is in flight", () => {
    const store = useSendMessage()

    expect(() => store.cancel()).not.toThrow()
  })

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
      consulted: true,
    })
  })

  it("stops the loop between iterations instead of starting the next request", async () => {
    mocks.state.note = makeNote({ attachments: [CORPUS_ATTACHMENT] })
    scriptStream([{ toolCalls: [SEARCH_TOOL_CALL] }, { content: "nunca llega" }])

    const store = useSendMessage()

    // The user cancels while the tool is running: the next iteration must not start.
    mocks.searchCorpus.mockImplementation(async () => {
      store.cancel()
      return [SEARCH_RESULT]
    })

    await store.send("¿Qué es el lenguaje ubicuo?")

    expect(mocks.streamChatCompletion).toHaveBeenCalledTimes(1)
    expect(mocks.state.calls).toEqual(["append:user", "stream"])
    expect(store.error).toBeNull()
    expect(store.sending).toBe(false)
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
