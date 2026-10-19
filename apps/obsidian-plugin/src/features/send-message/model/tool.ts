/**
 * The search tool: the one way the model reaches the chat's corpus material.
 *
 * Free of the store and of the loop on purpose. It answers three questions and
 * nothing else: what the model is offered, whether a call it made is inside the
 * chat's scope, and how the accepted call becomes the text of a tool result.
 *
 * The scope invariant lives in `decideToolCall`: the chat's attachments fix what
 * the model may read — a call that names nothing covers exactly those, and a call
 * that names anything else is refused instead of executed. Without it, "attach a
 * document to this chat" would mean nothing — the model could reach the whole
 * corpus with one invented id, or by naming nothing at all.
 */
import type { ChatAttachment } from "@/entities/chat"
import { searchCorpus } from "@/entities/search-result"
import type { SearchResult } from "@/entities/search-result"
import type { ToolCall, ToolSpec } from "@/shared/llm"
import { buildContext } from "./prompt"
import type { ContextBlock } from "./prompt"

export const SEARCH_MATERIAL_TOOL_NAME = "search_material"

/**
 * What the model is told about the tool.
 *
 * The description is written for the model, not for a UI: it says the material
 * is the one attached to this chat, that `resource_id` can only narrow that set,
 * and that a search that returns nothing is information too.
 */
export const SEARCH_MATERIAL_TOOL: ToolSpec = {
  type: "function",
  function: {
    name: SEARCH_MATERIAL_TOOL_NAME,
    description: [
      "Busca pasajes en el material adjunto a este chat.",
      "Usala antes de responder cuando la pregunta pueda estar cubierta por ese material.",
      "Sin resource_id busca en todo ese material, y solo en él: nunca en el resto del corpus.",
      "Devuelve los pasajes encontrados con su título y su página.",
      "Si la búsqueda no devuelve nada, el material adjunto no cubre la pregunta.",
    ].join(" "),
    parameters: {
      type: "object",
      properties: {
        query: {
          type: "string",
          description:
            "El texto a buscar en el material adjunto, en el idioma del material.",
        },
        resource_id: {
          type: "string",
          description:
            "Opcional. El id de un documento adjunto a este chat, para buscar solo en ese documento. " +
            "No sirve para buscar en documentos que no estén adjuntos.",
        },
      },
      required: ["query"],
    },
  },
}

/** A corpus attachment, which is the only kind the tool can read. */
type CorpusAttachment = Extract<ChatAttachment, { kind: "corpus" }>

/** An accepted call, already inside the chat's scope. */
export interface AcceptedToolCall {
  ok: true
  query: string
  /**
   * The attached documents this call may read, never empty: the one the model
   * named, or every one the chat has when it named none.
   */
  documentIds: string[]
}

/** A refused call, with the message the model reads instead of a result. */
export interface RefusedToolCall {
  ok: false
  error: string
}

export type ToolCallDecision = AcceptedToolCall | RefusedToolCall

/** The text of a tool result plus the blocks it was rendered from. */
export interface ToolResult {
  content: string
  blocks: ContextBlock[]
}

/** The search the tool runs; injectable so tests never touch the network. */
export type MaterialSearch = (
  query: string,
  limit?: number,
  documentId?: string,
) => Promise<SearchResult[]>

export interface ToolRunOptions {
  /** Defaults to the real corpus client. */
  search?: MaterialSearch
  /** How many passages to ask for; omitted lets the server apply its default. */
  limit?: number
}

const EMPTY_SEARCH_RESULT =
  "La búsqueda no devolvió ningún pasaje del material adjunto a este chat."

function invalidArgumentsMessage(): string {
  return (
    "No se pudo leer la llamada: los argumentos no son un objeto JSON válido. " +
    'Mandá un objeto con "query" y, si querés acotar, "resource_id".'
  )
}

function missingQueryMessage(): string {
  return 'Falta "query": la búsqueda necesita el texto que hay que buscar.'
}

function noAttachmentsMessage(): string {
  return (
    "No se puede buscar: este chat no tiene material del corpus adjunto. " +
    "Adjuntá un documento del corpus para que el modelo pueda leerlo."
  )
}

function notAttachedMessage(documentId: string, attachments: CorpusAttachment[]): string {
  const available = attachments
    .map((attachment) => `${attachment.title} (${attachment.documentId})`)
    .join(", ")

  return (
    `No se puede buscar en "${documentId}": ese documento no está adjunto a este chat. ` +
    `Podés buscar sin "resource_id" en todo el material adjunto, o usar uno de estos: ${available}.`
  )
}

