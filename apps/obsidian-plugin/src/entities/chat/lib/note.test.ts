import { describe, expect, it } from "vitest"
import type { ChatAttachment, ChatNoteSettings } from "../model/types"
import { parseFrontmatter } from "./frontmatter"
import {
  appendChatTurn,
  createChatNote,
  parseChatNote,
  setAttachments,
  setChatPrompt,
  setChatSettings,
} from "./note"

const PATH = "Study Dashboard/Chats/tutor.md"
const SETTINGS: ChatNoteSettings = {
  model: "qwen/qwen3.5-flash-02-23",
  temperature: 0.2,
  topK: 8,
}

const PROMPT_END = "<!-- prompt:end -->"
const CHAT_BEGIN = "<!-- chat:begin -->"
const CHAT_END = "<!-- chat:end -->"
const ATTACHMENTS_BEGIN = "<!-- adjuntos:begin -->"
const ATTACHMENTS_END = "<!-- adjuntos:end -->"

/** Everything strictly before the first occurrence of `marker`. */
function before(raw: string, marker: string): string {
  return raw.slice(0, raw.indexOf(marker))
}

/** Everything strictly after the first occurrence of `marker`. */
function after(raw: string, marker: string): string {
  const index = raw.indexOf(marker)
  return raw.slice(index + marker.length)
}

// ── parse / create round-trip ───────────────────────────────────────────────

describe("parseChatNote & createChatNote", () => {
  it("a fresh note parses back to what was created", () => {
    const raw = createChatNote("Sos mi tutor de DDD.", SETTINGS)
    const note = parseChatNote(raw, PATH)

    expect(note.chat.path).toBe(PATH)
    expect(note.chat.title).toBe("tutor")
    expect(note.chat.prompt).toBe("Sos mi tutor de DDD.")
    expect(note.chat.settings).toEqual(SETTINGS)
    expect(note.turns).toEqual([])
  })

  it("keeps null overrides as null", () => {
    const raw = createChatNote("p", {
      model: null,
      temperature: null,
      topK: null,
    })
    const note = parseChatNote(raw, PATH)

    expect(note.chat.settings).toEqual({
      model: null,
      temperature: null,
      topK: null,
    })
  })

  it("parses a plain note as an empty chat", () => {
    const note = parseChatNote("Just user text\n", PATH)

    expect(note.chat.prompt).toBe("")
    expect(note.chat.settings).toEqual({
      model: null,
      temperature: null,
      topK: null,
    })
    expect(note.turns).toEqual([])
  })

  it("derives the title from the file name", () => {
    expect(parseChatNote("", "Study Dashboard/Chats/mi-chat.md").chat.title).toBe(
      "mi-chat",
    )
  })
})

// ── appendChatTurn ──────────────────────────────────────────────────────────

