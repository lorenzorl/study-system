import { defineStore } from "pinia"
import { ref } from "vue"
import type { Topic, TopicResponse } from "./types"
import { fetchTopics } from "../api/topic-api"

/** Map a `TopicResponse` DTO to the `Topic` domain shape. */
function toTopic(dto: TopicResponse): Topic {
  return {
    id: dto.id,
    name: dto.name,
    concepts: dto.concepts.map((c) => ({
      id: c.id,
      title: c.title,
      file_path: c.file_path,
    })),
  }
}

export const useTopicsStore = defineStore("topics", () => {
  const topics = ref<Topic[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function loadTopics(): Promise<void> {
    loading.value = true
    error.value = null

    try {
      const data = await fetchTopics()
      topics.value = data.map(toTopic)
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

  function clearError(): void {
    error.value = null
  }

  return {
    topics,
    loading,
    error,
    loadTopics,
    clearError,
  }
})
