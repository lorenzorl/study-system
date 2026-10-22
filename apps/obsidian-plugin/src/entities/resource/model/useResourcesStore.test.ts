import { beforeEach, describe, expect, it, vi } from "vitest"
import { createPinia, setActivePinia } from "pinia"
import { fetchResources } from "../api/resource-api"
import { useResourcesStore } from "./useResourcesStore"
import type { ResourceResponse } from "./types"

vi.mock("../api/resource-api", () => ({ fetchResources: vi.fn() }))

const mockedFetchResources = vi.mocked(fetchResources)

const WIRE: ResourceResponse = {
  id: "r-1",
  title: "Clean Architecture",
  type: "book",
  source_uri: "obsidian://clean-arch.md",
  topic_name: "DDD",
  created_at: "2026-03-01T12:00:00Z",
}

beforeEach(() => {
  setActivePinia(createPinia())
  mockedFetchResources.mockReset()
})

describe("useResourcesStore", () => {
  it("resolves the wire shape into the domain shape", async () => {
    mockedFetchResources.mockResolvedValue([WIRE])

    const store = useResourcesStore()
    await store.loadResources()

    expect(store.resources).toEqual([
      {
        id: "r-1",
        title: "Clean Architecture",
        type: "book",
        sourceUri: "obsidian://clean-arch.md",
        topicTitle: "DDD",
        createdAt: "2026-03-01T12:00:00Z",
      },
    ])
    expect(store.loading).toBe(false)
    expect(store.error).toBeNull()
  })

  it("keeps the message of a failed load and stops loading", async () => {
    mockedFetchResources.mockRejectedValue(
      new Error("No se pudo conectar al servidor."),
    )

    const store = useResourcesStore()
    await store.loadResources()

    expect(store.resources).toEqual([])
    expect(store.error).toBe("No se pudo conectar al servidor.")
    expect(store.loading).toBe(false)
  })

  it("replaces the previous list instead of appending to it", async () => {
    const store = useResourcesStore()

    mockedFetchResources.mockResolvedValue([WIRE])
    await store.loadResources()

    mockedFetchResources.mockResolvedValue([])
    await store.loadResources()

    expect(store.resources).toEqual([])
  })

  it("drops the previous error when a retry starts", async () => {
    const store = useResourcesStore()

    mockedFetchResources.mockRejectedValueOnce(new Error("caído"))
    await store.loadResources()
    expect(store.error).toBe("caído")

    mockedFetchResources.mockResolvedValueOnce([])
    await store.loadResources()

    expect(store.error).toBeNull()
  })
})
