import { beforeEach, describe, expect, it, vi } from "vitest"

// The vault boundary is mocked with a single in-memory Map shared by every
// boundary function, so the api segment exercises the real region surgery from
// `lib/note.ts` against persisted content.
const fake = vi.hoisted(() => ({ store: new Map<string, string>() }))

vi.mock("@/shared/obsidian", () => ({
  chatsFolderPath: vi.fn(() => "Study Dashboard/Chats"),
  listVaultFiles: vi.fn((folder: string) =>
    [...fake.store.keys()]
      .filter((path) => path.startsWith(`${folder}/`))
      .sort(),
  ),
  readVaultFile: vi.fn(async (path: string) => {
    const content = fake.store.get(path)
    if (content === undefined) {
      throw new Error(`No se encontró la nota "${path}".`)
    }
    return content
  }),
  writeVaultFile: vi.fn(async (path: string, content: string) => {
    fake.store.set(path, content)
  }),
  processVaultFile: vi.fn(async (path: string, fn: (data: string) => string) => {
    const next = fn(fake.store.get(path) ?? "")
    fake.store.set(path, next)
    return next
  }),
  onVaultFileChange: vi.fn(() => () => {}),
}))

import { listVaultFiles, processVaultFile } from "@/shared/obsidian"
import {
  appendTurn,
  createChatForTitle,
  createChatNote,
  listChatPaths,
  writeChatAttachments,
} from "./chat-note"

const PATH = "Study Dashboard/Chats/tutor.md"

// A create that lands on a path that already exists must not replace it: the two
// default clicks on "Nuevo chat" both name the note "chat", and one of them must
// not consume the other's transcript.
describe("createChatNote on a path that already exists", () => {
  it("keeps the transcript, the prompt and the user's prose", async () => {
    const existing = [
      "---",
      "tags: mine",
      "---",
      "USER TEXT",
      "<!-- prompt:begin -->",
      "mi prompt",
      "<!-- prompt:end -->",
      "TAIL",
      "<!-- chat:begin -->",
      "### 🧑 hola",
      "<!-- chat:end -->",
    ].join("\n")
    fake.store.set(PATH, existing)

    await createChatNote(PATH, "nuevo", {
      model: null,
      temperature: null,
      topK: null,
    })

    const stored = fake.store.get(PATH) as string
    expect(stored).toContain("USER TEXT")
    expect(stored).toContain("TAIL")
    expect(stored).toContain("tags: mine")
    expect(stored).toContain("### 🧑 hola")
    expect(stored).toContain("mi prompt")
  })
})

// The scenario the user hits: "Nuevo chat" twice, both named "Chat".
describe("createChatForTitle", () => {
  it("counts up instead of replacing the chat that holds the name", async () => {
    const first = await createChatForTitle("Chat", "p", {
      model: null,
      temperature: null,
      topK: null,
    })
    await appendTurn(first, { role: "user", content: "hola", sources: [] })

    const second = await createChatForTitle("Chat", "p", {
      model: null,
      temperature: null,
      topK: null,
    })

    expect(first).toBe("Study Dashboard/Chats/chat.md")
    expect(second).toBe("Study Dashboard/Chats/chat-2.md")
    expect(fake.store.get(first)).toContain("hola")
  })
})

function baseNote(): string {
  return [
    "---",
    "model: m",
    "---",
    "<!-- prompt:begin -->",
    "p",
    "<!-- prompt:end -->",
    "",
    "<!-- chat:begin -->",
    "<!-- chat:end -->",
    "",
  ].join("\n")
}

beforeEach(() => {
  fake.store.clear()
  vi.clearAllMocks()
})

describe("listChatPaths", () => {
  it("returns what the vault boundary returns", async () => {
    vi.mocked(listVaultFiles).mockReturnValueOnce([
      PATH,
      "Study Dashboard/Chats/otro.md",
    ])

    await expect(listChatPaths()).resolves.toEqual([
      PATH,
      "Study Dashboard/Chats/otro.md",
    ])
    expect(listVaultFiles).toHaveBeenCalledWith("Study Dashboard/Chats")
  })
})

