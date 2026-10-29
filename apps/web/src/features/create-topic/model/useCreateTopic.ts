import { defineStore } from "pinia"
import { ref } from "vue"
import { createTopic as apiCreateTopic } from "@/entities/topic"
import { useTopicsStore } from "@/entities/topic"

/**
 * Create a topic in the backend and refresh the topic list.
 *
 * The topic is the backend record and nothing else: there is no folder to
 * mirror, because the app has no filesystem of its own.
 */
export const useCreateTopic = defineStore("create-topic", () => {
  const loading = ref(false)
  const error = ref<string | null>(null)
  const topicsStore = useTopicsStore()

  async function createTopic(name: string): Promise<void> {
    loading.value = true
    error.value = null

    try {
      await apiCreateTopic(name)
      await topicsStore.loadTopics()
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

  function clearError(): void {
    error.value = null
  }

  return {
    loading,
    error,
    createTopic,
    clearError,
  }
})
