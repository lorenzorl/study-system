import { beforeEach, describe, expect, it, vi } from "vitest"
import { createPinia, setActivePinia } from "pinia"
import { useChatsStore } from "./useChatsStore"
import type { ChatNote, ChatTurn } from "./types"

// The note adapter is mocked, so what is under test is the store's own decision --
// including the one it now reports: whether the note took the turn.
const api = vi.hoisted(() => ({
  appendTurn: vi.fn(),
  createChatForTitle: vi.fn(),
  listChatPaths: vi.fn(),
  readChatNote: vi.fn(),
  writeChatAttachments: vi.fn(),
  writeChatPrompt: vi.fn(),
  writeChatSettings: vi.fn(),
}))

vi.mock("../api/chat-note", () => api)

const NOTE: ChatNote = {
  chat: {
    path: "Study Dashboard/Chats/test.md",
    title: "test",
    prompt: "",
    settings: { model: null, temperature: null, topK: null },
  },
  turns: [],
  attachments: [],
}

const TURN: ChatTurn = {
  role: "user",
  content: "¿Qué es el agregado?",
  sources: [],
  consulted: true,
}

/** The store with a chat open, through the path the app uses. */
async function openChat(): Promise<ReturnType<typeof useChatsStore>> {
  const store = useChatsStore()
  api.readChatNote.mockResolvedValue(NOTE)
  await store.selectChat(NOTE.chat.path)

  return store
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe("useChatsStore.appendTurn", () => {
  it("answers true and updates the note when the write lands", async () => {
    const store = await openChat()

    await expect(store.appendTurn(TURN)).resolves.toBe(true)

    expect(store.note?.turns).toEqual([TURN])
    expect(store.error).toBeNull()
  })

  // The send path decides with this answer whether the user's question was
  // consumed, so a failed write cannot answer as if it had landed.
  it("answers false and keeps the reason when the note refuses the write", async () => {
    const store = await openChat()
    api.appendTurn.mockRejectedValueOnce(new Error("sin permiso"))

    await expect(store.appendTurn(TURN)).resolves.toBe(false)

    expect(store.error).toContain("sin permiso")
    expect(store.note?.turns).toEqual([])
  })

  it("answers false with no chat open instead of pretending it wrote", async () => {
    const store = useChatsStore()

    await expect(store.appendTurn(TURN)).resolves.toBe(false)

    expect(api.appendTurn).not.toHaveBeenCalled()
  })
})
