import { defineStore } from "pinia"
import { computed, ref } from "vue"
import type { Topic } from "@/entities/topic"
import { useTopicsStore } from "@/entities/topic"

/**
 * Which topic the user is currently working on. Selection is a feature (not an
 * entity) because it is stateful user interaction; the derived `currentTopic`
 * is resolved against `entities/topic` through its public API.
 */
export const useTopicSelection = defineStore("topic-selection", () => {
  const currentTopicId = ref<string | null>(null)
  const topicsStore = useTopicsStore()

  const currentTopic = computed<Topic | null>(() => {
    if (!currentTopicId.value) return null
    return topicsStore.topics.find((t) => t.id === currentTopicId.value) ?? null
  })

  function selectTopic(id: string | null): void {
    currentTopicId.value = id
  }

  return {
    currentTopicId,
    currentTopic,
    selectTopic,
  }
})
