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
const UNCONSULTED_MARKER = "<!-- material: sin consultar -->"

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
      consulted: true,
    })
    raw = appendChatTurn(raw, {
      role: "assistant",
      content: "Un agregado es…",
      sources: ["Domain-Driven Design, p. 42"],
      consulted: true,
    })

    expect(parseChatNote(raw, PATH).turns).toEqual([
      { role: "user", content: "¿Qué es un agregado?", sources: [], consulted: true },
      {
        role: "assistant",
        content: "Un agregado es…",
        sources: ["Domain-Driven Design, p. 42"],
        consulted: true,
      },
    ])
  })

  it("keeps the note's rendering out of the source data", () => {
    let raw = createChatNote("p", SETTINGS)
    raw = appendChatTurn(raw, {
      role: "assistant",
      content: "A",
      sources: ["Fuente: DDD, p. 1"],
      consulted: true,
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
      consulted: true,
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
      consulted: true,
    })

    expect(result.startsWith(raw)).toBe(true)
    expect(parseChatNote(result, PATH).turns).toEqual([
      { role: "user", content: "Hola", sources: [], consulted: true },
    ])
  })

  it("parses a turn whose content contains a heading or a comment-like string", () => {
    const content = "# Título\n\nTexto con <!-- comentario --> adentro."
    let raw = createChatNote("p", SETTINGS)
    raw = appendChatTurn(raw, { role: "assistant", content, sources: [], consulted: true })

    const turns = parseChatNote(raw, PATH).turns

    expect(turns).toHaveLength(1)
    expect(turns[0].content).toBe(content)
    expect(turns[0].sources).toEqual([])
  })
})

// ── appendChatTurn: untrusted text in a structural position ──────────────────

// A source line is built from data this plugin does not control: a document
// title comes from the server, a vault path and a citation come from the user.
// The writer is where that text reaches a structural position, so it is
// sanitised here, exactly as an attachment field is.
describe("appendChatTurn sanitises each source line", () => {
  it("renders a source with a line break as one Fuente line", () => {
    let raw = createChatNote("p", SETTINGS)
    raw = appendChatTurn(raw, {
      role: "assistant",
      content: "A",
      sources: ["DDD\np. 42"],
      consulted: true,
    })

    const fuenteLines = raw
      .split("\n")
      .filter((line) => line.startsWith("Fuente:"))

    expect(fuenteLines).toEqual(["Fuente: DDD p. 42"])
    expect(parseChatNote(raw, PATH).turns).toEqual([
      { role: "assistant", content: "A", sources: ["DDD p. 42"], consulted: true },
    ])
  })

  it("cannot be ended early by a chat marker embedded in a source", () => {
    const raw = [
      "USER INTRO",
      CHAT_BEGIN,
      "### 🧑 hola",
      CHAT_END,
      "USER TAIL",
      "",
    ].join("\n")

    const result = appendChatTurn(raw, {
      role: "assistant",
      content: "A",
      sources: ["DDD<!-- chat:end --> truco"],
      consulted: true,
    })

    // The only end marker left is the one the writer closed: everything after
    // it is still the user's bytes.
    expect(result.split(CHAT_END)).toHaveLength(2)
    expect(after(result, CHAT_END)).toBe(after(raw, CHAT_END))
    expect(parseChatNote(result, PATH).turns).toEqual([
      { role: "user", content: "hola", sources: [], consulted: true },
      { role: "assistant", content: "A", sources: ["DDD truco"], consulted: true },
    ])
  })

  it("strips a marker of any region from a source", () => {
    const hostile: [string, string][] = [
      ["<!-- prompt:begin -->p<!-- prompt:end -->", "p"],
      ["<!-- adjuntos:begin -->a<!-- adjuntos:end -->", "a"],
      ["<!-- chat:begin -->c<!-- chat:end -->", "c"],
    ]

    for (const [source, expected] of hostile) {
      let raw = createChatNote("n", SETTINGS)
      raw = appendChatTurn(raw, {
        role: "assistant",
        content: "A",
        sources: [source],
        consulted: true,
      })

      expect(raw.split(CHAT_END)).toHaveLength(2)
      expect(parseChatNote(raw, PATH).turns[0].sources).toEqual([expected])
    }
  })

  // Preserve: a source with nothing to sanitise is the common case, and it must
  // still be written byte for byte as before.
  it("writes a source with nothing to sanitise exactly as before", () => {
    let raw = createChatNote("p", SETTINGS)
    raw = appendChatTurn(raw, {
      role: "assistant",
      content: "A",
      sources: ["Domain-Driven Design, p. 42", "Fuente: Notas/Tesis.md"],
      consulted: true,
    })

    expect(raw).toContain(
      `${CHAT_BEGIN}\n### 🤖 A\n\nFuente: Domain-Driven Design, p. 42\nFuente: Notas/Tesis.md\n${CHAT_END}`,
    )
    expect(parseChatNote(raw, PATH).turns[0].sources).toEqual([
      "Domain-Driven Design, p. 42",
      "Notas/Tesis.md",
    ])
  })
})

