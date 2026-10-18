/**
 * Pure region surgery for a chat note. Every function is `string -> string`
 * and owns exactly one region: frontmatter, the prompt block, or the chat
 * block. Anything the user wrote outside those regions is copied byte for byte.
 */
import type {
  ChatNote,
  ChatNoteSettings,
  ChatTurn,
  ChatTurnRole,
} from "../model/types"
import { parseFrontmatter, serializeFrontmatter } from "./frontmatter"

export const PROMPT_BEGIN = "<!-- prompt:begin -->"
export const PROMPT_END = "<!-- prompt:end -->"
export const CHAT_BEGIN = "<!-- chat:begin -->"
export const CHAT_END = "<!-- chat:end -->"

const USER_HEADING = "### 🧑"
const ASSISTANT_HEADING = "### 🤖"
const SOURCE_PREFIX = "Fuente: "

export function parseChatNote(raw: string, path: string): ChatNote {
  const { data } = parseFrontmatter(raw)
  const promptRegion = extractRegion(raw, PROMPT_BEGIN, PROMPT_END)
  const chatRegion = extractRegion(raw, CHAT_BEGIN, CHAT_END)

  return {
    chat: {
      path,
      title: titleFromPath(path),
      prompt: promptRegion === null ? "" : stripRegionPadding(promptRegion),
      settings: settingsFromData(data),
    },
    turns: chatRegion === null ? [] : parseTurns(chatRegion),
  }
}

export function createChatNote(
  prompt: string,
  settings: ChatNoteSettings,
): string {
  const frontmatter = serializeFrontmatter(settingsToData(settings))
  return `${frontmatter}${renderPromptRegion(prompt)}\n\n${CHAT_BEGIN}\n${CHAT_END}\n`
}

/** Replaces only what is between the prompt markers; inserts a region if absent. */
export function setChatPrompt(raw: string, prompt: string): string {
  const beginIndex = raw.indexOf(PROMPT_BEGIN)
  const endIndex =
    beginIndex === -1
      ? -1
      : raw.indexOf(PROMPT_END, beginIndex + PROMPT_BEGIN.length)

  if (beginIndex !== -1 && endIndex !== -1) {
    const inner = prompt === "" ? "\n" : `\n${prompt}\n`
    return (
      raw.slice(0, beginIndex + PROMPT_BEGIN.length) + inner + raw.slice(endIndex)
    )
  }

  return insertAfterFrontmatter(raw, renderPromptRegion(prompt))
}

/**
 * Replaces only the three keys the plugin owns, inside the frontmatter block,
 * adding a block at the top when the note has none.
 *
 * Every other key the user put there is kept: a chat note is still a note, and
 * `tags:` or `title:` are theirs. The plugin writes only `model`,
 * `temperature` and `top_k`, which is what makes clearing an override exact
 * instead of a guess about which line it was.
 */
export function setChatSettings(raw: string, settings: ChatNoteSettings): string {
  const { data, body } = parseFrontmatter(raw)
  const merged = { ...data, ...settingsToData(settings) }

  // A note with no frontmatter and no overrides to write gains nothing from an
  // empty block, so it is left exactly as it was.
  if (Object.keys(data).length === 0 && !hasSerializableValue(merged)) {
    return raw
  }

  return serializeFrontmatter(merged) + body
}

/** Inserts a turn before `<!-- chat:end -->`, or appends a whole region. */
export function appendChatTurn(raw: string, turn: ChatTurn): string {
  const rendered = renderTurn(turn)
  const beginIndex = raw.indexOf(CHAT_BEGIN)
  const endIndex =
    beginIndex === -1
      ? -1
      : raw.indexOf(CHAT_END, beginIndex + CHAT_BEGIN.length)

  if (beginIndex === -1 || endIndex === -1) {
    return appendChatRegion(raw, rendered)
  }

  const before = raw.slice(0, endIndex)
  const region = raw.slice(beginIndex + CHAT_BEGIN.length, endIndex)
  let insertion = ""
  if (!before.endsWith("\n")) insertion += "\n"
  if (region.trim() !== "") insertion += "\n"
  insertion += `${rendered}\n`

  return before + insertion + raw.slice(endIndex)
}

