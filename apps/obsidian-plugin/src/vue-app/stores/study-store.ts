import { defineStore } from "pinia"
import { ref, computed } from "vue"
import type { Topic, ConceptSummary, Flashcard } from "../../types"
import { fetchTopics, syncConcept, syncFlashcards, createTopic, createConcept } from "../services/api"
import { parseFlashcards } from "../services/markdown-parser"
import { ensureFolder, topicFolderPath, conceptFolderPath } from "../composables/useObsidian"

export const useStudyStore = defineStore("study", () => {
  const topics = ref<Topic[]>([])
  const currentTopicId = ref<string | null>(null)
  const currentConceptId = ref<string | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const flashcardsMap = ref<Map<string, Flashcard[]>>(new Map())

  // ── Getters ──────────────────────────────────────────────────────────────

  const currentTopic = computed<Topic | null>(() => {
    if (!currentTopicId.value) return null
    return topics.value.find((t) => t.id === currentTopicId.value) ?? null
  })

  const currentConcept = computed<ConceptSummary | null>(() => {
    if (!currentTopic.value || !currentConceptId.value) return null
    return (
      currentTopic.value.concepts.find(
        (c) => c.id === currentConceptId.value,
      ) ?? null
    )
  })

  const currentFlashcards = computed<Flashcard[]>(() => {
    if (!currentConceptId.value) return []
    return getCachedFlashcards(currentConceptId.value)
  })

  // ── Actions ──────────────────────────────────────────────────────────────

  function selectTopic(id: string) {
    currentTopicId.value = id
    currentConceptId.value = null
  }

  function selectConcept(id: string) {
    currentConceptId.value = id
  }

  function clearError() {
    error.value = null
  }

  function getCachedFlashcards(conceptId: string): Flashcard[] {
    return flashcardsMap.value.get(conceptId) ?? []
  }

  async function loadTopics(): Promise<void> {
    loading.value = true
    error.value = null

    try {
      const data = await fetchTopics()
      topics.value = data.map((t) => ({
        id: t.id,
        name: t.name,
        concepts: t.concepts.map((c) => ({
          id: c.id,
          title: c.title,
          file_path: c.file_path,
        })),
      }))
    } catch (e) {
      if (e instanceof Error) {
        error.value = e.message
      } else {
        error.value = "Error desconocido al cargar temas."
      }
    } finally {
      loading.value = false
    }
  }

  async function syncCurrentNote(
    topicName: string,
    conceptTitle: string,
    filePath: string,
    fileContent: string,
  ): Promise<void> {
    loading.value = true
    error.value = null

    try {
      // 1. Sync concept to get concept_id
      const conceptResponse = await syncConcept({
        topic_name: topicName,
        concept_title: conceptTitle,
        file_path: filePath,
      })

      const conceptId = conceptResponse.concept_id

      // 2. Parse flashcards from markdown
      const parsedCards = parseFlashcards(fileContent)
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

      // 4. Cache flashcards locally
      const cachedFlashcards: Flashcard[] = parsedCards.map((card, idx) => ({
        id: `cached-${conceptId}-${idx}`,
        concept_id: conceptId,
        question: card.question,
        answer: card.answer,
        obsidian_id: card.obsidian_id,
        created_at: new Date().toISOString(),
      }))
      flashcardsMap.value.set(conceptId, cachedFlashcards)

      // 5. Reload topics to reflect new concept in the list
      await loadTopics()

      // Force select the newly synced concept
      currentTopicId.value =
        topics.value.find((t) => t.name === topicName)?.id ?? null
      currentConceptId.value = conceptId
    } catch (e) {
      if (e instanceof Error) {
        error.value = e.message
      } else {
        error.value = "Error desconocido al sincronizar la nota."
      }
    } finally {
      loading.value = false
    }
  }

  async function createTopicAction(name: string): Promise<void> {
    loading.value = true
    error.value = null
    try {
      await createTopic(name)
      // Create vault folder for the new topic
      ensureFolder(topicFolderPath(name)).catch((err) => {
        console.error("[Study Dashboard] Topic folder creation failed:", err)
      })
      await loadTopics()
    } catch (e) {
      if (e instanceof Error) {
        error.value = e.message
      } else {
        error.value = "Error al crear el tema"
      }
    } finally {
      loading.value = false
    }
  }

  async function createConceptAction(
    topicId: string,
    title: string,
  ): Promise<void> {
    loading.value = true
    error.value = null
    try {
      await createConcept(topicId, title)
      // Find the topic name from the store to build the folder path
      const topic = topics.value.find((t) => t.id === topicId)
      if (topic) {
        ensureFolder(conceptFolderPath(topic.name, title)).catch((err) => {
          console.error("[Study Dashboard] Concept folder creation failed:", err)
        })
      }
      await loadTopics()
    } catch (e) {
      if (e instanceof Error) {
        error.value = e.message
      } else {
        error.value = "Error al crear el concepto"
      }
    } finally {
      loading.value = false
    }
  }

  return {
    topics,
    currentTopicId,
    currentConceptId,
    currentTopic,
    currentConcept,
    currentFlashcards,
    loading,
    error,
    flashcardsMap,
    selectTopic,
    selectConcept,
    clearError,
    loadTopics,
    syncCurrentNote,
    getCachedFlashcards,
    createTopic: createTopicAction,
    createConcept: createConceptAction,
  }
})
