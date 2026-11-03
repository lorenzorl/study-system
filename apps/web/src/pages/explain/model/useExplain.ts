import { computed, reactive, ref } from "vue"
import { storeToRefs } from "pinia"
import { useExplanationStore } from "@/entities/explanation"
import type { GapState, RangeGap } from "@/entities/explanation"

/** The range the route names: `/explain/:documentId/:from/:to`. */
export interface ExplainTarget {
  documentId: string
  from: number
  to: number
}

/**
 * The page's own model over `useExplanationStore`, the split `pages/today` keeps:
 * the entity owns the wire shape and the requests, the page owns how a range and its
 * gaps are worded, and which of the failures the reader is looking at.
 *
 * The router deliberately stays out: the page puts the new explanation's id in the
 * URL, this answers it. `submit()` resolving with `null` is the whole contract for
 * "nothing was stored".
 */
export function useExplain(target: ExplainTarget) {
  const store = useExplanationStore()
  const { explanation, loading, saving, error } = storeToRefs(store)

  /** The reader's words. Kept across every failure: losing them is the worst thing
   * this page can do, and a refusal is asking for more of them. */
  const text = ref("")

  /** The explanation a retry would read again, when the failure was a read. */
  let lastId: string | null = null

  /**
   * The draft front of each gap's card, by gap id.
   *
   * A gap's `point` is a statement — "the prototype binds model and implementation" —
   * and a card whose front asserts can be recognised but not recalled. So the front is
   * pre-filled with the point and the reader rewrites it as a question; the book
   * provides the evidence behind it (`L5`). The draft lives here, in the page's own
   * model, because it is nothing until the reader accepts it: no request carries it
   * until then, and `startOver` throws it away with the explanation.
   */
  const fronts = reactive(new Map<string, { text: string }>())

  const gaps = computed<RangeGap[]>(() => explanation.value?.gaps ?? [])
  /** A stored explanation with no gaps is an answer, not an empty screen. */
  const isEmpty = computed(
    () => explanation.value !== null && gaps.value.length === 0,
  )

  const rangeLabel = computed(() => `páginas ${target.from}-${target.to}`)

  /** The page a gap is stated on, as a citation and not as a score. */
  function pageLabel(gap: RangeGap): string {
    return `página ${gap.page}`
  }

  /**
   * Explain this range again.
   *
   * The only way back to the form once an explanation is on screen, and it is the
   * reader's decision: the second run is a second model call, and it is what you want
   * after re-reading the chapter or after an answer that missed your point.
   */
  function startOver(): void {
    store.reset()
    text.value = ""
    fronts.clear()
    lastId = null
  }

  /**
   * The editable front of one gap's card, created from the gap's point on first read.
   *
   * It answers a stable object per gap id, so `v-model` in the row binds to one draft
   * across renders instead of a fresh one each time.
   */
  function front(gap: RangeGap): { text: string } {
    const existing = fronts.get(gap.id)
    if (existing) return existing

    const draft = reactive({ text: gap.point })
    fronts.set(gap.id, draft)
    return draft
  }

  /**
   * Every failure answered in the reader's language, except a refusal, which is the
   * server's own sentence verbatim (S4, S5).
   *
   * The client does not own a threshold: how wide a range may be and how thin an
   * explanation may be live in the server, which refuses before it calls the model,
   * so the sentence is immediate and a copied constant can only drift away from it.
   */
  const errorMessage = computed<string | null>(() => {
    const failure = error.value
    if (!failure) return null

    // Accepting a gap fails differently from reading or writing a state: the server's
    // refusals (`question is required`, `this gap already has a card`) are sentences the
    // reader acts on, so they are rendered verbatim. Only the two that carry no useful
    // sentence — a gap that is gone, a write that failed — are re-worded.
    if (failure.source === "card") {
      if (failure.status === 404) {
        return "Esa laguna ya no está guardada: recargá la explicación."
      }
      if (failure.status !== null && failure.status >= 500) {
        return "No se creó la tarjeta. Probá de nuevo."
      }
      return failure.message
    }

    if (failure.status === 404) {
      return failure.source === "load"
        ? "Esa explicación ya no está guardada. Escribí el rango de nuevo."
        : "Ese documento o ese rango no existe."
    }

    if (failure.status !== null && failure.status >= 500) {
      return failure.source === "load"
        ? "No se pudo leer la explicación guardada. Probá de nuevo."
        : "No se guardó nada: el modelo falló. Probá de nuevo si querés — es otra consulta al modelo."
    }

    // A refusal (400) and a dropped connection keep the wording they already have.
    return failure.message
  })

  /**
   * A retry is a decision, never automatic: after a model failure it costs another
   * call, and after a refusal the answer is more words, not another one.
   */
  const canRetry = computed(() => {
    const failure = error.value
    if (!failure) return false
    if (failure.source === "state") return false
    // The card control is the retry for a card write, and it keeps the reader's front.
    if (failure.source === "card") return false
    if (failure.status === 404 || failure.status === 400) return false
    return true
  })

  /**
   * Store the explanation and answer the id to put in the URL, or `null` when
   * nothing was stored.
   */
  async function submit(): Promise<string | null> {
    // Blank is not an explanation, and the server says so too — this only keeps the
    // page from spending a round trip to hear it.
    if (text.value.trim() === "") return null

    const stored = await store.explain({
      document_id: target.documentId,
      from_page: target.from,
      to_page: target.to,
      text: text.value,
    })

    return stored ? stored.id : null
  }

  /** Read a stored explanation: what a deep link and a reload both pay. */
  async function load(id: string): Promise<boolean> {
    lastId = id
    return store.load(id)
  }

  /** Re-read after a failure, at the reader's command. */
  async function retry(): Promise<void> {
    if (error.value?.source === "load" && lastId !== null) {
      await load(lastId)
      return
    }

    await submit()
  }

  /**
   * Accept this gap into a card, with the front as the reader edited it (S6).
   *
   * A blank front does not spend a round trip: the server refuses it with the same
   * sentence, and the page already knows. On success the row renders the state the
   * server stored, so nothing on the page claims a card a failed write never made.
   */
  async function accept(gap: RangeGap): Promise<boolean> {
    const question = front(gap).text.trim()
    if (question === "") return false

    return store.acceptGap(gap, question)
  }

  /**
   * The one discard control on a gap sends the state opposite to the one it shows.
   *
   * A carded gap is not discardable: the state only moves forward, so the page never
   * offers the control for it and this guard keeps a stray call from trying.
   */
  function toggleGap(gap: RangeGap): Promise<boolean> {
    if (isCarded(gap)) return Promise.resolve(false)

    return store.setGapState(gap, isDiscarded(gap) ? "open" : "discarded")
  }

  function stateOf(gap: RangeGap): GapState {
    return store.stateOf(gap)
  }

  function isDiscarded(gap: RangeGap): boolean {
    return store.stateOf(gap) === "discarded"
  }

  function isCarded(gap: RangeGap): boolean {
    return store.stateOf(gap) === "carded"
  }

  return {
    explanation,
    loading,
    saving,
    error,
    text,
    gaps,
    isEmpty,
    rangeLabel,
    errorMessage,
    canRetry,
    pageLabel,
    front,
    submit,
    load,
    retry,
    startOver,
    accept,
    toggleGap,
    setGapState: store.setGapState,
    stateOf,
    isDiscarded,
    isCarded,
  }
}
