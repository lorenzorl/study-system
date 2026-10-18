// ─── RAG search result domain types ─────────────────────────────────────────
// One element of the bare JSON array returned by `GET /api/rag/search`
// (rag/http/dto.go).

/** Retrieval stage that produced a result. */
export type SearchStage = "lexical" | "semantic" | "hybrid"

export interface SearchResult {
  chunk_id: string
  document_id: string
  document_title: string
  /** 1-based page to open. A chunk never spans two pages. */
  page_number: number
  /**
   * A window of the chunk's text with the matched terms marked, on one line.
   * This is what a caller shows to a human to scan.
   */
  snippet: string
  /**
   * The chunk's text verbatim. This is what a caller prompts the model with;
   * `snippet` is the 12-token marked window and is far too short as context.
   */
  text: string
  stage: SearchStage
  /** Lexical score magnitude: bigger is better, 0 when the stage did not rank. */
  bm25: number
  /** Semantic cosine in [-1, 1]: bigger is better, 0 when unranked. */
  similarity: number
}
