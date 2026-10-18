import { defineStore } from "pinia"
import { ref } from "vue"
import type { Flashcard } from "./types"

/**
 * Local cache of the flashcards parsed from the vault, keyed by concept id.
 * Populated after a successful sync; read by the concept/topic/flashcard views.
 */
export const useFlashcardCache = defineStore("flashcard-cache", () => {
  const flashcardsMap = ref<Map<string, Flashcard[]>>(new Map())

  function getCachedFlashcards(conceptId: string): Flashcard[] {
    return flashcardsMap.value.get(conceptId) ?? []
  }

  /** Replace the cached flashcards of a concept (called after a sync). */
  function setCachedFlashcards(conceptId: string, cards: Flashcard[]): void {
    flashcardsMap.value.set(conceptId, cards)
  }

  return {
    flashcardsMap,
    getCachedFlashcards,
    setCachedFlashcards,
  }
})
