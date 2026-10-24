import { request, uploadForm } from "@/shared/api/client"

/**
 * The body the upload route and the two text routes answer with.
 *
 * All three name the document, because that is the only thing the client needs
 * to register the material afterwards.
 */
export interface IngestedDocument {
  document_id: string
  state: string
}

/**
 * The text route's answer: the counts say whether the corpus could embed the
 * document or indexed it lexically only.
 */
export interface IngestedTextDocument extends IngestedDocument {
  chunks: number
  embedded: number
}

/** The URL route's answer: the same as a paste's, plus what was fetched. */
export interface IngestedUrlDocument extends IngestedTextDocument {
  /** The URL the server fetched, after any redirects. */
  source_uri: string
}

/**
 * Upload a PDF.
 *
 * The answer is a 202, not a 201: the route queues the document and the
 * transcription runs behind it, so a document exists before it can be read.
 */
export async function ingestFile(file: File): Promise<IngestedDocument> {
  const form = new FormData()
  form.append("file", file)

  return uploadForm<IngestedDocument>("/api/rag/documents", form)
}

/** Store a pasted text as a corpus document. */
export async function ingestText(input: {
  title: string
  text: string
}): Promise<IngestedTextDocument> {
  return request<IngestedTextDocument>("/api/rag/documents/text", {
    method: "POST",
    body: JSON.stringify(input),
  })
}

/** Ask the server to fetch a URL's text and store it as a corpus document. */
export async function ingestUrl(input: {
  title: string
  url: string
}): Promise<IngestedUrlDocument> {
  return request<IngestedUrlDocument>("/api/rag/documents/url", {
    method: "POST",
    body: JSON.stringify(input),
  })
}