/** A filesystem-safe slug for a chat title, used to name the note. */
export function slugifyChatTitle(title: string): string {
  const slug = title
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")

  return slug === "" ? "chat" : slug
}

/**
 * Fold a chat's prompt and overrides into a note that already exists, without
 * replacing any of it.
 *
 * This is the second line of defence: `createChatForTitle` in the slice's api
 * picks a free path first. It exists because a create that lands on an existing
 * path would otherwise consume a note the user owns, and losing a transcript to
 * two clicks on "Nuevo chat" is not a failure a reader could detect afterwards.
 *
 * The prompt is added only when the note has no prompt region of its own: an
 * existing prompt is the user's, and the note already holds a conversation that
 * the plugin did not start.
 */
export function mergeIntoExistingNote(
  raw: string,
  prompt: string,
  settings: ChatNoteSettings,
): string {
  const withSettings = setChatSettings(raw, settings)
  const withPrompt = hasRegion(withSettings, PROMPT_BEGIN, PROMPT_END)
    ? withSettings
    : setChatPrompt(withSettings, prompt)

  return ensureChatRegion(withPrompt)
}

/**
 * The path a new chat titled `title` should take inside `folder`.
 *
 * The title is the user's and may repeat; the path is ours and may not. A taken
 * name gets a counter, so a second "Nuevo chat" makes a second chat instead of
 * replacing the first one's note.
 */
export function uniqueChatPath(
  existingPaths: string[],
  folder: string,
  title: string,
): string {
  const slug = slugifyChatTitle(title)
  const taken = new Set(existingPaths)

  let candidate = `${folder}/${slug}.md`
  let counter = 2

  while (taken.has(candidate)) {
    candidate = `${folder}/${slug}-${counter}.md`
    counter += 1
  }

  return candidate
}

// ── internals ───────────────────────────────────────────────────────────────

function hasRegion(raw: string, begin: string, end: string): boolean {
  return extractRegion(raw, begin, end) !== null
}

/** Adds an empty chat region at the end when the note has none. */
function ensureChatRegion(raw: string): string {
  return hasRegion(raw, CHAT_BEGIN, CHAT_END) ? raw : appendChatRegion(raw, "")
}

function hasSerializableValue(
  data: Record<string, string | number | null | undefined>,
): boolean {
  return Object.values(data).some(
    (value) => value !== null && value !== undefined && value !== "",
  )
}

function extractRegion(
  raw: string,
  begin: string,
  end: string,
): string | null {
  const beginIndex = raw.indexOf(begin)
  if (beginIndex === -1) return null

  const endIndex = raw.indexOf(end, beginIndex + begin.length)
  if (endIndex === -1) return null

  return raw.slice(beginIndex + begin.length, endIndex)
}

function stripRegionPadding(region: string): string {
  let result = region
  if (result.startsWith("\r\n")) result = result.slice(2)
  else if (result.startsWith("\n")) result = result.slice(1)

  if (result.endsWith("\r\n")) result = result.slice(0, -2)
  else if (result.endsWith("\n")) result = result.slice(0, -1)

  return result
}

function renderPromptRegion(prompt: string): string {
  if (prompt === "") {
    return `${PROMPT_BEGIN}\n${PROMPT_END}`
  }
  return `${PROMPT_BEGIN}\n${prompt}\n${PROMPT_END}`
}

function insertAfterFrontmatter(raw: string, region: string): string {
  const { body } = parseFrontmatter(raw)
  const insertAt = raw.length - body.length
  const head = raw.slice(0, insertAt)
  const tail = raw.slice(insertAt)
  const prefix = head === "" || head.endsWith("\n") ? "" : "\n"

  if (tail === "") {
    return head + prefix + region
  }
  return `${head}${prefix}${region}\n\n${tail}`
}

