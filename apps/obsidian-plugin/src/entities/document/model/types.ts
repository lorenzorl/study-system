// ─── Document domain + API types ────────────────────────────────────────────
// Wire shapes of `POST /api/rag/documents` and
// `GET /api/rag/documents/{document_id}/status` (rag/http/dto.go). The server
// owns the state vocabulary; an unknown state simply matches no UI label.

/** Processing state a document moves through while it is ingested. */
export type DocumentState =
  | "queued"
  | "converting"
  | "indexing"
  | "completed"
  | "failed"

/** Per-page breakdown of a document's progress. */
export interface DocumentPageProgress {
  total: number
  extracted: number
  transcribed: number
  failed: number
  pending: number
}

/** Body of the 202 returned when a document is accepted for processing. */
export interface UploadDocumentResponse {
  document_id: string
  state: DocumentState
}

/** Body of the polling endpoint. Timestamps are RFC3339 and absent when empty. */
export interface DocumentStatus {
  document_id: string
  title: string
  state: DocumentState
  pages: DocumentPageProgress
  chunks: number
  embedded: number
  error?: string
  started_at?: string
  updated_at?: string
  finished_at?: string
}
