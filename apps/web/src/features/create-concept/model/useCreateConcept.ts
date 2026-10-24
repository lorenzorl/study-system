import { defineStore } from "pinia"
import { ref } from "vue"
import { createConcept as apiCreateConcept } from "@/entities/concept"
import { useTopicsStore } from "@/entities/topic"

/**
 * Create a concept under a topic and refresh the topic list.
 *
 * The concept is the backend record and nothing else: there is no folder to
 * mirror, because the app has no filesystem of its own.
 */
export const useCreateConcept = defineStore("create-concept", () => {
  const loading = ref(false)
  const error = ref<string | null>(null)
  const topicsStore = useTopicsStore()

  async function createConcept(topicId: string, title: string): Promise<void> {
    loading.value = true
    error.value = null

    try {
      await apiCreateConcept(topicId, title)
      await topicsStore.loadTopics()
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

  function clearError(): void {
    error.value = null
  }

  return {
    loading,
    error,
    createConcept,
    clearError,
  }
})
