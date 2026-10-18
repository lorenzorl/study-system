export interface Flashcard {
  id: string
  concept_id: string
  question: string
  answer: string
  obsidian_id: string
  created_at: string
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
