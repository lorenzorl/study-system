import { defineStore } from "pinia"
import { computed, ref } from "vue"
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
  /**
   * Whether a load has settled at least once (`backlog.md` D6).
   *
   * `loading` cannot answer this: it is false both before the first request and
   * after the last one, so a page that paints "not found" whenever the store is
   * empty paints it for one frame on every cold visit — before `onMounted` has
   * asked for anything. With this flag, "empty and never settled" is *pending*.
   */
  const loaded = ref(false)

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
      // Settled, successfully or not: the page has an answer to render, and a
      // failed load reports itself through `error` instead of staying pending.
      loaded.value = true
    }
  }

  /**
   * Nothing to render yet, and nobody has asked: the page shows its loading state
   * instead of claiming the topic or concept does not exist.
   *
   * `topics.length === 0` is part of the rule, not a detail (`R3-pending-wedge-nonempty-unloaded`):
   * a store that already holds topics has something to show, and without this the
   * page would spin forever over real data whenever a load had not settled.
   */
  const isPending = computed(
    () =>
      !loaded.value &&
      !loading.value &&
      !error.value &&
      topics.value.length === 0,
  )

  function clearError(): void {
    error.value = null
  }

  return {
    topics,
    loading,
    error,
    loaded,
    isPending,
    loadTopics,
    clearError,
  }
})
