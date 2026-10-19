import { beforeEach, describe, expect, it, vi } from "vitest"
import { createPinia, setActivePinia } from "pinia"
import type { ChatAttachment, ChatNote } from "@/entities/chat"
import { useChatsStore } from "@/entities/chat"
import type { DocumentSummary } from "@/entities/document"
import { MAX_ATTACHMENT_CHARS } from "./attachment-limits"
import { useChatAttachments } from "./useChatAttachments"

// The chat slice's note adapter is mocked, so the real store is exercised and the
// assertions land on what reached the note instead of on a stand-in for it.
const noteApi = vi.hoisted(() => ({
  appendTurn: vi.fn(async () => {}),
  createChatForTitle: vi.fn(async () => "Study Dashboard/Chats/chat.md"),
  listChatPaths: vi.fn(async () => []),
  readChatNote: vi.fn(async () => null),
  writeChatAttachments: vi.fn(async () => {}),
  writeChatPrompt: vi.fn(async () => {}),
  writeChatSettings: vi.fn(async () => {}),
}))

vi.mock("@/entities/chat/api/chat-note", () => noteApi)

// The corpus list is the one network call this store makes.
const documentApi = vi.hoisted(() => ({ listDocuments: vi.fn() }))

vi.mock("@/entities/document", () => documentApi)

// The vault boundary: one in-memory map shared by the read and the listing.
const vault = vi.hoisted(() => ({ files: new Map<string, string>() }))

vi.mock("@/shared/obsidian", () => ({
  readVaultFile: vi.fn(async (path: string) => {
    const content = vault.files.get(path)
    if (content === undefined) throw new Error(`No se encontró la nota "${path}".`)
    return content
  }),
  listVaultMarkdownFiles: vi.fn(() => [...vault.files.keys()].sort()),
}))

const CHAT_PATH = "Study Dashboard/Chats/chat.md"

const DDD: DocumentSummary = {
  id: "d1",
  title: "DDD",
  createdAt: "2026-10-18T10:00:00Z",
  pages: 359,
  chunks: 1200,
}

const OTHER: DocumentSummary = {
  id: "d2",
  title: "SICP",
  createdAt: "2026-10-19T10:00:00Z",
  pages: 0,
  chunks: 0,
}

function noteWith(attachments: ChatAttachment[]): ChatNote {
  return {
    chat: {
      path: CHAT_PATH,
      title: "chat",
      prompt: "",
      settings: { model: null, temperature: null, topK: null },
    },
    turns: [],
    attachments,
  }
}

/** Every attachment list that reached the note, in call order. */
function writes(): ChatAttachment[][] {
  const calls = noteApi.writeChatAttachments.mock.calls as unknown as [
    string,
    ChatAttachment[],
  ][]

  for (const [path] of calls) {
    expect(path).toBe(CHAT_PATH)
  }

  return calls.map(([, attachments]) => attachments)
}

/** Open a chat the way `selectChat` does: the path and the note together. */
function openChat(attachments: ChatAttachment[] = []): void {
  const chats = useChatsStore()
  chats.currentPath = CHAT_PATH
  chats.note = noteWith(attachments)
}

beforeEach(() => {
  // A fresh pinia per test, so the chats store cannot carry one test's note into
  // the next one.
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vault.files.clear()

  documentApi.listDocuments.mockResolvedValue([DDD, OTHER])

  openChat()
})

