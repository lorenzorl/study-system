/**
 * Pure region surgery for a chat note. Every function is `string -> string`
 * and owns exactly one region: frontmatter, the prompt block, or the chat
 * block. Anything the user wrote outside those regions is copied byte for byte.
 */
import type {
  ChatAttachment,
  ChatNote,
  ChatNoteSettings,
  ChatTurn,
  ChatTurnRole,
} from "../model/types"
import { parseFrontmatter, serializeFrontmatter } from "./frontmatter"

export const PROMPT_BEGIN = "<!-- prompt:begin -->"
export const PROMPT_END = "<!-- prompt:end -->"
export const ATTACHMENTS_BEGIN = "<!-- adjuntos:begin -->"
export const ATTACHMENTS_END = "<!-- adjuntos:end -->"
export const CHAT_BEGIN = "<!-- chat:begin -->"
export const CHAT_END = "<!-- chat:end -->"

const USER_HEADING = "### 🧑"
const ASSISTANT_HEADING = "### 🤖"
const SOURCE_PREFIX = "Fuente: "

export function parseChatNote(raw: string, path: string): ChatNote {
  const { data } = parseFrontmatter(raw)
  const promptRegion = extractRegion(raw, PROMPT_BEGIN, PROMPT_END)
  const attachmentsRegion = extractRegion(raw, ATTACHMENTS_BEGIN, ATTACHMENTS_END)
  const chatRegion = extractRegion(raw, CHAT_BEGIN, CHAT_END)

  return {
    chat: {
      path,
      title: titleFromPath(path),
      prompt: promptRegion === null ? "" : stripRegionPadding(promptRegion),
      settings: settingsFromData(data),
    },
    turns: chatRegion === null ? [] : parseTurns(chatRegion),
    attachments: parseAttachments(attachmentsRegion),
  }
}

export function createChatNote(
  prompt: string,
  settings: ChatNoteSettings,
): string {
  const frontmatter = serializeFrontmatter(settingsToData(settings))
  return `${frontmatter}${renderPromptRegion(prompt)}\n\n${renderAttachmentsRegion([])}\n\n${CHAT_BEGIN}\n${CHAT_END}\n`
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

/**
 * Replaces the attachments region with `attachments`, writing one line per
 * entry in the given order, and copies everything outside the region byte for
 * byte.
 *
 * The plugin owns this region, exactly as it owns the prompt and chat regions:
 * a line the parser cannot read is dropped on rewrite instead of being kept as
 * an error. The note is still readable and editable by hand, and the walker
 * that reads it simply ignores what it cannot understand. When the note has no
 * region yet, one is inserted before the chat region (or at the end when there
 * is no chat region).
 */
export function setAttachments(
  raw: string,
  attachments: ChatAttachment[],
): string {
  const beginIndex = raw.indexOf(ATTACHMENTS_BEGIN)
  const endIndex =
    beginIndex === -1
      ? -1
      : raw.indexOf(ATTACHMENTS_END, beginIndex + ATTACHMENTS_BEGIN.length)

  if (beginIndex !== -1 && endIndex !== -1) {
    const inner = renderAttachmentsInner(attachments)
    return (
      raw.slice(0, beginIndex + ATTACHMENTS_BEGIN.length) + inner + raw.slice(endIndex)
    )
  }

  return insertAttachmentsRegion(raw, renderAttachmentsRegion(attachments))
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

/**
 * Turns one field into one line's worth of text before it is written into the
 * region. Nothing in an attachment is ours: a corpus title comes from the
 * server, a vault path comes from the user.
 *
 * - A line break is collapsed into a space, because a break would inject an
 *   extra line: the parser would read a line nobody wrote, and the next
 *   rewrite would no longer hold what the writer meant.
 * - Both region markers are removed, because a marker is structure and a field
 *   is data: an end marker inside a title makes `setAttachments` find the
 *   region's end early, so the rewrite truncates the region and corrupts the
 *   note around it.
 * - A pipe is left alone, because the parser splits on the first pipe for the
 *   class and on the second for a corpus id, so a pipe inside a title or a path
 *   already survives the round trip; stripping it would throw away text for
 *   nothing.
 *
 * This sanitises what the plugin writes; it does not make the parser stricter.
 * The note stays hand-editable, and a line the walker cannot read is still
 * ignored instead of reported as an error.
 *
 * The trim runs last so that removing a marker cannot leave the field with an
 * edge space of its own.
 */
function toSingleLine(field: string): string {
  return field
    .split(ATTACHMENTS_BEGIN)
    .join("")
    .split(ATTACHMENTS_END)
    .join("")
    .replace(/(?:\r\n|\r|\n)+/g, " ")
    .trim()
}

function renderAttachment(attachment: ChatAttachment): string {
  if (attachment.kind === "corpus") {
    return `- corpus | ${toSingleLine(attachment.documentId)} | ${toSingleLine(attachment.title)}`
  }
  return `- vault | ${toSingleLine(attachment.path)}`
}

/** What goes between the markers: a single newline when the list is empty. */
function renderAttachmentsInner(attachments: ChatAttachment[]): string {
  if (attachments.length === 0) return "\n"
  return `\n${attachments.map(renderAttachment).join("\n")}\n`
}

function renderAttachmentsRegion(attachments: ChatAttachment[]): string {
  return `${ATTACHMENTS_BEGIN}${renderAttachmentsInner(attachments)}${ATTACHMENTS_END}`
}

/**
 * Inserts a whole attachments region before the chat region, or at the end
 * when the note has no chat region. The original bytes stay as they are; only
 * a separator is added to keep the block readable.
 */
function insertAttachmentsRegion(raw: string, region: string): string {
  const chatIndex = raw.indexOf(CHAT_BEGIN)

  if (chatIndex === -1) {
    const separator = raw === "" ? "" : raw.endsWith("\n") ? "\n" : "\n\n"
    return `${raw}${separator}${region}\n`
  }

  const head = raw.slice(0, chatIndex)
  const prefix = head === "" || head.endsWith("\n") ? "" : "\n"
  return `${head}${prefix}${region}\n\n${raw.slice(chatIndex)}`
}

function parseAttachments(region: string | null): ChatAttachment[] {
  if (region === null) return []

  const attachments: ChatAttachment[] = []
  for (const line of region.split("\n")) {
    const parsed = parseAttachmentLine(line)
    if (parsed !== null) attachments.push(parsed)
  }

  return attachments
}

/**
 * Reads one `- clase | identidad | etiqueta` line. Anything malformed returns
 * `null` so the caller ignores it; a note is user-editable, and a typo must not
 * break the chat.
 */
function parseAttachmentLine(line: string): ChatAttachment | null {
  if (!line.startsWith("- ")) return null

  const body = line.slice(2)
  const firstPipe = body.indexOf("|")
  if (firstPipe === -1) return null

  const kind = body.slice(0, firstPipe).trim()
  const rest = body.slice(firstPipe + 1)

  if (kind === "vault") {
    const path = rest.trim()
    return path === "" ? null : { kind: "vault", path }
  }

  if (kind === "corpus") {
    const secondPipe = rest.indexOf("|")
    if (secondPipe === -1) return null

    const documentId = rest.slice(0, secondPipe).trim()
    const title = rest.slice(secondPipe + 1).trim()
    if (documentId === "" || title === "") return null

    return { kind: "corpus", documentId, title }
  }

  return null
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