describe("appendChatTurn", () => {
  it("appends two turns in order and round-trips content and sources", () => {
    let raw = createChatNote("Sos mi tutor.", SETTINGS)
    raw = appendChatTurn(raw, {
      role: "user",
      content: "¿Qué es un agregado?",
      sources: [],
    })
    raw = appendChatTurn(raw, {
      role: "assistant",
      content: "Un agregado es…",
      sources: ["Domain-Driven Design, p. 42"],
    })

    expect(parseChatNote(raw, PATH).turns).toEqual([
      { role: "user", content: "¿Qué es un agregado?", sources: [] },
      {
        role: "assistant",
        content: "Un agregado es…",
        sources: ["Domain-Driven Design, p. 42"],
      },
    ])
  })

  it("keeps the note's rendering out of the source data", () => {
    let raw = createChatNote("p", SETTINGS)
    raw = appendChatTurn(raw, {
      role: "assistant",
      content: "A",
      sources: ["Fuente: DDD, p. 1"],
    })

    expect(raw).toContain("Fuente: DDD, p. 1")
    expect(raw).not.toContain("Fuente: Fuente:")
    expect(parseChatNote(raw, PATH).turns[0].sources).toEqual(["DDD, p. 1"])
  })

  it("leaves everything outside the chat region byte-identical", () => {
    const raw = [
      "---",
      "model: m",
      "---",
      "USER INTRO",
      "<!-- prompt:begin -->",
      "p",
      PROMPT_END,
      "USER MIDDLE",
      CHAT_BEGIN,
      CHAT_END,
      "USER TAIL",
      "",
    ].join("\n")

    const result = appendChatTurn(raw, {
      role: "user",
      content: "Hola",
      sources: [],
    })

    expect(before(result, CHAT_BEGIN)).toBe(before(raw, CHAT_BEGIN))
    expect(after(result, CHAT_END)).toBe(after(raw, CHAT_END))
    expect(parseChatNote(result, PATH).turns).toHaveLength(1)
  })

  it("adds a chat region at the very end when none exists", () => {
    const raw = "# Nota\n\nTexto del usuario.\n"

    const result = appendChatTurn(raw, {
      role: "user",
      content: "Hola",
      sources: [],
    })

    expect(result.startsWith(raw)).toBe(true)
    expect(parseChatNote(result, PATH).turns).toEqual([
      { role: "user", content: "Hola", sources: [] },
    ])
  })

  it("parses a turn whose content contains a heading or a comment-like string", () => {
    const content = "# Título\n\nTexto con <!-- comentario --> adentro."
    let raw = createChatNote("p", SETTINGS)
    raw = appendChatTurn(raw, { role: "assistant", content, sources: [] })

    const turns = parseChatNote(raw, PATH).turns

    expect(turns).toHaveLength(1)
    expect(turns[0].content).toBe(content)
    expect(turns[0].sources).toEqual([])
  })
})

// ── setChatPrompt ───────────────────────────────────────────────────────────

describe("setChatPrompt", () => {
  it("replaces only what is between the prompt markers", () => {
    const raw = [
      "---",
      "model: m",
      "---",
      "USER INTRO",
      "<!-- prompt:begin -->",
      "old prompt",
      PROMPT_END,
      "USER MIDDLE",
      CHAT_BEGIN,
      CHAT_END,
      "USER TAIL",
      "",
    ].join("\n")

    const result = setChatPrompt(raw, "new prompt")

    expect(before(result, "<!-- prompt:begin -->")).toBe(
      before(raw, "<!-- prompt:begin -->"),
    )
    expect(after(result, PROMPT_END)).toBe(after(raw, PROMPT_END))
    expect(parseChatNote(result, PATH).chat.prompt).toBe("new prompt")
  })

  it("inserts a prompt region after the frontmatter when there are no markers", () => {
    const raw = "---\nmodel: m\n---\nUSER BODY\n"

    const result = setChatPrompt(raw, "nuevo")

    expect(result.startsWith("---\nmodel: m\n---\n")).toBe(true)
    expect(result).toContain("<!-- prompt:begin -->\nnuevo\n<!-- prompt:end -->")
    expect(result.endsWith("USER BODY\n")).toBe(true)
  })

  it("inserts a prompt region at the top when there is no frontmatter", () => {
    const raw = "USER BODY\n"

    const result = setChatPrompt(raw, "nuevo")

    expect(result.startsWith("<!-- prompt:begin -->\nnuevo\n<!-- prompt:end -->")).toBe(
      true,
    )
    expect(result.endsWith(raw)).toBe(true)
  })

  // The note is the user's file and they edit it with Obsidian's own editor
  // (S2), so the markers can be duplicated or inverted by hand. Neither state
  // may make the plugin write a prompt that it then re-reads wrong.
  it("inserts a region ahead of an inverted end marker and parses its own write", () => {
    const raw = "<!-- prompt:end -->\nviejo\n"

    const result = setChatPrompt(raw, "nuevo")

    expect(parseChatNote(result, PATH).chat.prompt).toBe("nuevo")
    expect(result).toContain("viejo")
  })

  it("replaces the first of two regions and keeps the second one's bytes", () => {
    const raw = [
      "<!-- prompt:begin -->",
      "primero",
      "<!-- prompt:end -->",
      "",
      "<!-- prompt:begin -->",
      "segundo",
      "<!-- prompt:end -->",
    ].join("\n")

    const result = setChatPrompt(raw, "nuevo")

    expect(parseChatNote(result, PATH).chat.prompt).toBe("nuevo")
    expect(result).toContain("segundo")
  })
})

