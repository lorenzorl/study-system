import { beforeEach, describe, expect, it, vi } from "vitest"
import { createPinia, setActivePinia } from "pinia"
import { ApiError } from "@/shared/api/client"
import type {
  PlanNextEntry,
  ReadNotDistilledEntry,
  TodayResponse,
} from "@/entities/today"
import { useToday } from "./useToday"

// The write path is the one thing about `/today` that is not a render: the toggle
// has to reach `PUT /api/study/reading` with the right state, refresh the
// projection afterwards, and leave the toggle alone when the write is refused.
// `useToday` is the page's model over `useTodayStore`, so this suite covers both.
//
// The api module is mocked through `vi.hoisted` instead of an import of its path:
// `entities/today/api/today-api` is a segment, not a slice's public API, and the
// boundary guard reads every specifier in this file (`ARCHITECTURE.md` §10).
const api = vi.hoisted(() => ({
  fetchToday: vi.fn(),
  setReading: vi.fn(),
}))

vi.mock("@/entities/today/api/today-api", () => api)

/** `GET /api/study/today` as the handler answers it (S13). */
const PROJECTION: TodayResponse = {
  due_count: 2,
  entries: [
    {
      kind: "read_not_distilled",
      document_id: "doc-1",
      document_title: "DDD",
      from_page: 24,
      to_page: 36,
      read_at: "2026-11-02T10:00:00Z",
    },
    {
      kind: "plan_next",
      document_id: "doc-1",
      document_title: "DDD",
      title: "Capítulo 5",
      from_page: 100,
      to_page: 118,
      note: "volver si hace falta",
    },
  ],
}

/** The row `PUT /api/study/reading` answers with (S3). */
const STORED_ROW = {
  document_id: "doc-1",
  from_page: 100,
  to_page: 118,
  state: "read" as const,
  created_at: "2026-11-02T10:00:00Z",
  updated_at: "2026-11-02T10:00:00Z",
}

/** The projection's `plan_next` entry, in the domain shape the toggle receives. */
function planEntry(today: ReturnType<typeof useToday>): PlanNextEntry {
  const entry = today.entries.value.find((item) => item.kind === "plan_next")
  if (!entry || entry.kind !== "plan_next") {
    throw new Error("the projection has no plan_next entry")
  }
  return entry
}

/** The range nobody distilled yet: the one entry that leads to the explanation. */
function readEntry(today: ReturnType<typeof useToday>): ReadNotDistilledEntry {
  const entry = today.entries.value.find(
    (item) => item.kind === "read_not_distilled",
  )
  if (!entry || entry.kind !== "read_not_distilled") {
    throw new Error("the projection has no read_not_distilled entry")
  }
  return entry
}

beforeEach(() => {
  setActivePinia(createPinia())
  api.fetchToday.mockReset()
  api.setReading.mockReset()
  api.fetchToday.mockResolvedValue(PROJECTION)
  api.setReading.mockResolvedValue(STORED_ROW)
})

describe("useToday", () => {
  it("resolves the projection into the page's own view", async () => {
    const today = useToday()

    await today.load()

    expect(today.dueCount.value).toBe(2)
    expect(today.entries.value.map((entry) => entry.kind)).toEqual([
      "read_not_distilled",
      "plan_next",
    ])
    expect(today.error.value).toBeNull()
  })

  // S14: the toggle calls S3 with `read` on and `unread` off, and the page then
  // asks the projection again instead of guessing what the server now thinks.
  it("marks a plan item read, then unread, refreshing the projection each time", async () => {
    const today = useToday()
    await today.load()
    const entry = planEntry(today)

    await today.setRead(entry, true)

    expect(api.setReading).toHaveBeenLastCalledWith({
      document_id: "doc-1",
      from_page: 100,
      to_page: 118,
      state: "read",
    })
    expect(today.isRead(entry)).toBe(true)
    expect(api.fetchToday).toHaveBeenCalledTimes(2)

    await today.setRead(entry, false)

    expect(api.setReading).toHaveBeenLastCalledWith({
      document_id: "doc-1",
      from_page: 100,
      to_page: 118,
      state: "unread",
    })
    expect(today.isRead(entry)).toBe(false)
    expect(api.fetchToday).toHaveBeenCalledTimes(3)
  })

  it("reports a refused write and leaves the toggle where it was", async () => {
    const today = useToday()
    await today.load()
    const entry = planEntry(today)
    api.setReading.mockRejectedValue(new ApiError("document_id is required", 400))

    await today.setRead(entry, true)

    expect(today.error.value).toBe("document_id is required")
    expect(today.isRead(entry)).toBe(false)
    // Nothing to re-read: the write never landed, so the projection is unchanged.
    expect(api.fetchToday).toHaveBeenCalledTimes(1)
  })

  // S9: the read range is the one entry with somewhere to go. The route names the
  // document and the range, so opening it again re-reads the stored explanation
  // instead of paying for the model.
  it("names the explanation route for a read range", async () => {
    const today = useToday()
    await today.load()

    expect(today.explainRoute(readEntry(today))).toEqual({
      name: "explain",
      params: { documentId: "doc-1", from: 24, to: 36 },
    })
  })

  // D6/D2: a failed load must not read as an empty day.
  it("reports a failed load as an error, not as an empty day", async () => {
    const today = useToday()
    api.fetchToday.mockRejectedValue(new ApiError("not found", 500))

    await today.load()

    expect(today.error.value).toBe("not found")
    expect(today.today.value).toBeNull()
  })
})
