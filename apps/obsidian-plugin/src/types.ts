// ─── Domain Types (used in store + components) ─────────────────────────────

export interface Topic {
  id: string
  name: string
  concepts: ConceptSummary[]
}

export interface ConceptSummary {
  id: string
  title: string
  file_path: string
}

export interface Flashcard {
  id: string
  concept_id: string
  question: string
  answer: string
  obsidian_id: string
  created_at: string
}

export interface StudyMetrics {
  dailyCardCount: number
  retentionRate: number // 0.0 – 1.0
  currentStreak: number // days
  totalReviewed: number
}

// ─── API Response Types (matching Go DTOs) ──────────────────────────────────

export interface TopicResponse {
  id: string
  name: string
  concepts: ConceptDTO[]
}

export interface ConceptDTO {
  id: string
  title: string
  file_path: string
}

// ─── API Request / Response Types ───────────────────────────────────────────

export interface SyncConceptRequest {
  topic_name: string
  concept_title: string
  file_path: string
}

export interface SyncConceptResponse {
  concept_id: string
}

export interface FlashcardInput {
  obsidian_id: string
  question: string
  answer: string
}

export interface SyncFlashcardsRequest {
  concept_id: string
  cards: FlashcardInput[]
}

export interface SyncFlashcardsResponse {
  synced: number
}
