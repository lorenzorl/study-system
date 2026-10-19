/**
 * What a chat's copy of a corpus document is worth right now, in words.
 *
 * The numbers come from the corpus listing (`GET /api/rag/documents`), which is
 * the server's own report of the document: nothing here is guessed or derived.
 * The one decision this module makes is which fact matters to a reader — a
 * document with no chunks cannot match a search, so attaching it does not yet
 * change what the model can find, and that is worth saying instead of showing a
 * count that looks fine.
 *
 * It lives in the model and not in the panel because it is a claim about the
 * system's state, and a claim belongs where a test can pin it.
 */
import type { DocumentSummary } from "@/entities/document"

/** The state of a document the listing did not bring, so it cannot be described. */
const NO_DATA = "sin datos del servidor"

export function corpusStateLabel(document: DocumentSummary | undefined): string {
  // The listing is fetched when the pickers open, and the chat can hold an
  // attachment whose document is not in it -- because the server is down, or
  // because it was attached before this listing existed.
  if (document === undefined) return NO_DATA

  if (document.chunks === 0) {
    return document.pages === 0
      ? "sin procesar"
      : `${pages(document.pages)}, todavía sin fragmentos buscables`
  }

  return `${pages(document.pages)}, ${chunks(document.chunks)}`
}

function pages(count: number): string {
  return count === 1 ? "1 página" : `${count} páginas`
}

function chunks(count: number): string {
  return count === 1 ? "1 fragmento" : `${count} fragmentos`
}