// ── setChatSettings ─────────────────────────────────────────────────────────

describe("setChatSettings", () => {
  it("replaces the frontmatter and leaves the body byte-identical", () => {
    const raw =
      "---\nmodel: old\ntemperature: 0.9\n---\nBODY LINE\n<!-- prompt:begin -->\np\n<!-- prompt:end -->\n<!-- chat:begin -->\n<!-- chat:end -->\n"
    const body = parseFrontmatter(raw).body

    const result = setChatSettings(raw, {
      model: "new",
      temperature: null,
      topK: null,
    })

    expect(result.startsWith("---\nmodel: new\n---\n")).toBe(true)
    expect(result.endsWith(body)).toBe(true)
    expect(parseChatNote(result, PATH).chat.settings).toEqual({
      model: "new",
      temperature: null,
      topK: null,
    })
  })

  it("adds a frontmatter block at the top when none exists", () => {
    const raw = "<!-- prompt:begin -->\nvp\n<!-- prompt:end -->\n"

    const result = setChatSettings(raw, {
      model: "m",
      temperature: null,
      topK: null,
    })

    expect(result.startsWith("---\nmodel: m\n---\n")).toBe(true)
    expect(result.endsWith(raw)).toBe(true)
  })
})

// ── attachments region ──────────────────────────────────────────────────────

/** A note whose only content is an attachments region, one line per entry. */
function noteWithAttachmentLines(lines: string[]): string {
  return [ATTACHMENTS_BEGIN, ...lines, ATTACHMENTS_END].join("\n")
}

const CORPUS: ChatAttachment = {
  kind: "corpus",
  documentId: "279fadac-07d5-442d-97dd-b9d107f97164",
  title: "DDD",
}
const VAULT: ChatAttachment = { kind: "vault", path: "Notas/Tesis.md" }

describe("parseChatNote attachments", () => {
  it("a fresh note carries an empty attachments region between prompt and chat", () => {
    const raw = createChatNote("p", SETTINGS)

    const promptEnd = raw.indexOf(PROMPT_END)
    const attachBegin = raw.indexOf(ATTACHMENTS_BEGIN)
    const attachEnd = raw.indexOf(ATTACHMENTS_END)
    const chatBegin = raw.indexOf(CHAT_BEGIN)

    expect(promptEnd).toBeGreaterThanOrEqual(0)
    expect(attachBegin).toBeGreaterThan(promptEnd)
    expect(attachEnd).toBeGreaterThan(attachBegin)
    expect(chatBegin).toBeGreaterThan(attachEnd)
    expect(parseChatNote(raw, PATH).attachments).toEqual([])
  })

  it("parses one entry per line, trims each field and keeps the written order", () => {
    const raw = noteWithAttachmentLines([
      "- corpus | 279fadac-07d5-442d-97dd-b9d107f97164 |   DDD   ",
      "- vault |   Notas/Tesis.md   ",
    ])

    expect(parseChatNote(raw, PATH).attachments).toEqual([CORPUS, VAULT])
  })

  it("reports no attachments when the region is absent", () => {
    expect(parseChatNote("Just user text\n", PATH).attachments).toEqual([])
  })

  // Tolerance is a decision, not an accident: a malformed line is ignored,
  // never an error, and the plugin drops it when it rewrites the region.
  it("ignores a line that does not start with '- '", () => {
    const raw = noteWithAttachmentLines([
      "- vault | Notas/Tesis.md",
      "vault | Notas/Otra.md",
    ])

    expect(parseChatNote(raw, PATH).attachments).toEqual([VAULT])
  })

  it("ignores a line whose kind is neither corpus nor vault", () => {
    const raw = noteWithAttachmentLines(["- recurso | id | Título"])

    expect(parseChatNote(raw, PATH).attachments).toEqual([])
  })

  it("ignores a corpus line missing its id", () => {
    const raw = noteWithAttachmentLines(["- corpus |  | DDD"])

    expect(parseChatNote(raw, PATH).attachments).toEqual([])
  })

  it("ignores a corpus line missing its title", () => {
    const raw = noteWithAttachmentLines([
      "- corpus | 279fadac-07d5-442d-97dd-b9d107f97164 |   ",
    ])

    expect(parseChatNote(raw, PATH).attachments).toEqual([])
  })

  it("ignores a vault line missing its path", () => {
    const raw = noteWithAttachmentLines(["- vault |   "])

    expect(parseChatNote(raw, PATH).attachments).toEqual([])
  })
})

