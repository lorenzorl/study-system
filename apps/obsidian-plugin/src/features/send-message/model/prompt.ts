/**
 * The pure prompt builder for a chat answer.
 *
 * No I/O: given a chat's own prompt, its previous turns, the material blocks
 * and the question, it produces the messages the provider receives.
 * The citation rules live here, in code, so a chat's prompt (S1: a prompt that
 * works for that chat only) can never replace them — it is appended inside the
 * frame, never substituted for it.
 */
import type { LlmMessage } from "@/shared/llm"

/**
 * One block of material the model can ground on.
 *
 * Two classes, because they are cited differently and that asymmetry is the
 * whole reason for the union. A corpus document is a book or paper the server
 * indexed, so it is cited by title and page. A vault file has no pages at all,
 * so it is cited by its path, which is also its identity — giving it a title
 * or a page would mean inventing data the note does not have (L14).
 */
export type ContextBlock =
  | { kind: "corpus"; title: string; page: number; text: string }
  | { kind: "vault"; path: string; text: string }

export interface PromptTurn {
  role: "user" | "assistant"
  content: string
}

export interface PromptInput {
  /** The chat's own system prompt, read from its note. */
  chatPrompt: string
  /** The note's previous turns, oldest first. */
  history: PromptTurn[]
  /** The material to ground on, in the order it should be numbered. */
  blocks: ContextBlock[]
  /**
   * Whether the chat has material the model can ground on.
   *
   * An explicit input and not `blocks.length === 0`, because those are not the
   * same thing: a chat whose material the model chose not to consult has no
   * blocks and still has material, and it owes the user a different frame.
   */
  grounded: boolean
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

/**
 * The rules the model receives when the chat has no material to ground on.
 *
 * The difference with `CITATION_FRAME` is not cosmetic. That frame forbids the
 * model's own knowledge, which is right when there is material and wrong when
 * there is none: forbidding it would turn every answer into an "I can't". And
 * it mandates the citation format, which a model with no pages to cite will
 * satisfy by inventing them, so here the format has to be forbidden by name.
 */
export const NO_MATERIAL_FRAME = [
  "Este chat no tiene material adjunto: respondé con tu propio conocimiento.",
  "No escribas líneas «Fuente: …» ni cites páginas: no hay material que citar.",
  "Si la respuesta depende de material que no tenés, decilo explícitamente.",
  "Respondé en el idioma de la pregunta.",
].join("\n")

/** Said plainly when retrieval returned nothing, so the frame has something to act on. */
const EMPTY_CORPUS_MESSAGE =
  "El corpus no devolvió ningún pasaje para esta pregunta."

/**
 * The frame followed by the chat's prompt. An empty prompt leaves the frame
 * alone, with no trailing separator.
 */
function composeSystemMessage(frame: string, chatPrompt: string): string {
  const trimmed = chatPrompt.trim()

  return trimmed === "" ? frame : `${frame}\n\n${trimmed}`
}

/** The system message for a chat with material: the citation rules and its prompt. */
export function buildSystemMessage(chatPrompt: string): string {
  return composeSystemMessage(CITATION_FRAME, chatPrompt)
}

/** The system message for a chat with no material: its own knowledge, no citations. */
export function buildNoMaterialSystemMessage(chatPrompt: string): string {
  return composeSystemMessage(NO_MATERIAL_FRAME, chatPrompt)
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

/**
 * One numbered block per block of material, in order, from both classes:
 * index, title and exact page for the corpus, index and path for the vault,
 * then the text.
 */
export function buildContext(blocks: ContextBlock[]): string {
  return blocks.map(renderBlock).join("\n\n")
}

/**
 * The citation data in the note's own shape, deduplicated in block order across
 * both classes. A citation is plain data (`Domain-Driven Design, p. 42`, or
 * `Notas/Tesis.md` for a vault file); the note adds the `Fuente: ` prefix when
 * it renders.
 */
export function buildSources(blocks: ContextBlock[]): string[] {
  const seen = new Set<string>()
  const sources: string[] = []

  for (const block of blocks) {
    const source =
      block.kind === "corpus" ? `${block.title}, p. ${block.page}` : block.path

    if (seen.has(source)) continue

    seen.add(source)
    sources.push(source)
  }

  return sources
}

/**
 * `[system, ...trimmed history, user]`. The last message carries the context
 * block and the question when the chat is grounded, and the bare question when
 * it is not.
 */
export function buildMessages(input: PromptInput & { maxTurns: number }): LlmMessage[] {
  const system: LlmMessage = {
    role: "system",
    content: input.grounded
      ? buildSystemMessage(input.chatPrompt)
      : buildNoMaterialSystemMessage(input.chatPrompt),
  }
  const history: LlmMessage[] = trimHistory(input.history, input.maxTurns).map(
    (turn) => ({ role: turn.role, content: turn.content }),
  )
  const user: LlmMessage = {
    role: "user",
    content: buildUserMessage(input.blocks, input.question, input.grounded),
  }

  return [system, ...history, user]
}

function renderBlock(block: ContextBlock, index: number): string {
  const heading = block.kind === "corpus" ? `${block.title} — p. ${block.page}` : block.path

  return `[${index + 1}] ${heading}\n${block.text}`
}

function buildUserMessage(
  blocks: ContextBlock[],
  question: string,
  grounded: boolean,
): string {
  // With no material there is no context block and no empty-corpus line: a
  // corpus that was never consulted cannot have "returned nothing".
  if (!grounded) return question.trim()

  const context = buildContext(blocks)
  const block = context === "" ? EMPTY_CORPUS_MESSAGE : context

  return `${block}\n\n${question.trim()}`
}