function corpusAttachments(attachments: ChatAttachment[]): CorpusAttachment[] {
  return attachments.filter(
    (attachment): attachment is CorpusAttachment => attachment.kind === "corpus",
  )
}

/** The `arguments` object of a call, or `null` when it is not a usable object. */
function argumentsOf(call: ToolCall): Record<string, unknown> | null {
  let parsed: unknown

  try {
    parsed = JSON.parse(call.function.arguments)
  } catch {
    return null
  }

  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return null

  return parsed as Record<string, unknown>
}

/**
 * Decide whether a call the model made is inside the chat's scope.
 *
 * The scope is the chat's corpus attachments on every path. The optional
 * `resource_id` may narrow it to one of them and nothing else: an id that is not
 * attached is refused with a message the model can read, never executed. When no
 * id is named — absent or blank — the call covers all of the attachments, which
 * is the corpus the chat owns and not the corpus the server holds.
 */
export function decideToolCall(
  call: ToolCall,
  attachments: ChatAttachment[],
): ToolCallDecision {
  const args = argumentsOf(call)

  if (args === null) return { ok: false, error: invalidArgumentsMessage() }

  const query = typeof args.query === "string" ? args.query.trim() : ""

  if (query === "") return { ok: false, error: missingQueryMessage() }

  const attached = corpusAttachments(attachments)

  // The tool is only declared when the chat has material; if that state is ever
  // reachable, this refusal is what keeps the whole corpus out of reach.
  if (attached.length === 0) return { ok: false, error: noAttachmentsMessage() }

  const requested =
    typeof args.resource_id === "string" ? args.resource_id.trim() : ""

  if (requested === "") {
    return { ok: true, query, documentIds: attached.map((attachment) => attachment.documentId) }
  }

  const match = attached.find((attachment) => attachment.documentId === requested)

  if (match === undefined) {
    return { ok: false, error: notAttachedMessage(requested, attached) }
  }

  return { ok: true, query, documentIds: [match.documentId] }
}

/** One search result as a context block: the corpus class, with its page. */
function toBlock(result: SearchResult): ContextBlock {
  return {
    kind: "corpus",
    title: result.document_title,
    page: result.page_number,
    text: result.text,
  }
}

/**
 * Merge the per-document result lists into one ranking.
 *
 * `bm25 + similarity` is a crude composition of the two stages the server
 * usually merges itself, and it is acceptable here because it is monotone in
 * what the server already ranked (both scores are bigger-is-better) and because
 * it is deterministic: equal scores break on `document_id` and then `chunk_id`,
 * so the same inputs always produce the same order.
 */
function mergeRanked(results: SearchResult[]): SearchResult[] {
  return [...results].sort((a, b) => {
    const byScore = b.bm25 + b.similarity - (a.bm25 + a.similarity)

    if (byScore !== 0) return byScore
    if (a.document_id !== b.document_id) return a.document_id < b.document_id ? -1 : 1

    return a.chunk_id === b.chunk_id ? 0 : a.chunk_id < b.chunk_id ? -1 : 1
  })
}

/**
 * Run an accepted call and turn its results into the text of a tool result.
 *
 * `searchCorpus` narrows one document at a time, so a call that covers several
 * attachments is one scoped search per document, run concurrently. The merge and
 * the caller's limit happen here, on the merged list.
 *
 * The rendering is `buildContext`, the same composer the prompt uses, so a
 * passage reads identically whether it arrived in the message or through the
 * tool. The blocks come back with the result so the caller can cite what the
 * tool actually returned.
 */
export async function runSearchTool(
  decision: AcceptedToolCall,
  options: ToolRunOptions = {},
): Promise<ToolResult> {
  const search = options.search ?? searchCorpus
  const searched = await Promise.all(
    decision.documentIds.map((documentId) =>
      search(decision.query, options.limit, documentId),
    ),
  )
  const ranked = mergeRanked(searched.flat())
  const results = options.limit === undefined ? ranked : ranked.slice(0, options.limit)
  const blocks = results.map(toBlock)

  return {
    content: blocks.length === 0 ? EMPTY_SEARCH_RESULT : buildContext(blocks),
    blocks,
  }
}
