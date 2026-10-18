/**
 * The pure prompt builder for a chat answer.
 *
 * No I/O: given a chat's own prompt, its previous turns, the passages retrieval
 * returned and the question, it produces the messages the provider receives.
 * The citation rules live here, in code, so a chat's prompt (S1: a prompt that
 * works for that chat only) can never replace them — it is appended inside the
 * frame, never substituted for it.
 */
import type { LlmMessage } from "@/shared/llm"

export interface PromptPassage {
  title: string
  page: number
  text: string
}

export interface PromptTurn {
  role: "user" | "assistant"
  content: string
}

export interface PromptInput {
  /** The chat's own system prompt, read from its note. */
  chatPrompt: string
  /** The note's previous turns, oldest first. */
  history: PromptTurn[]
  /** What retrieval returned. */
  passages: PromptPassage[]
  question: string
}

/**
 * The rules the model always receives, in Spanish because the user's questions
 * are Spanish. They are deliberately not editable by a chat's prompt.
 */
export const CITATION_FRAME = [
  "Respondé la pregunta usando solamente el contexto que aparece a continuación.",
  "Si el contexto no alcanza para responderla, decilo explícitamente; no respondas con tu propio conocimiento.",
  "No inventes títulos, páginas ni datos: usá únicamente lo que figura en el contexto.",
  "Cerrá la respuesta con una línea por cada pasaje que hayas usado, con este formato exacto: Fuente: <título>, p. <página>.",
  "Respondé en el idioma de la pregunta.",
].join("\n")

/** Said plainly when retrieval returned nothing, so the frame has something to act on. */
const EMPTY_CORPUS_MESSAGE =
  "El corpus no devolvió ningún pasaje para esta pregunta."

/**
 * The frame followed by the chat's prompt. An empty prompt leaves the frame
 * alone, with no trailing separator.
 */
export function buildSystemMessage(chatPrompt: string): string {
  const trimmed = chatPrompt.trim()

  return trimmed === "" ? CITATION_FRAME : `${CITATION_FRAME}\n\n${trimmed}`
}

/**
 * The last `maxTurns` turns, without a leading assistant turn: a provider
 * reads a leading assistant message as a pre-filled answer.
 */
export function trimHistory(turns: PromptTurn[], maxTurns: number): PromptTurn[] {
  if (maxTurns <= 0) return []

  const kept = turns.slice(-maxTurns)
  let start = 0

  while (start < kept.length && kept[start].role === "assistant") {
    start += 1
  }

  return kept.slice(start)
}

/** One numbered block per passage: index, title, exact page, then its text. */
export function buildContext(passages: PromptPassage[]): string {
  return passages.map(renderPassage).join("\n\n")
}

/**
 * The citation data in the note's own shape, deduplicated in passage order.
 * A citation is plain data (`Domain-Driven Design, p. 42`); the note adds the
 * `Fuente: ` prefix when it renders.
 */
export function buildSources(passages: PromptPassage[]): string[] {
  const seen = new Set<string>()
  const sources: string[] = []

  for (const passage of passages) {
    const source = `${passage.title}, p. ${passage.page}`

    if (seen.has(source)) continue

    seen.add(source)
    sources.push(source)
  }

  return sources
}

/**
 * `[system, ...trimmed history, user]`; the last message always carries the
 * context block and the question.
 */
export function buildMessages(input: PromptInput & { maxTurns: number }): LlmMessage[] {
  const system: LlmMessage = {
    role: "system",
    content: buildSystemMessage(input.chatPrompt),
  }
  const history: LlmMessage[] = trimHistory(input.history, input.maxTurns).map(
    (turn) => ({ role: turn.role, content: turn.content }),
  )
  const user: LlmMessage = {
    role: "user",
    content: buildUserMessage(input.passages, input.question),
  }

  return [system, ...history, user]
}

function renderPassage(passage: PromptPassage, index: number): string {
  return `[${index + 1}] ${passage.title} — p. ${passage.page}\n${passage.text}`
}

function buildUserMessage(passages: PromptPassage[], question: string): string {
  const context = buildContext(passages)
  const block = context === "" ? EMPTY_CORPUS_MESSAGE : context

  return `${block}\n\n${question.trim()}`
}
