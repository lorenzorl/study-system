import { defineStore } from "pinia"
import { ref } from "vue"
import type { Resource, ResourceResponse } from "./types"
import { fetchResources } from "../api/resource-api"

/** Resolve the wire's snake_case into the domain shape, once. */
function toResource(dto: ResourceResponse): Resource {
  return {
    id: dto.id,
    title: dto.title,
    type: dto.type,
    sourceUri: dto.source_uri,
    topicTitle: dto.topic_name,
    createdAt: dto.created_at,
  }
}

export const useResourcesStore = defineStore("resources", () => {
  const resources = ref<Resource[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function loadResources(): Promise<void> {
    loading.value = true
    error.value = null

    try {
      const data = await fetchResources()
      resources.value = data.map(toResource)
    } catch (e) {
      if (e instanceof Error) {
        error.value = e.message
      } else {
        error.value = "Error desconocido al cargar recursos."
      }
    } finally {
      loading.value = false
    }
  }

  function clearError(): void {
    error.value = null
  }

  return {
    resources,
    loading,
    error,
    loadResources,
    clearError,
  }
})
