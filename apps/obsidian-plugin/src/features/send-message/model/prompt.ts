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

/**
 * What the model has to work with, in the four states the chat can be in.
 *
 * They are not a boolean because "has material" and "has it in the message" are
 * different facts, and the model owes the user a different frame for each. The
 * two facts are independent, so their four combinations are all reachable:
 *
 * - `none`: the chat has no material. The model answers from its own knowledge
 *   and must not write citations it cannot support.
 * - `searchable`: the chat has material attached, but none of it is in the
 *   message. The model is expected to search it with the tool before answering.
 * - `in-message`: the material travels in this request, so the model grounds on
 *   it directly (the vault files sent whole).
 * - `in-message-and-searchable`: both at once. The chat has material in the
 *   message **and** material that is not in it, so it needs one frame that says
 *   both: ground on what it can see and search for the rest. Neither neighbour
 *   frame can speak for this state -- `in-message` would forbid the search its
 *   own request declares, and `searchable` would deny the text it is carrying.
 */
export type MaterialMode =
  | "none"
  | "searchable"
  | "in-message"
  | "in-message-and-searchable"

export interface PromptInput {
  /** The chat's own system prompt, read from its note. */
  chatPrompt: string
  /** The note's previous turns, oldest first. */
  history: PromptTurn[]
  /** The material to ground on, in the order it should be numbered. */
  blocks: ContextBlock[]
  /**
   * Whether the chat has material the model can ground on, and where it is.
   *
   * An explicit input and not `blocks.length === 0`, because those are not the
   * same thing: a chat whose material is behind the tool has no blocks and
   * still has material, so it cannot receive the no-material frame.
   */
  material: MaterialMode
  question: string
}

/**
 * The rules the model always receives, in Spanish because the user's questions
 * are Spanish. They are deliberately not editable by a chat's prompt.
 *
 * The citation rule names both forms because the material in front of the model
 * can be either class, and they are cited differently: a vault file is cited by
 * its path, which is its identity and the only thing it has, while a passage that
 * came from a search is a book or paper and is cited by title and page. Asking for
 * a page from a note would make the model invent one, exactly what the third rule
 * forbids.
 */
export const CITATION_FRAME = [
  "Respondé la pregunta usando solamente el contexto que aparece a continuación.",
  "Si el contexto no alcanza para responderla, decilo explícitamente; no respondas con tu propio conocimiento.",
  "No inventes títulos, páginas ni datos: usá únicamente lo que figura en el contexto.",
  "Cerrá la respuesta con una línea por cada pasaje que hayas usado: si el pasaje es un archivo del vault, citá su ruta como Fuente: <ruta de la nota>; si el pasaje vino de una búsqueda, citá título y página como Fuente: <título>, p. <página>.",
  "Respondé en el idioma de la pregunta.",
].join("\n")

/**
 * The rules the model receives when the chat has no material to ground on.
 *
 * The difference with `CITATION_FRAME` is not cosmetic. That frame forbids the
 * model's own knowledge, which is right when there is material and wrong when
 * there is none: forbidding it would turn every answer into an "I can't". And
 * it mandates the citation format, which a model with nothing to cite will
 * satisfy by inventing sources, so here every citation is forbidden by name.
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
 * The rules the model receives when the chat has material it must search.
 *
 * This is the third state, and its difference with `CITATION_FRAME` is that
 * grounding is a decision the model takes by searching, not a duty it discharges
 * on material already in front of it: forbidding its own knowledge without giving
 * it a way to read the material would leave it with nothing to say. So the frame
 * names the tool, and it also says what to do when the search comes back empty,
 * because a model that cannot tell "no material" from "not consulted" invents
 * either a citation or an answer.
 *
 * The third rule names a failure instead of offering a choice: not searching is
 * not an option the frame grants, it is a lapse the model has to admit, because
 * an answer from memory dressed as a founded one is the failure this state is
 * detected to catch. It comes after the duty to search for that reason: the
 * obligation is stated first, and the admission is what is left when it fails.
 */
export const SEARCHABLE_MATERIAL_FRAME = [
  "Este chat tiene material adjunto, pero el material no está en el mensaje.",
  "Buscá ese material con la herramienta de búsqueda antes de responder.",
  "Si no llegaste a buscar el material, no respondas como si lo hubieras hecho: decí explícitamente que no lo consultaste.",
  "Si el material cubre la pregunta, no la respondas con tu propio conocimiento.",
  "Si el material no cubre la pregunta, decilo explícitamente en la respuesta.",
  "No inventes títulos, páginas ni datos: usá únicamente los pasajes que devolvió la búsqueda.",
  "Cerrá la respuesta con una línea por cada fuente que hayas usado, con este formato exacto: Fuente: <título>, p. <página>.",
  "Respondé en el idioma de la pregunta.",
].join("\n")

