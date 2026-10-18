/**
 * Pure frontmatter helpers. No I/O: they only move strings.
 *
 * Only a block that opens the text with `---` on its own line counts as
 * frontmatter. Values stay strings on purpose — the note's `0.2` is read and
 * written back verbatim so formatting never drifts.
 */

export interface Frontmatter {
  data: Record<string, string>
  body: string
}

const DELIMITER = "---"

/** Keys always emitted in this order so the same settings produce the same bytes. */
const KEY_ORDER = ["model", "temperature", "top_k"] as const

export function parseFrontmatter(raw: string): Frontmatter {
  if (!raw.startsWith(DELIMITER)) {
    return { data: {}, body: raw }
  }

  const afterOpen = raw.slice(DELIMITER.length)
  if (!afterOpen.startsWith("\n") && !afterOpen.startsWith("\r\n")) {
    return { data: {}, body: raw }
  }

  const firstBreakLength = afterOpen.startsWith("\r\n") ? 2 : 1
  const headerStart = DELIMITER.length + firstBreakLength

  let cursor = headerStart
  let closeStart = -1
  let bodyStart = -1

  while (cursor <= raw.length) {
    const newline = raw.indexOf("\n", cursor)
    const lineEnd = newline === -1 ? raw.length : newline
    const line = raw.slice(cursor, lineEnd).replace(/\r$/, "")

    if (line.trim() === DELIMITER) {
      closeStart = cursor
      bodyStart = newline === -1 ? raw.length : newline + 1
      break
    }

    if (newline === -1) break
    cursor = newline + 1
  }

  // A block with no closing `---` is not frontmatter.
  if (closeStart === -1) {
    return { data: {}, body: raw }
  }

  return {
    data: parseHeader(raw.slice(headerStart, closeStart)),
    body: raw.slice(bodyStart),
  }
}

export function serializeFrontmatter(
  data: Record<string, string | number | null | undefined>,
): string {
  const lines = orderedKeys(data).map((key) => `${key}: ${String(data[key])}`)
  const inner = lines.length > 0 ? `${lines.join("\n")}\n` : ""
  return `${DELIMITER}\n${inner}${DELIMITER}\n`
}

function parseHeader(header: string): Record<string, string> {
  const data: Record<string, string> = {}

  for (const rawLine of header.split("\n")) {
    const line = rawLine.replace(/\r$/, "").trim()
    if (line === "" || line.startsWith("#")) continue

    const separator = line.indexOf(":")
    if (separator === -1) continue

    const key = line.slice(0, separator).trim()
    if (key === "") continue

    data[key] = line.slice(separator + 1).trim()
  }

  return data
}

function orderedKeys(
  data: Record<string, string | number | null | undefined>,
): string[] {
  const known = KEY_ORDER.filter((key) => hasValue(data[key]))
  const extra = Object.keys(data)
    .filter(
      (key) =>
        !(KEY_ORDER as readonly string[]).includes(key) && hasValue(data[key]),
    )
    .sort((a, b) => (a < b ? -1 : a > b ? 1 : 0))

  return [...known, ...extra]
}

function hasValue(value: string | number | null | undefined): boolean {
  return value !== null && value !== undefined && value !== ""
}
