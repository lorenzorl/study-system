import { defineStore } from "pinia"
import { ref } from "vue"
import { createTopic as apiCreateTopic } from "@/entities/topic"
import { useTopicsStore } from "@/entities/topic"

/**
 * Create a topic in the backend and refresh the topic list. The vault folder is
 * created through the Obsidian boundary, loaded on demand so this slice stays
 * importable outside Obsidian (browser dev harness, tests).
 */
export const useCreateTopic = defineStore("create-topic", () => {
  const loading = ref(false)
  const error = ref<string | null>(null)
  const topicsStore = useTopicsStore()

  function ensureTopicFolder(name: string): void {
    import("@/shared/obsidian")
      .then(({ ensureFolder, topicFolderPath }) =>
        ensureFolder(topicFolderPath(name)),
      )
      .catch((err) => {
        console.error("[Study Dashboard] Topic folder creation failed:", err)
      })
  }

  async function createTopic(name: string): Promise<void> {
    loading.value = true
    error.value = null

    try {
      await apiCreateTopic(name)
      // Create vault folder for the new topic
      ensureTopicFolder(name)
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