/**
 * The rules the model receives when the chat has material in the message and
 * material behind the tool at the same time.
 *
 * This is the fourth state, and it exists because the other two frames
 * contradict one of its halves. `CITATION_FRAME` opens by asking for the context
 * that appears below "solamente", which forbids reading the corpus the same
 * request declares a tool for; `SEARCHABLE_MATERIAL_FRAME` says the material is
 * not in the message, which is false for the vault files it carries. So this
 * frame states both facts and splits the duties: ground on what is there, search
 * for what is not.
 *
 * It reuses text instead of paraphrasing it, so the states cannot drift apart.
 * The admission rule ("Si no llegaste a buscar el material ... no lo
 * consultaste") is copied verbatim from `SEARCHABLE_MATERIAL_FRAME`: it is what
 * keeps the `consulted` badge coherent. The citation rule is copied verbatim
 * from `CITATION_FRAME`, which already names both forms and ties each one to its
 * class. The prohibition on inventing titles, pages and data is `CITATION_FRAME`'s
 * rule extended with `SEARCHABLE_MATERIAL_FRAME`'s "pasajes que devolvió la
 * búsqueda" -- both places the material can come from. The closing rule is the
 * language rule all three share, word for word.
 */
export const IN_MESSAGE_AND_SEARCHABLE_FRAME = [
  "Este chat tiene material en el mensaje y material que no está en el mensaje.",
  "Buscá ese otro material con la herramienta de búsqueda antes de responder.",
  "Si no llegaste a buscar el material, no respondas como si lo hubieras hecho: decí explícitamente que no lo consultaste.",
  "Si el material no cubre la pregunta, decilo explícitamente en la respuesta; no respondas con tu propio conocimiento.",
  "No inventes títulos, páginas ni datos: usá únicamente lo que figura en el contexto y en los pasajes que devolvió la búsqueda.",
  "Cerrá la respuesta con una línea por cada pasaje que hayas usado: si el pasaje es un archivo del vault, citá su ruta como Fuente: <ruta de la nota>; si el pasaje vino de una búsqueda, citá título y página como Fuente: <título>, p. <página>.",
  "Respondé en el idioma de la pregunta.",
].join("\n")

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

/** The system message for a chat whose material is reachable only through the tool. */
export function buildSearchableSystemMessage(chatPrompt: string): string {
  return composeSystemMessage(SEARCHABLE_MATERIAL_FRAME, chatPrompt)
}

/** The system message for a chat with material in the message and behind the tool. */
export function buildInMessageAndSearchableSystemMessage(chatPrompt: string): string {
  return composeSystemMessage(IN_MESSAGE_AND_SEARCHABLE_FRAME, chatPrompt)
}

/** The frame that belongs to each state; the composer only appends the prompt. */
function systemMessageFor(mode: MaterialMode, chatPrompt: string): string {
  switch (mode) {
    case "none":
      return buildNoMaterialSystemMessage(chatPrompt)
    case "searchable":
      return buildSearchableSystemMessage(chatPrompt)
    case "in-message":
      return buildSystemMessage(chatPrompt)
    case "in-message-and-searchable":
      return buildInMessageAndSearchableSystemMessage(chatPrompt)
  }
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
 * block and the question when the material travels in the message, and the bare
 * question otherwise: when the material is behind the tool, the instruction to
 * search it belongs in the frame, not in the question.
 */
export function buildMessages(input: PromptInput & { maxTurns: number }): LlmMessage[] {
  const system: LlmMessage = {
    role: "system",
    content: systemMessageFor(input.material, input.chatPrompt),
  }
  const history: LlmMessage[] = trimHistory(input.history, input.maxTurns).map(
    (turn) => ({ role: turn.role, content: turn.content }),
  )
  const user: LlmMessage = {
    role: "user",
    content: buildUserMessage(input.blocks, input.question, input.material),
  }

  return [system, ...history, user]
}

function renderBlock(block: ContextBlock, index: number): string {
  const heading = block.kind === "corpus" ? `${block.title} — p. ${block.page}` : block.path

  return `[${index + 1}] ${heading}\n${block.text}`
}

/**
 * Whether the material of this mode travels in this request.
 *
 * Both modes that carry blocks qualify, and the mixed one is easy to forget: it
 * is named after the tool, and a mode check that only knows `in-message` drops
 * the vault text from the message, so the model answers as if the note had never
 * been attached -- while the frame tells it to search for it.
 */
function carriesBlocksInMessage(mode: MaterialMode): boolean {
  return mode === "in-message" || mode === "in-message-and-searchable"
}

function buildUserMessage(
  blocks: ContextBlock[],
  question: string,
  mode: MaterialMode,
): string {
  // Only material that travels in this request has a context block. With no
  // material there is nothing to render, and a corpus that was never consulted
  // cannot have "returned nothing".
  if (!carriesBlocksInMessage(mode)) return question.trim()

  const context = buildContext(blocks)
  const block = context === "" ? EMPTY_CORPUS_MESSAGE : context

  return `${block}\n\n${question.trim()}`
}
