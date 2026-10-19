/**
 * The search tool: the one way the model reaches the chat's corpus material.
 *
 * Free of the store and of the loop on purpose. It answers three questions and
 * nothing else: what the model is offered, whether a call it made is inside the
 * chat's scope, and how the accepted call becomes the text of a tool result.
 *
 * The scope invariant lives in `decideToolCall`: the chat's attachments fix what
 * the model may read, and a call naming anything else is refused instead of
 * executed. Without it, "attach a document to this chat" would mean nothing —
 * the model could reach the whole corpus with one invented id.
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
  /** `null` means unscoped: the whole attached material. */
  documentId: string | null
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
 * The optional `resource_id` may narrow the search to a document the chat has
 * attached, and nothing else: an id that is not attached is refused with a
 * message the model can read, never executed. A blank id narrows nothing, so it
 * is treated as absent — that cannot widen the scope.
 */
export function decideToolCall(
  call: ToolCall,
  attachments: ChatAttachment[],
): ToolCallDecision {
  const args = argumentsOf(call)

  if (args === null) return { ok: false, error: invalidArgumentsMessage() }

  const query = typeof args.query === "string" ? args.query.trim() : ""

  if (query === "") return { ok: false, error: missingQueryMessage() }

  const requested =
    typeof args.resource_id === "string" ? args.resource_id.trim() : ""

  if (requested === "") return { ok: true, query, documentId: null }

  const attached = corpusAttachments(attachments)
  const match = attached.find((attachment) => attachment.documentId === requested)

  if (match === undefined) {
    return { ok: false, error: notAttachedMessage(requested, attached) }
  }

  return { ok: true, query, documentId: match.documentId }
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
 * Run an accepted call and turn its results into the text of a tool result.
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
  const results = await search(decision.query, options.limit, decision.documentId ?? undefined)
  const blocks = results.map(toBlock)

  return {
    content: blocks.length === 0 ? EMPTY_SEARCH_RESULT : buildContext(blocks),
    blocks,
  }
}