describe("setAttachments", () => {
  it("replaces only what is between the markers", () => {
    const raw = [
      "USER INTRO",
      ATTACHMENTS_BEGIN,
      "- vault | viejo.md",
      ATTACHMENTS_END,
      "USER TAIL",
      "",
    ].join("\n")

    const result = setAttachments(raw, [CORPUS])

    expect(before(result, ATTACHMENTS_BEGIN)).toBe(before(raw, ATTACHMENTS_BEGIN))
    expect(after(result, ATTACHMENTS_END)).toBe(after(raw, ATTACHMENTS_END))
    expect(parseChatNote(result, PATH).attachments).toEqual([CORPUS])
  })

  it("round-trips both classes in the written order", () => {
    const result = setAttachments(createChatNote("p", SETTINGS), [CORPUS, VAULT])

    expect(parseChatNote(result, PATH).attachments).toEqual([CORPUS, VAULT])
    expect(result).toContain(
      "- corpus | 279fadac-07d5-442d-97dd-b9d107f97164 | DDD",
    )
    expect(result).toContain("- vault | Notas/Tesis.md")
  })

  it("writes an empty region", () => {
    const result = setAttachments(noteWithAttachmentLines(["- vault | x.md"]), [])

    expect(result).toContain(`${ATTACHMENTS_BEGIN}\n${ATTACHMENTS_END}`)
    expect(parseChatNote(result, PATH).attachments).toEqual([])
  })

  it("is idempotent when called twice with the same list", () => {
    const once = setAttachments(createChatNote("p", SETTINGS), [VAULT])
    const twice = setAttachments(once, [VAULT])

    expect(twice).toBe(once)
  })

  it("inserts a region before the chat region when the note has none", () => {
    const raw = [
      "USER INTRO",
      "<!-- prompt:begin -->",
      "p",
      PROMPT_END,
      "USER MIDDLE",
      CHAT_BEGIN,
      CHAT_END,
      "USER TAIL",
      "",
    ].join("\n")

    const result = setAttachments(raw, [VAULT])

    expect(result.indexOf(ATTACHMENTS_BEGIN)).toBeGreaterThan(
      result.indexOf(PROMPT_END),
    )
    expect(result.indexOf(ATTACHMENTS_BEGIN)).toBeLessThan(
      result.indexOf(CHAT_BEGIN),
    )
    // Everything from the chat marker on is the original bytes, untouched.
    expect(result.slice(result.indexOf(CHAT_BEGIN))).toBe(
      raw.slice(raw.indexOf(CHAT_BEGIN)),
    )
    expect(parseChatNote(result, PATH).chat.prompt).toBe("p")
    expect(parseChatNote(result, PATH).attachments).toEqual([VAULT])
  })

  it("appends a region at the end when the note has no chat region", () => {
    const raw = "USER TEXT\n"

    const result = setAttachments(raw, [VAULT])

    expect(result.startsWith(raw)).toBe(true)
    expect(parseChatNote(result, PATH).attachments).toEqual([VAULT])
  })

  it("drops a malformed line when it rewrites the region", () => {
    const raw = noteWithAttachmentLines([
      "- corpus | 279fadac-07d5-442d-97dd-b9d107f97164 | DDD",
      "- roto | x | y",
    ])

    const result = setAttachments(raw, [VAULT])

    expect(result).not.toContain("- roto")
    expect(parseChatNote(result, PATH).attachments).toEqual([VAULT])
  })
})
