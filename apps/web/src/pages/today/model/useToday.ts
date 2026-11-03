import { computed } from "vue"
import { storeToRefs } from "pinia"
import { useTodayStore } from "@/entities/today"
import type { TodayEntry } from "@/entities/today"

/**
 * The page's own model over `useTodayStore`, the same split `PRESENTATION-UI.md`
 * §8 records for the launcher: the entity owns the wire shape and the requests, the
 * page owns how a day is worded. Nothing here decides what to do — the projection
 * already did (S11, S13).
 */
export function useToday() {
  const store = useTodayStore()
  const { today, loading, error, saving } = storeToRefs(store)

  const entries = computed<TodayEntry[]>(() => today.value?.entries ?? [])
  const dueCount = computed(() => today.value?.dueCount ?? 0)
  const hasDue = computed(() => dueCount.value > 0)

  /**
   * Nothing to do is only claimable once the server said so: an unloaded day and a
   * loaded, empty one look alike in the data and must not in the page (D6).
   */
  const isEmpty = computed(
    () =>
      today.value !== null &&
      dueCount.value === 0 &&
      entries.value.length === 0,
  )

  /** Due reviews are a count, not an entry: there is no range to name (S13). */
  const dueLabel = computed(() =>
    dueCount.value === 1
      ? "Tenés 1 tarjeta para repasar"
      : `Tenés ${dueCount.value} tarjetas para repasar`,
  )

  /** The material's name, or its id when the corpus can no longer resolve one. */
  function materialName(entry: TodayEntry): string {
    return entry.documentTitle || entry.documentId
  }

  function rangeLabel(entry: TodayEntry): string {
    return `páginas ${entry.fromPage}-${entry.toPage}`
  }

  /** Stable within one render: the wire carries no id for a plan item. */
  function entryKey(entry: TodayEntry): string {
    return `${entry.kind}:${entry.documentId}:${entry.fromPage}-${entry.toPage}`
  }

  return {
    today,
    loading,
    error,
    saving,
    entries,
    dueCount,
    hasDue,
    isEmpty,
    dueLabel,
    materialName,
    rangeLabel,
    entryKey,
    load: store.load,
    setRead: store.setRead,
    isRead: store.isRead,
  }
}
