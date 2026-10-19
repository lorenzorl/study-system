import type { DocumentStatus, DocumentSummary, UploadDocumentResponse } from "../model/types"
import { request, requestMultipart } from "@/shared/api/client"

/**
 * Upload one document into the RAG corpus.
 *
 * `file` is the only form field the endpoint reads. The body goes through
 * `requestMultipart`, never through `request`, because the browser has to write
 * the multipart boundary itself.
 */
export async function uploadDocument(
  file: File,
): Promise<UploadDocumentResponse> {
  const body = new FormData()
  body.append("file", file)
  return requestMultipart<UploadDocumentResponse>("/api/rag/documents", body)
}

/** Read one document's ingestion progress and page counts. */
export async function fetchDocumentStatus(
  documentId: string,
): Promise<DocumentStatus> {
  return request<DocumentStatus>(
    `/api/rag/documents/${encodeURIComponent(documentId)}/status`,
  )
}

/**
 * List the corpus.
 *
 * A bare JSON array, never null: an empty corpus is an empty array, so a caller
 * ranges over it instead of special-casing "there is nothing to attach".
 *
 * `signal` is what lets a caller give up on the request. It exists for the
 * corpus preflight: that probe is a network await in front of the first model
 * call, so a server that accepts the connection and never answers has to be
 * abandonable, or the whole send hangs behind it. Optional, so every existing
 * caller keeps the request it already made.
 */
export async function listDocuments(
  signal?: AbortSignal,
): Promise<DocumentSummary[]> {
  return request<DocumentSummary[]>("/api/rag/documents", { signal })
}
