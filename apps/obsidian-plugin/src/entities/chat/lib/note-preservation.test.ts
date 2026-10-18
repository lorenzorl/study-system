/**
 * The invariants that protect what the user wrote in a chat note.
 *
 * A chat note lives in the user's vault and the user edits it too, so the
 * plugin's writes are confined by contract (odd/tasks/chat-workspaces.md): the
 * frontmatter, the prompt region and the chat region are the only bytes it
 * owns. These cases are the ones that broke that contract before: an existing
 * note being replaced by a fresh one, and a frontmatter field the plugin does
 * not own being dropped while the ones it does own were rewritten.
 */
import { describe, expect, it } from "vitest"
import type { ChatNoteSettings } from "../model/types"
import {
  mergeIntoExistingNote,
  setChatSettings,
  uniqueChatPath,
} from "./note"

const FOLDER = "Study Dashboard/Chats"
const NO_OVERRIDES: ChatNoteSettings = {
  model: null,
  temperature: null,
  topK: null,
}

describe("setChatSettings", () => {
  it("keeps the frontmatter fields it does not own", () => {
    const raw = "---\ntags: [ddd]\ntitle: Mi nota\n---\n\nUSER TEXT\n"

    const result = setChatSettings(raw, {
      model: "qwen/qwen3.5-flash-02-23",
      temperature: 0.5,
      topK: 8,
    })

    expect(result).toContain("tags: [ddd]")
    expect(result).toContain("title: Mi nota")
    expect(result).toContain("model: qwen/qwen3.5-flash-02-23")
    expect(result).toContain("USER TEXT")
  })

  it("drops only an override the chat no longer sets", () => {
    const raw = "---\ntags: x\nmodel: previous\n---\n\nbody\n"

    const result = setChatSettings(raw, NO_OVERRIDES)

    expect(result).not.toContain("model:")
    expect(result).toContain("tags: x")
    expect(result).toContain("body")
  })
})

describe("mergeIntoExistingNote", () => {
  const EXISTING = [
    "---",
    "tags: [ddd]",
    "---",
    "MY OWN TEXT",
    "<!-- prompt:begin -->",
    "mi prompt",
    "<!-- prompt:end -->",
    "",
    "<!-- chat:begin -->",
    "### 🧑 hola",
    "",
    "### 🤖 chau",
    "<!-- chat:end -->",
    "",
    "TRAILING TEXT",
    "",
  ].join("\n")

  it("keeps the existing prompt, the transcript and the user's prose", () => {
    const result = mergeIntoExistingNote(EXISTING, "prompt nuevo", {
      model: "m",
      temperature: null,
      topK: 3,
    })

    expect(result).toContain("MY OWN TEXT")
    expect(result).toContain("TRAILING TEXT")
    expect(result).toContain("tags: [ddd]")
    expect(result).toContain("### 🧑 hola")
    expect(result).toContain("### 🤖 chau")
    expect(result).toContain("mi prompt")
    expect(result).not.toContain("prompt nuevo")
    expect(result).toContain("model: m")
  })

  it("adds the prompt when the existing note has none", () => {
    const result = mergeIntoExistingNote("USER TEXT\n", "prompt nuevo", NO_OVERRIDES)

    expect(result).toContain("USER TEXT")
    expect(result).toContain("prompt nuevo")
    expect(result).toContain("<!-- chat:begin -->")
  })
})

describe("uniqueChatPath", () => {
  it("uses the slug when it is free", () => {
    expect(uniqueChatPath([], FOLDER, "DDD")).toBe(`${FOLDER}/ddd.md`)
  })

  it("counts up while the path is taken", () => {
    expect(uniqueChatPath([`${FOLDER}/chat.md`], FOLDER, "Chat")).toBe(
      `${FOLDER}/chat-2.md`,
    )
    expect(
      uniqueChatPath([`${FOLDER}/chat.md`, `${FOLDER}/chat-2.md`], FOLDER, "Chat"),
    ).toBe(`${FOLDER}/chat-3.md`)
  })

  it("ignores paths outside the chats folder", () => {
    expect(uniqueChatPath(["Study Dashboard/otro/chat.md"], FOLDER, "Chat")).toBe(
      `${FOLDER}/chat.md`,
    )
  })
})