function renderTurn(turn: ChatTurn): string {
  const heading = turn.role === "user" ? USER_HEADING : ASSISTANT_HEADING
  const lines = [`${heading} ${turn.content}`]

  if (turn.role === "assistant" && turn.sources.length > 0) {
    lines.push("")
    for (const source of turn.sources) {
      lines.push(renderSource(source))
    }
  }

  return lines.join("\n")
}

function renderSource(source: string): string {
  const trimmed = source.trim()
  return trimmed.startsWith(SOURCE_PREFIX) ? trimmed : `${SOURCE_PREFIX}${trimmed}`
}

function appendChatRegion(raw: string, rendered: string): string {
  const prefix = raw === "" ? "" : raw.endsWith("\n") ? "\n" : "\n\n"
  return `${raw}${prefix}${CHAT_BEGIN}\n${rendered}\n${CHAT_END}\n`
}

function parseTurns(region: string): ChatTurn[] {
  const turns: ChatTurn[] = []
  let role: ChatTurnRole | null = null
  let lines: string[] = []

  const flush = (): void => {
    if (role === null) return
    turns.push(buildTurn(role, lines))
    role = null
    lines = []
  }

  for (const line of region.split("\n")) {
    const detected = detectRole(line)
    if (detected !== null) {
      flush()
      role = detected
      lines = [contentAfterHeading(line, detected)]
      continue
    }
    if (role !== null) {
      lines.push(line)
    }
  }
  flush()

  return turns
}

function detectRole(line: string): ChatTurnRole | null {
  if (line.startsWith(USER_HEADING)) return "user"
  if (line.startsWith(ASSISTANT_HEADING)) return "assistant"
  return null
}

function contentAfterHeading(line: string, role: ChatTurnRole): string {
  const heading = role === "user" ? USER_HEADING : ASSISTANT_HEADING
  const rest = line.slice(heading.length)
  return rest.startsWith(" ") ? rest.slice(1) : rest
}

function buildTurn(role: ChatTurnRole, rawLines: string[]): ChatTurn {
  const lines = [...rawLines]
  const sources: string[] = []

  // The region's own trailing newline shows up as an empty final line; drop it
  // before the trailing source lines can be recognized.
  while (lines.length > 0 && lines[lines.length - 1].trim() === "") {
    lines.pop()
  }

  if (role === "assistant") {
    while (lines.length > 0 && lines[lines.length - 1].startsWith(SOURCE_PREFIX)) {
      // The prefix is presentation, so it belongs to the note and not to the
      // data: a source read back here is what a caller would have passed in.
      sources.unshift((lines.pop() as string).slice(SOURCE_PREFIX.length))
    }

    while (lines.length > 0 && lines[lines.length - 1].trim() === "") {
      lines.pop()
    }
  }

  return { role, content: lines.join("\n"), sources }
}

function settingsFromData(data: Record<string, string>): ChatNoteSettings {
  return {
    model: optionalString(data.model),
    temperature: optionalNumber(data.temperature),
    topK: optionalNumber(data.top_k),
  }
}

function settingsToData(
  settings: ChatNoteSettings,
): Record<string, string | number | null> {
  return {
    model: settings.model,
    temperature: settings.temperature,
    top_k: settings.topK,
  }
}

function optionalString(value: string | undefined): string | null {
  const trimmed = value?.trim() ?? ""
  return trimmed === "" ? null : trimmed
}

function optionalNumber(value: string | undefined): number | null {
  const trimmed = value?.trim() ?? ""
  if (trimmed === "") return null

  const parsed = Number(trimmed)
  return Number.isFinite(parsed) ? parsed : null
}

function titleFromPath(path: string): string {
  const filename = path.split("/").pop() ?? path
  return filename.replace(/\.md$/, "")
}
