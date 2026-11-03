import { defineStore } from "pinia"
import { ref } from "vue"
import type {
  Today,
  TodayEntry,
  TodayEntryResponse,
  TodayResponse,
} from "./types"
import { fetchToday, setReading } from "../api/today-api"

/**
 * A range is the identity: the projection names page ranges, not rows, and the
 * same document can hold two of them.
 */
function keyOf(entry: TodayEntry): string {
  return `${entry.documentId}:${entry.fromPage}-${entry.toPage}`
}

function toEntry(dto: TodayEntryResponse): TodayEntry {
  const base = {
    documentId: dto.document_id,
    documentTitle: dto.document_title,
    fromPage: dto.from_page,
    toPage: dto.to_page,
  }

  if (dto.kind === "read_not_distilled") {
    return { kind: "read_not_distilled", ...base }
  }

  return {
    kind: "plan_next",
    ...base,
    title: dto.title ?? "",
    note: dto.note ?? null,
  }
}

/** Resolve the wire's snake_case into the domain shape, once. */
function toToday(dto: TodayResponse): Today {
  return { dueCount: dto.due_count, entries: dto.entries.map(toEntry) }
}

function messageOf(e: unknown, fallback: string): string {
  return e instanceof Error ? e.message : fallback
}

export const useTodayStore = defineStore("today", () => {
  /** `null` until a read settles: "not loaded" and "nothing to do" are different
   * days, and the page has to be able to tell them apart (`backlog.md` D6). */
  const today = ref<Today | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)
  /** A write in flight. It is not `loading`: the day stays on screen while a
   * toggle is being saved, and the toggle is disabled meanwhile. */
  const saving = ref(false)
  /**
   * The ranges this session marked read and the server accepted.
   *
   * The projection only lists work that is still pending, so the toggle's "on"
   * state cannot come from it — and the same control has to be able to undo with
   * `unread`, which is why the accepted fact lives next to the write that produced
   * it rather than being guessed from the list.
   */
  const readRanges = ref(new Set<string>())

  async function load(): Promise<void> {
    loading.value = true
    error.value = null

    try {
      today.value = toToday(await fetchToday())
    } catch (e) {
      error.value = messageOf(e, "Error desconocido al cargar el día.")
    } finally {
      loading.value = false
    }
  }

  /**
   * Store one range as read or unread (S3), then re-read the projection.
   *
   * The reload is the point: what the item's state is now is the server's answer,
   * so the page never flips a row by itself — a refused write leaves both the day
   * and the toggle exactly as they were, with the message to say so.
   */
  async function setRead(entry: TodayEntry, read: boolean): Promise<boolean> {
    // Two clicks are two writes with no order between them (`R4-retry-click-fanout`
    // is the same failure on the launcher's retry).
    if (saving.value) return false

    saving.value = true
    error.value = null

    try {
      await setReading({
        document_id: entry.documentId,
        from_page: entry.fromPage,
        to_page: entry.toPage,
        state: read ? "read" : "unread",
      })

      if (read) {
        readRanges.value.add(keyOf(entry))
      } else {
        readRanges.value.delete(keyOf(entry))
      }

      await load()
      return true
    } catch (e) {
      error.value = messageOf(e, "Error desconocido al guardar la lectura.")
      return false
    } finally {
      saving.value = false
    }
  }

  function isRead(entry: TodayEntry): boolean {
    return readRanges.value.has(keyOf(entry))
  }

  return {
    today,
    loading,
    error,
    saving,
    load,
    setRead,
    isRead,
  }
})
