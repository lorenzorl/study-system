import { defineStore, storeToRefs } from "pinia"
import { computed, ref } from "vue"
import type { ComputedRef, Ref } from "vue"
import type { ConceptDTO } from "@/entities/topic"
import { useTopicsStore } from "@/entities/topic"
import type { Flashcard } from "@/entities/flashcard"
import { useFlashcardCache } from "@/entities/flashcard"

/**
 * Which concept the user is currently working on, plus the flashcards cached
 * for it. Selection is a feature (not an entity) because it crosses the topic
 * and flashcard entities.
 */
export const useConceptSelection = defineStore("concept-selection", () => {
  const currentConceptId = ref<string | null>(null)
  const flashcardCache = useFlashcardCache()

  const currentFlashcards = computed<Flashcard[]>(() => {
    if (!currentConceptId.value) return []
    return flashcardCache.getCachedFlashcards(currentConceptId.value)
  })

  function selectConcept(id: string | null): void {
    currentConceptId.value = id
  }

  /** Clear the selected concept (used when the selected topic changes). */
  function resetConcept(): void {
    currentConceptId.value = null
  }

  return {
    currentConceptId,
    currentFlashcards,
    selectConcept,
    resetConcept,
  }
})

/**
 * Resolve the selected concept inside a given topic. The topic id is injected
 * by the caller (a page composes both selection features), so this slice never
 * imports a sibling feature of its own layer.
 */
export function useCurrentConcept(
  topicId: Ref<string | null>,
): ComputedRef<ConceptDTO | null> {
  const { currentConceptId } = storeToRefs(useConceptSelection())
  const topicsStore = useTopicsStore()

  return computed<ConceptDTO | null>(() => {
    if (!topicId.value || !currentConceptId.value) return null
    const topic = topicsStore.topics.find((t) => t.id === topicId.value) ?? null
    if (!topic) return null
    return topic.concepts.find((c) => c.id === currentConceptId.value) ?? null
  })
}
