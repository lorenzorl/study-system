export interface ConceptSummary {
  id: string
  title: string
  file_path: string
}

export interface SyncConceptRequest {
  topic_name: string
  concept_title: string
  file_path: string
}

export interface SyncConceptResponse {
  concept_id: string
}

export interface CreateConceptRequest {
  topic_id: string
  title: string
}

export interface CreateConceptResponse {
  id: string
  topic_id: string
  title: string
  file_path: string
  created_at: string
}
