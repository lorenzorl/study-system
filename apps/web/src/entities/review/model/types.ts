export interface DueCard {
  flashcard_id: string
  question: string
  answer: string
  concept_title: string
  topic_name: string
  next_review: string // ISO 8601
}

export interface ReviewRequest {
  flashcard_id: string
  grade: number // 1-4
  duration_ms: number
}

export interface ReviewResponse {
  next_review: string
  stability: number
  difficulty: number
}