describe("useChatAttachments", () => {
  it("lists the corpus and clears the error of a previous failure", async () => {
    documentApi.listDocuments.mockRejectedValueOnce(new Error("sin servidor"))

    const store = useChatAttachments()
    await store.loadCorpus()
    expect(store.error).not.toBeNull()

    await store.loadCorpus()

    expect(store.corpus).toEqual([DDD, OTHER])
    expect(store.error).toBeNull()
    expect(store.loadingCorpus).toBe(false)
  })

  it("reports a list failure in the store's own error instead of throwing", async () => {
    documentApi.listDocuments.mockRejectedValue(new Error("sin servidor"))

    const store = useChatAttachments()
    await store.loadCorpus()

    expect(store.corpus).toEqual([])
    expect(store.error).toContain("No se pudo listar el corpus")
    expect(store.loadingCorpus).toBe(false)
  })

  it("offers the vault's markdown files as candidates", () => {
    vault.files.set("Notas/Tesis.md", "# Tesis")

    const store = useChatAttachments()
    store.loadVaultFiles()

    expect(store.vaultFiles).toEqual(["Notas/Tesis.md"])
  })

  it("attaches a corpus document and writes it to the note", async () => {
    const store = useChatAttachments()
    await store.attachCorpus(DDD)

    expect(writes()).toEqual([
      [{ kind: "corpus", documentId: "d1", title: "DDD" }],
    ])
    expect(store.attachments).toEqual([
      { kind: "corpus", documentId: "d1", title: "DDD" },
    ])
  })

  it("attaches the same corpus document once, however many times it is asked", async () => {
    const store = useChatAttachments()
    await store.attachCorpus(DDD)
    await store.attachCorpus(DDD)

    expect(writes()).toHaveLength(1)
  })

  it("attaches two different corpus documents in the order they were chosen", async () => {
    const store = useChatAttachments()
    await store.attachCorpus(DDD)
    await store.attachCorpus(OTHER)

    expect(writes()).toEqual([
      [{ kind: "corpus", documentId: "d1", title: "DDD" }],
      [
        { kind: "corpus", documentId: "d1", title: "DDD" },
        { kind: "corpus", documentId: "d2", title: "SICP" },
      ],
    ])
  })

  it("attaches a vault file by its path", async () => {
    vault.files.set("Notas/Tesis.md", "# Tesis")
    const store = useChatAttachments()

    await store.attachVault("Notas/Tesis.md")

    expect(writes()).toEqual([[{ kind: "vault", path: "Notas/Tesis.md" }]])
  })

  it("refuses a vault file over the cap with the size policy's message and writes nothing", async () => {
    vault.files.set("Notas/grande.md", "x".repeat(MAX_ATTACHMENT_CHARS + 1))

    const store = useChatAttachments()
    await store.attachVault("Notas/grande.md")

    expect(store.error).toContain("Notas/grande.md")
    expect(store.error).toContain(String(MAX_ATTACHMENT_CHARS))
    expect(writes()).toEqual([])
  })

  it("refuses a vault path that is already attached instead of counting it twice", async () => {
    // The size policy sums the note's vault attachments by reading them, so a
    // path attached twice would spend the total budget twice for one file.
    vault.files.set("Notas/Tesis.md", "# Tesis")

    const store = useChatAttachments()
    await store.attachVault("Notas/Tesis.md")
    await store.attachVault("Notas/Tesis.md")

    expect(store.error).toContain("ya está adjunto")
    expect(writes()).toEqual([[{ kind: "vault", path: "Notas/Tesis.md" }]])
  })

  it("refuses a vault file that cannot be read, and writes nothing", async () => {
    const store = useChatAttachments()
    await store.attachVault("Notas/falta.md")

    expect(store.error).toContain("Notas/falta.md")
    expect(writes()).toEqual([])
  })

  it("detaches one attachment and keeps the rest in order", async () => {
    openChat([
      { kind: "corpus", documentId: "d1", title: "DDD" },
      { kind: "vault", path: "Notas/Tesis.md" },
    ])

    const store = useChatAttachments()
    await store.detach({ kind: "corpus", documentId: "d1", title: "DDD" })

    expect(writes()).toEqual([[{ kind: "vault", path: "Notas/Tesis.md" }]])
  })

  it("detaches the last attachment by writing an empty region", async () => {
    openChat([{ kind: "vault", path: "Notas/Tesis.md" }])

    const store = useChatAttachments()
    await store.detach({ kind: "vault", path: "Notas/Tesis.md" })

    expect(writes()).toEqual([[]])
    expect(store.attachments).toEqual([])
  })

  it("reports a write the note refused, and does not show it as attached", async () => {
    noteApi.writeChatAttachments.mockRejectedValueOnce(new Error("sin permiso"))

    const store = useChatAttachments()
    await store.attachCorpus(DDD)

    expect(store.error).toContain("sin permiso")
    expect(store.attachments).toEqual([])
  })

  it("refuses to attach with no chat open instead of writing nowhere", async () => {
    const chats = useChatsStore()
    chats.currentPath = null
    chats.note = null

    const store = useChatAttachments()
    await store.attachCorpus(DDD)

    expect(writes()).toEqual([])
    expect(store.error).toContain("No hay un chat abierto")
  })
})