describe("createChatNote", () => {
  it("writes the full note to the given path", async () => {
    await createChatNote(PATH, "Sos mi tutor.", {
      model: "m",
      temperature: 0.2,
      topK: 8,
    })

    const stored = fake.store.get(PATH)
    expect(stored).toBeDefined()
    expect(stored).toContain("---\nmodel: m\ntemperature: 0.2\ntop_k: 8\n---")
    expect(stored).toContain(
      "<!-- prompt:begin -->\nSos mi tutor.\n<!-- prompt:end -->",
    )
    expect(stored).toContain("<!-- chat:begin -->\n<!-- chat:end -->")
    expect(processVaultFile).toHaveBeenCalled()
  })
})

describe("appendTurn", () => {
  it("persists through processVaultFile and the stored note gains the turn", async () => {
    fake.store.set(PATH, baseNote())

    await appendTurn(PATH, { role: "user", content: "Hola", sources: [] })

    expect(processVaultFile).toHaveBeenCalledWith(PATH, expect.any(Function))
    expect(fake.store.get(PATH)).toContain("### 🧑 Hola")
  })

  it("leaves user text outside the regions unchanged", async () => {
    const raw = [
      "---",
      "model: m",
      "---",
      "USER INTRO",
      "<!-- prompt:begin -->",
      "p",
      "<!-- prompt:end -->",
      "USER MIDDLE",
      "<!-- chat:begin -->",
      "<!-- chat:end -->",
      "USER TAIL",
      "",
    ].join("\n")
    fake.store.set(PATH, raw)

    await appendTurn(PATH, { role: "user", content: "Hola", sources: [] })

    const stored = fake.store.get(PATH) as string
    const chatBegin = "<!-- chat:begin -->"
    const chatEnd = "<!-- chat:end -->"
    expect(stored.slice(0, stored.indexOf(chatBegin))).toBe(
      raw.slice(0, raw.indexOf(chatBegin)),
    )
    expect(
      stored.slice(stored.indexOf(chatEnd) + chatEnd.length),
    ).toBe(raw.slice(raw.indexOf(chatEnd) + chatEnd.length))
  })
})

// The attachment list is a region like the prompt and the transcript: one write
// routes through `processVaultFile`, and nothing outside the region moves.
describe("writeChatAttachments", () => {
  it("persists through processVaultFile and the stored note holds the list", async () => {
    fake.store.set(PATH, baseNote())

    await writeChatAttachments(PATH, [
      { kind: "corpus", documentId: "d1", title: "DDD" },
      { kind: "vault", path: "Notas/Tesis.md" },
    ])

    expect(processVaultFile).toHaveBeenCalledWith(PATH, expect.any(Function))
    const stored = fake.store.get(PATH) as string
    expect(stored).toContain("<!-- adjuntos:begin -->\n- corpus | d1 | DDD\n- vault | Notas/Tesis.md\n<!-- adjuntos:end -->")
  })

  it("leaves user text outside the regions unchanged", async () => {
    const raw = [
      "---",
      "model: m",
      "---",
      "USER INTRO",
      "<!-- prompt:begin -->",
      "p",
      "<!-- prompt:end -->",
      "USER MIDDLE",
      "<!-- adjuntos:begin -->",
      "<!-- adjuntos:end -->",
      "USER TAIL",
      "<!-- chat:begin -->",
      "<!-- chat:end -->",
      "USER END",
      "",
    ].join("\n")
    fake.store.set(PATH, raw)

    await writeChatAttachments(PATH, [{ kind: "vault", path: "Notas/Tesis.md" }])

    const stored = fake.store.get(PATH) as string
    const before = "<!-- adjuntos:begin -->"
    const after = "<!-- adjuntos:end -->"
    expect(stored.slice(0, stored.indexOf(before))).toBe(
      raw.slice(0, raw.indexOf(before)),
    )
    expect(stored.slice(stored.indexOf(after) + after.length)).toBe(
      raw.slice(raw.indexOf(after) + after.length),
    )
  })

  it("empties the region when the last attachment is removed", async () => {
    fake.store.set(PATH, baseNote())

    await writeChatAttachments(PATH, [{ kind: "vault", path: "Notas/Tesis.md" }])
    await writeChatAttachments(PATH, [])

    const stored = fake.store.get(PATH) as string
    expect(stored).toContain("<!-- adjuntos:begin -->\n<!-- adjuntos:end -->")
    expect(stored).not.toContain("Notas/Tesis.md")
  })
})
