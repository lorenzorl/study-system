// ─── Topic domain + API types ───────────────────────────────────────────────
// `ConceptDTO` is the topic-owned shape for the embedded concept list; the
// concept entity owns `ConceptSummary` (structurally identical). Validating the
// embedded shape locally keeps `entities/topic` free of a same-layer import of
// `entities/concept` (ARCHITECTURE §7c/§13).

export interface ConceptDTO {
  id: string
  title: string
  file_path: string
}

export interface Topic {
  id: string
  name: string
  concepts: ConceptDTO[]
}

export interface TopicResponse {
  id: string
  name: string
  concepts: ConceptDTO[]
}

export interface CreateTopicRequest {
  name: string
}

export interface CreateTopicResponse {
  id: string
  name: string
  created_at: string
}
