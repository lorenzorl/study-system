import type { SearchResult } from "../model/types"
import { request } from "@/shared/api/client"

/** Largest `limit` the endpoint accepts (rag/http MaxSearchLimit). */
export const MAX_SEARCH_LIMIT = 50

/**
 * Query the RAG corpus.
 *
 * The server trims `q` and rejects it when empty, and it rejects a `limit`
 * outside [1, MAX_SEARCH_LIMIT]; omitting `limit` lets the server apply its own
 * default. No matches come back as an empty array, never as null.
 *
 * `documentId` narrows the search to one document. It is optional and the query
 * string is unchanged when it is absent, because the corpus search page calls
 * this same function to search everything.
 */
export async function searchCorpus(
  query: string,
  limit?: number,
  documentId?: string,
): Promise<SearchResult[]> {
  const params = new URLSearchParams({ q: query })
  if (limit !== undefined) {
    params.set("limit", String(limit))
  }
  if (documentId !== undefined) {
    params.set("document_id", documentId)
  }
  return request<SearchResult[]>(`/api/rag/search?${params.toString()}`)
}
