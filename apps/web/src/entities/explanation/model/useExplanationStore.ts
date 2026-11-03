import { defineStore } from "pinia"
import { ref } from "vue"
import type {
  Explanation,
  ExplanationFailure,
  ExplanationFailureSource,
  ExplanationResponse,
  ExplainRangeRequest,
  GapState,
  RangeGap,
  RangeGapResponse,
  SettableGapState,
} from "./types"
import {
  acceptGap as postAcceptGap,
  explainRange,
  fetchExplanation,
  setGapState as putGapState,
} from "../api/explanation-api"
import { ApiError } from "@/shared/api/client"

/** Resolve one wire gap into the shape the page renders, once. */
function toGap(dto: RangeGapResponse): RangeGap {
  return {
    id: dto.gap_id,
    point: dto.point,
    page: dto.page,
    quote: dto.quote,
    state: dto.state,
  }
}

/** One mapping for both routes: a gap created and a gap read are the same gap. */
function toExplanation(dto: ExplanationResponse): Explanation {
  return {
    id: dto.explanation_id,
    documentId: dto.document_id,
    fromPage: dto.from_page,
    toPage: dto.to_page,
    gaps: dto.gaps.map(toGap),
  }
}

const FALLBACK: Record<ExplanationFailureSource, string> = {
  explain: "Error desconocido al explicar el rango.",
  load: "Error desconocido al leer la explicación.",
  state: "Error desconocido al guardar la laguna.",
  card: "Error desconocido al crear la tarjeta.",
}

/**
 * Keep the status next to the message: the page has to tell a refusal from a stale
 * link, an unknown document and a model failure, and the sentence alone is not a
 * contract (`ExplanationFailure`).
 */
function toFailure(
  e: unknown,
  source: ExplanationFailureSource,
): ExplanationFailure {
  if (e instanceof ApiError) {
    return { message: e.message, status: e.status, source }
  }

  if (e instanceof Error) {
    return { message: e.message, status: null, source }
  }

  return { message: FALLBACK[source], status: null, source }
}

/**
 * One range, one explanation, and the gaps it left (S1, S2, S4, S5, S6, S7, S9).
 *
 * Two flags, because they are two different waits. `loading` is the read of a stored
 * explanation: fast, and what a deep link pays on mount. `saving` is a write — the
 * submit, which is the 30-to-60-second model call, or a gap's state. The form and the
 * gap list never render together, so the two writes cannot be in flight at once.
 */
export const useExplanationStore = defineStore("explanation", () => {
  /** `null` until a run is stored or read back; "not loaded" is not "no gaps". */
  const explanation = ref<Explanation | null>(null)
  /**
   * The states this session had the server confirm, by gap id.
   *
   * A gap's point, page and quote are stored verbatim and cannot change, so the only
   * thing a write can move is its state — and only an accepted answer moves it. That
   * is why a refused write leaves the row exactly as it was (`R4-retry-click-fanout`
   * is the same rule on the launcher's retry).
   */
  const gapStates = ref(new Map<string, GapState>())
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<ExplanationFailure | null>(null)

  /**
   * Store the reader's explanation and read the gaps it left (S1, S4, S5).
   *
   * It answers the stored explanation, or `null` when the run was refused or the
   * model failed: the page needs the new id to put it in the URL, and it must not
   * have one to put there after a failure.
   */
  async function explain(
    req: ExplainRangeRequest,
  ): Promise<Explanation | null> {
    // A second click is a second model call, and a disabled attribute is not a lock.
    if (saving.value) return null

    saving.value = true
    error.value = null

    try {
      const stored = toExplanation(await explainRange(req))
      explanation.value = stored
      gapStates.value = new Map()
      return stored
    } catch (e) {
      error.value = toFailure(e, "explain")
      return null
    } finally {
      saving.value = false
    }
  }

  /**
   * Read a stored explanation by id, calling no model (S9).
   *
   * A stale link is the one failure that clears the screen: there is no explanation
   * behind that id, so the page has to offer the form again instead of rendering
   * gaps it never received.
   */
  async function load(id: string): Promise<boolean> {
    loading.value = true
    error.value = null

    try {
      explanation.value = toExplanation(await fetchExplanation(id))
      gapStates.value = new Map()
      return true
    } catch (e) {
      explanation.value = null
      error.value = toFailure(e, "load")
      return false
    } finally {
      loading.value = false
    }
  }

  /**
   * Discard a gap, or return it to `open` (S7), rendering the state the server kept.
   *
   * Discarding is a state and not a deletion: the row stays, its evidence stays, and
   * the only thing that changes is the word beside it.
   */
  async function setGapState(
    gap: RangeGap,
    state: SettableGapState,
  ): Promise<boolean> {
    if (saving.value) return false

    saving.value = true
    error.value = null

    try {
      const stored = await putGapState(gap.id, state)
      gapStates.value.set(stored.gap_id, stored.state)
      return true
    } catch (e) {
      error.value = toFailure(e, "state")
      return false
    } finally {
      saving.value = false
    }
  }

  /**
   * Accept a gap into a card, with the reader's question as its front (S6, S10).
   *
   * One request writes the card and moves the gap to `carded`, and only the answer's
   * state is rendered: the front is the reader's own, and the back is the quote the
   * row already cites, so echoing either back would add nothing. A refusal leaves the
   * row exactly as it was — the state is the server's claim, and nothing claims a card
   * a failed write never stored.
   */
  async function acceptGap(gap: RangeGap, question: string): Promise<boolean> {
    if (saving.value) return false

    saving.value = true
    error.value = null

    try {
      const stored = await postAcceptGap(gap.id, question)
      gapStates.value.set(stored.gap_id, stored.state)
      return true
    } catch (e) {
      error.value = toFailure(e, "card")
      return false
    } finally {
      saving.value = false
    }
  }

  /**
   * Clear the stored explanation so the same range can be explained again.
   * Deliberate and never automatic: a second explanation is a second model call, so
   * the reader asks for it — after re-reading the chapter, or because the answer
   * missed what they meant. Without this, the form would be unreachable in any URL
   * that already names a stored explanation.
   */
  function reset(): void {
    explanation.value = null
    gapStates.value = new Map()
    error.value = null
  }

  /** The gap's state as the server last said; the wire's own state until then. */
  function stateOf(gap: RangeGap): GapState {
    return gapStates.value.get(gap.id) ?? gap.state
  }

  return {
    explanation,
    gapStates,
    loading,
    saving,
    error,
    explain,
    load,
    setGapState,
    acceptGap,
    stateOf,
    reset,
  }
})
