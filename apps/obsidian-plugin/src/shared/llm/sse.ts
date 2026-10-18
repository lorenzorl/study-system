/**
 * Pure Server-Sent Events parsing for the chat-completions stream.
 *
 * Nothing here touches the network, the DOM or the vault: the transport feeds
 * decoded text in and the parser reports each complete `data:` payload.
 */
import { LlmError, extractErrorMessage } from "./errors"

export { extractErrorMessage }

const DATA_PREFIX = "data:"

function handleLine(line: string, onData: (payload: string) => void): void {
  // A blank line separates events and a `:` line is a comment / keep-alive
  // (`: OPENROUTER PROCESSING`); both contribute nothing.
  if (line === "" || line.startsWith(":")) return
  if (!line.startsWith(DATA_PREFIX)) return

  const payload = line.slice(DATA_PREFIX.length)

  // `data:` may or may not have a single space after the colon.
  onData(payload.startsWith(" ") ? payload.slice(1) : payload)
}

/**
 * Feed one decoded network chunk; call `onData` per complete `data:` payload.
 *
 * Returns the unconsumed tail, so a payload split across two chunks is buffered
 * until its newline arrives and no delta is lost.
 */
export function parseSseChunk(buffer: string, onData: (payload: string) => void): string {
  let remaining = buffer
  let newlineIndex = remaining.indexOf("\n")

  while (newlineIndex !== -1) {
    let line = remaining.slice(0, newlineIndex)

    if (line.endsWith("\r")) line = line.slice(0, -1)

    handleLine(line, onData)
    remaining = remaining.slice(newlineIndex + 1)
    newlineIndex = remaining.indexOf("\n")
  }

  return remaining
}

/**
 * The text delta inside one SSE JSON payload.
 *
 * Returns `""` when the payload carries none (for example OpenRouter's final
 * usage-only chunk). A payload that is not valid JSON throws, because skipping
 * it would silently truncate the answer.
 */
export function extractDelta(payload: string): string {
  let parsed: unknown

  try {
    parsed = JSON.parse(payload)
  } catch {
    throw new LlmError("El proveedor envió un stream malformado (JSON inválido).", 0)
  }

  if (typeof parsed !== "object" || parsed === null) return ""

  const choices = (parsed as { choices?: unknown }).choices

  if (!Array.isArray(choices) || choices.length === 0) return ""

  const delta = (choices[0] as { delta?: unknown }).delta

  if (typeof delta !== "object" || delta === null) return ""

  const content = (delta as { content?: unknown }).content

  return typeof content === "string" ? content : ""
}
