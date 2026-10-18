import { defineStore } from "pinia"
import { ref } from "vue"
import { createConcept as apiCreateConcept } from "@/entities/concept"
import { useTopicsStore } from "@/entities/topic"

/**
 * Create a concept under a topic and refresh the topic list. The vault folder is
 * created through the Obsidian boundary, loaded on demand so this slice stays
 * importable outside Obsidian (browser dev harness, tests).
 */
export const useCreateConcept = defineStore("create-concept", () => {
  const loading = ref(false)
  const error = ref<string | null>(null)
  const topicsStore = useTopicsStore()

  function ensureConceptFolder(topicName: string, title: string): void {
    import("@/shared/obsidian")
      .then(({ ensureFolder, conceptFolderPath }) =>
        ensureFolder(conceptFolderPath(topicName, title)),
      )
      .catch((err) => {
        console.error("[Study Dashboard] Concept folder creation failed:", err)
      })
  }

  async function createConcept(topicId: string, title: string): Promise<void> {
    loading.value = true
    error.value = null

    try {
      await apiCreateConcept(topicId, title)
      // Find the topic name from the store to build the folder path
      const topic = topicsStore.topics.find((t) => t.id === topicId)
      if (topic) {
        ensureConceptFolder(topic.name, title)
      }
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
