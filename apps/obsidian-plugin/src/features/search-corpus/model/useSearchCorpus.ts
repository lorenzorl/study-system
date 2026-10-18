import { defineStore } from "pinia"
import { ref } from "vue"
import type { SearchResult } from "@/entities/search-result"
import { searchCorpus } from "@/entities/search-result"

/**
 * The corpus-search interaction: run one query and keep its results. An empty
 * query is a no-op because the endpoint rejects it.
 */
export const useSearchCorpus = defineStore("search-corpus", () => {
  const results = ref<SearchResult[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function search(query: string, limit?: number): Promise<void> {
    const trimmed = query.trim()
    if (!trimmed) return

    loading.value = true
    error.value = null
    results.value = []

    try {
      results.value = await searchCorpus(trimmed, limit)
    } catch (e) {
      error.value =
        e instanceof Error ? e.message : "No se pudo buscar en el corpus."
    } finally {
      loading.value = false
    }
  }

  function clear(): void {
    results.value = []
    error.value = null
  }

  return {
    results,
    loading,
    error,
    search,
    clear,
  }
})