// ── consulted: the third state of the frame ──────────────────────────────────

// `consulted` has three readings in one field: there was nothing to consult,
// the material was read, or the material was behind the tool and no search ran.
// The note records only the third one, because it is the only one that is a
// claim about the answer. The other two write nothing: an absent marker claims
// nothing, so older notes never show the badge.
describe("the consulted flag in the note", () => {
  it("renders the marker as the assistant turn's last line when consulted is false", () => {
    let raw = createChatNote("p", SETTINGS)
    raw = appendChatTurn(raw, {
      role: "assistant",
      content: "No consulté el material, te respondo de memoria.",
      sources: [],
      consulted: false,
    })

    expect(raw).toContain(
      `${CHAT_BEGIN}\n### 🤖 No consulté el material, te respondo de memoria.\n${UNCONSULTED_MARKER}\n${CHAT_END}`,
    )
  })

  it("keeps consulted false and the content through parse → render → parse", () => {
    const original = [
      CHAT_BEGIN,
      "### 🤖 No pude consultar el material.",
      "",
      "Fuente: DDD, p. 42",
      UNCONSULTED_MARKER,
      CHAT_END,
    ].join("\n")
    const parsed = parseChatNote(original, PATH).turns[0]

    const rendered = appendChatTurn(createChatNote("p", SETTINGS), parsed)
    const again = parseChatNote(rendered, PATH).turns[0]

    expect(parsed.consulted).toBe(false)
    expect(again.consulted).toBe(false)
    expect(again.content).toBe("No pude consultar el material.")
    expect(again.sources).toEqual(["DDD, p. 42"])
  })

  // A chat can have a vault file in the message and a corpus document behind the
  // tool, and the model may answer without searching. That turn has sources and
  // `consulted: false`, which only parses right because the marker is popped
  // before the source lines are read.
  it("parses a turn that has both sources and the marker into both of them", () => {
    const raw = [
      CHAT_BEGIN,
      "### 🤖 A",
      "",
      "Fuente: DDD, p. 42",
      "Fuente: Notas/Tesis.md",
      UNCONSULTED_MARKER,
      CHAT_END,
    ].join("\n")

    expect(parseChatNote(raw, PATH).turns).toEqual([
      {
        role: "assistant",
        content: "A",
        sources: ["DDD, p. 42", "Notas/Tesis.md"],
        consulted: false,
      },
    ])
  })

  it("writes nothing for a consulted turn, so the marker never appears", () => {
    let raw = createChatNote("p", SETTINGS)
    raw = appendChatTurn(raw, {
      role: "assistant",
      content: "Un agregado es…",
      sources: ["Domain-Driven Design, p. 42"],
      consulted: true,
    })

    expect(raw).not.toContain(UNCONSULTED_MARKER)
    expect(raw).not.toContain("material: sin consultar")
    expect(parseChatNote(raw, PATH).turns[0].consulted).toBe(true)
  })

  // Hand-written notes and notes written before this field existed carry no
  // marker, and an absent marker claims nothing.
  it("reads a note with no marker as consulted true", () => {
    const raw = [
      CHAT_BEGIN,
      "### 🧑 hola",
      "### 🤖 A",
      "",
      "Fuente: DDD, p. 1",
      CHAT_END,
    ].join("\n")

    expect(parseChatNote(raw, PATH).turns).toEqual([
      { role: "user", content: "hola", sources: [], consulted: true },
      { role: "assistant", content: "A", sources: ["DDD, p. 1"], consulted: true },
    ])
  })

  // An accepted collision, pinned so it cannot change by accident. The marker is
  // a data channel in a hand-editable note, exactly like the `Fuente: ` prefix and
  // the region markers: an answer whose own last line is the marker loses that
  // line and reads as unconsulted. What is lost is an HTML comment, invisible in
  // the reading view, and the misread errs toward warning instead of silence.
  it("reads an answer whose last line is the marker as the marker", () => {
    const raw = [
      CHAT_BEGIN,
      "### 🤖 A",
      UNCONSULTED_MARKER,
      CHAT_END,
    ].join("\n")

    expect(parseChatNote(raw, PATH).turns).toEqual([
      { role: "assistant", content: "A", sources: [], consulted: false },
    ])
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

/** The attachment lines themselves: between the markers, blanks dropped. */
function attachmentLines(raw: string): string[] {
  const begin = raw.indexOf(ATTACHMENTS_BEGIN) + ATTACHMENTS_BEGIN.length
  const end = raw.indexOf(ATTACHMENTS_END)
  return raw
    .slice(begin, end)
    .split("\n")
    .filter((line) => line.trim() !== "")
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

// ── setAttachments: untrusted text in a structural position ──────────────────

// A corpus title comes from the server and a vault path comes from the user.
// Neither is ours to trust with a newline or with a marker, because both would
// change the shape of the note instead of the text inside it.
describe("setAttachments sanitises each field into one line", () => {
  it("renders a corpus title with a run of line breaks as one line", () => {
    const result = setAttachments(noteWithAttachmentLines([]), [
      { kind: "corpus", documentId: "id-1", title: "DDD\n\n\r\ntomo 1" },
    ])

    expect(attachmentLines(result)).toEqual(["- corpus | id-1 | DDD tomo 1"])
    expect(parseChatNote(result, PATH).attachments).toEqual([
      { kind: "corpus", documentId: "id-1", title: "DDD tomo 1" },
    ])
  })

  it("renders a vault path with a windows line break as one line", () => {
    const result = setAttachments(noteWithAttachmentLines([]), [
      { kind: "vault", path: "Notas/Tesis\r\nv2.md" },
    ])

    expect(attachmentLines(result)).toEqual(["- vault | Notas/Tesis v2.md"])
    expect(parseChatNote(result, PATH).attachments).toEqual([
      { kind: "vault", path: "Notas/Tesis v2.md" },
    ])
  })

  it("cannot be ended early by an end marker embedded in a field", () => {
    const hostile: ChatAttachment = {
      kind: "corpus",
      documentId: "id-1",
      title: "DDD<!-- adjuntos:end --> truco",
    }
    const raw = [
      "USER INTRO",
      ATTACHMENTS_BEGIN,
      "- vault | viejo.md",
      ATTACHMENTS_END,
      "USER TAIL",
      "",
    ].join("\n")

    const once = setAttachments(raw, [hostile])

    // One end marker is left, and it is the one the writer closed: everything
    // after it is still the user's bytes.
    expect(once.split(ATTACHMENTS_END)).toHaveLength(2)
    expect(after(once, ATTACHMENTS_END)).toBe(after(raw, ATTACHMENTS_END))
    expect(parseChatNote(once, PATH).attachments).toEqual([
      { kind: "corpus", documentId: "id-1", title: "DDD truco" },
    ])

    // A second rewrite must not find an injected marker and truncate there.
    const twice = setAttachments(once, [VAULT])

    expect(twice.split(ATTACHMENTS_END)).toHaveLength(2)
    expect(after(twice, ATTACHMENTS_END)).toBe(after(raw, ATTACHMENTS_END))
    expect(parseChatNote(twice, PATH).attachments).toEqual([VAULT])
  })

  // One removal pass is not enough: removing a marker can rebuild one. The
  // field below loses its inner end marker, the halves around it join, and what
  // is left is the very marker the sanitiser exists to remove. A field is data
  // and a marker is structure, so the removal runs until no marker is left,
  // however the input is arranged.
  it("cannot be rebuilt into a marker by removing a nested marker", () => {
    const exactNested = "<!-- adjuntos:<!-- adjuntos:end -->end -->"
    const arrangements = [
      exactNested,
      `DDD ${exactNested} truco`,
      `${exactNested}${exactNested}`,
      "<!-- adjuntos:be<!-- adjuntos:begin -->gin -->",
    ]

    for (const title of arrangements) {
      const result = setAttachments(noteWithAttachmentLines([]), [
        { kind: "corpus", documentId: "id-1", title },
      ])

      const lines = attachmentLines(result)
      expect(lines).toHaveLength(1)
      expect(lines[0]).not.toContain("adjuntos:")
      // The only end marker left is the one the writer closed, and a second
      // rewrite therefore finds the real end of the region.
      expect(result.split(ATTACHMENTS_END)).toHaveLength(2)

      const twice = setAttachments(result, [VAULT])
      expect(twice.split(ATTACHMENTS_END)).toHaveLength(2)
      expect(parseChatNote(twice, PATH).attachments).toEqual([VAULT])
    }
  })

  it("cannot be rebuilt into a marker inside a vault path either", () => {
    const result = setAttachments(noteWithAttachmentLines([]), [
      {
        kind: "vault",
        path: "Notas/<!-- adjuntos:<!-- adjuntos:end -->end -->Tesis.md",
      },
    ])

    expect(attachmentLines(result)).toEqual(["- vault | Notas/Tesis.md"])
    expect(parseChatNote(result, PATH).attachments).toEqual([
      { kind: "vault", path: "Notas/Tesis.md" },
    ])
  })

  // A pipe is data, not structure: the parser splits on the first pipe for the
  // class and on the second for a corpus id, so a pipe survives the round trip
  // and stripping it would throw away text for nothing.
  it("keeps a pipe inside a title", () => {
    const titled: ChatAttachment = {
      kind: "corpus",
      documentId: "id-1",
      title: "DDD | tomo 1",
    }

    const result = setAttachments(noteWithAttachmentLines([]), [titled])

    expect(attachmentLines(result)).toEqual(["- corpus | id-1 | DDD | tomo 1"])
    expect(parseChatNote(result, PATH).attachments).toEqual([titled])
  })

  it("renders the same bytes after a parse round trip", () => {
    const first = setAttachments(noteWithAttachmentLines([]), [
      { kind: "corpus", documentId: "id-1", title: "DDD\r\ntomo 1" },
      { kind: "vault", path: "Notas/Tesis\nv2.md" },
    ])

    const again = setAttachments(first, parseChatNote(first, PATH).attachments)

    expect(again).toBe(first)
  })

  // Preserve: the render already trimmed each field, and a field with no break
  // and no marker must still be written exactly as it was before.
  it("still trims the space around a field", () => {
    const result = setAttachments(noteWithAttachmentLines([]), [
      { kind: "corpus", documentId: "  id-1  ", title: "  DDD  " },
      { kind: "vault", path: "  Notas/Tesis.md  " },
    ])

    expect(attachmentLines(result)).toEqual([
      "- corpus | id-1 | DDD",
      "- vault | Notas/Tesis.md",
    ])
  })
})
