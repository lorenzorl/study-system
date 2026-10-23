import { defineStore } from "pinia"
import { ref } from "vue"
import { syncConcept } from "@/entities/concept"
import type { ParsedFlashcard } from "@/entities/flashcard"
import {
  parseFlashcards,
  syncFlashcards,
  useFlashcardCache,
} from "@/entities/flashcard"
import type { Flashcard } from "@/entities/flashcard"
import { useTopicsStore } from "@/entities/topic"

export interface SyncNoteInput {
  topicName: string
  conceptTitle: string
  filePath: string
  fileContent: string
}

export interface SyncNotePayload {
  conceptId: string
  /** Number of flashcards the backend accepted. */
  synced: number
  /** The cards parsed from the note, ready to be cached locally. */
  parsedCards: ParsedFlashcard[]
}

export interface SyncNoteResult {
  conceptId: string
  /** Topic that owns the synced concept, or null when its name is unknown. */
  topicId: string | null
  synced: number
}

/**
 * The sync flow itself — sync the concept, parse the note, sync the flashcards.
 * Pinia-free so the app-layer command can use it before any Pinia instance
 * exists. Throws on failure; callers decide how to surface the error.
 */
export async function syncNoteToBackend(
  input: SyncNoteInput,
): Promise<SyncNotePayload> {
  // 1. Sync concept to get concept_id
  const conceptResponse = await syncConcept({
    topic_name: input.topicName,
    concept_title: input.conceptTitle,
    file_path: input.filePath,
  })

  const conceptId = conceptResponse.concept_id

  // 2. Parse flashcards from markdown
  const parsedCards = parseFlashcards(input.fileContent)
  if (parsedCards.length === 0) {
    throw new Error(
      "No se encontraron tarjetas en la nota. Usá el formato pregunta::respuesta.",
    )
  }

  // 3. Sync flashcards to backend
  const flashcardResponse = await syncFlashcards({
    concept_id: conceptId,
    cards: parsedCards,
  })

  return {
    conceptId,
    synced: flashcardResponse.synced,
    parsedCards,
  }
}

/** Build the locally cached flashcards for a synced concept. */
function toCachedFlashcards(
  conceptId: string,
  cards: ParsedFlashcard[],
): Flashcard[] {
  return cards.map((card, idx) => ({
    id: `cached-${conceptId}-${idx}`,
    concept_id: conceptId,
    question: card.question,
    answer: card.answer,
    obsidian_id: card.obsidian_id,
    created_at: new Date().toISOString(),
  }))
}

/**
 * The interactive sync feature: runs the sync flow, caches the parsed
 * flashcards, refreshes the topic list and reports the synced ids so the
 * caller can select them.
 */
export const useSyncNote = defineStore("sync-note", () => {
  const loading = ref(false)
  const error = ref<string | null>(null)
  const topicsStore = useTopicsStore()
  const flashcardCache = useFlashcardCache()

  async function syncCurrentNote(
    topicName: string,
    conceptTitle: string,
    filePath: string,
    fileContent: string,
  ): Promise<SyncNoteResult | null> {
    loading.value = true
    error.value = null

    try {
      const payload = await syncNoteToBackend({
        topicName,
        conceptTitle,
        filePath,
        fileContent,
      })

      // 4. Cache flashcards locally
      flashcardCache.setCachedFlashcards(
        payload.conceptId,
        toCachedFlashcards(payload.conceptId, payload.parsedCards),
      )

      // 5. Reload topics to reflect new concept in the list
      await topicsStore.loadTopics()

      // Resolve the topic the newly synced concept belongs to
      const topicId =
        topicsStore.topics.find((t) => t.name === topicName)?.id ?? null

      return { conceptId: payload.conceptId, topicId, synced: payload.synced }
    } catch (e) {
      if (e instanceof Error) {
        error.value = e.message
      } else {
        error.value = "Error desconocido al sincronizar la nota."
      }
      return null
    } finally {
      loading.value = false
    }
  }

  function clearError(): void {
    error.value = null
  }

  return {
    loading,
    error,
    syncCurrentNote,
    clearError,
  }
})
