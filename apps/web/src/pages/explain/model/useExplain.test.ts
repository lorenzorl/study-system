import { beforeEach, describe, expect, it, vi } from "vitest"
import { createPinia, setActivePinia } from "pinia"
import { ApiError } from "@/shared/api/client"
import type {
  ExplanationResponse,
  RangeGapResponse,
} from "@/entities/explanation"
import { useExplain } from "./useExplain"

// The page's model is where every state that needs an answer lives: what goes on
// the wire, what a refusal keeps, what a stale link does, and what a gap write
// leaves behind when it is refused. A render test cannot reach any of it — the
// suite renders through `vue/server-renderer`, so there is no event loop to type
// into the textarea or click a button with.
//
// The api module is mocked through `vi.hoisted` instead of an import of its path:
// `entities/explanation/api/explanation-api` is a segment, and the boundary guard
// reads every specifier in this file (`ARCHITECTURE.md` §10).
const api = vi.hoisted(() => ({
  explainRange: vi.fn(),
  fetchExplanation: vi.fn(),
  setGapState: vi.fn(),
}))

vi.mock("@/entities/explanation/api/explanation-api", () => api)

/** The range the route names: `/explain/doc-1/15/23`. */
const RANGE = { documentId: "doc-1", from: 15, to: 23 }

const TEXT =
  "El capítulo cuenta cómo el equipo construyó el modelo hablando con los expertos, y qué cambió cuando lo escribieron."

/** `POST /api/study/explanation` and `GET /api/study/explanations/{id}` share it. */
const STORED: ExplanationResponse = {
  explanation_id: "exp-1",
  document_id: "doc-1",
  from_page: 15,
  to_page: 23,
  gaps: [
    {
      gap_id: "g1",
      point: "El prototipo ata modelo e implementación.",
      page: 18,
      quote: "That crude prototype forged the essential link.",
      state: "open",
    },
  ],
}

/** The refusal S4 names, verbatim from the server (`api.md`). */
const TOO_SHORT =
  "this range is 9 pages and your explanation covers 62 characters: explain one section, or say more"

const DISCARDED: RangeGapResponse = { ...STORED.gaps[0]!, state: "discarded" }
const REOPENED: RangeGapResponse = { ...STORED.gaps[0]!, state: "open" }

/** The stored gap in the domain shape the page renders. */
function storedGap(model: ReturnType<typeof useExplain>) {
  const gap = model.gaps.value[0]
  if (!gap) throw new Error("the fixture has no gap")
  return gap
}

beforeEach(() => {
  setActivePinia(createPinia())
  api.explainRange.mockReset()
  api.fetchExplanation.mockReset()
  api.setGapState.mockReset()
  api.explainRange.mockResolvedValue(STORED)
  api.fetchExplanation.mockResolvedValue(STORED)
  api.setGapState.mockResolvedValue(DISCARDED)
})

describe("useExplain", () => {
  it("names the range in the reader's words", () => {
    const model = useExplain(RANGE)

    expect(model.rangeLabel.value).toBe("páginas 15-23")
    expect(model.pageLabel(storedGapAfter(STORED))).toBe("página 18")
  })

  // S4/S5: one submit is one model call, with the range and the reader's words.
  it("submits the range and the text, and answers the stored id", async () => {
    const model = useExplain(RANGE)
    model.text.value = TEXT

    const id = await model.submit()

    expect(api.explainRange).toHaveBeenCalledWith({
      document_id: "doc-1",
      from_page: 15,
      to_page: 23,
      text: TEXT,
    })
    expect(id).toBe("exp-1")
    expect(model.error.value).toBeNull()
  })

  // The way back to the form after an explanation is on screen. Deliberate: the
  // second run is a second model call, and it is the reader who asks for it.
  it("starts the range over, clearing the explanation and the text", async () => {
    const model = useExplain(RANGE)
    model.text.value = TEXT
    await model.submit()
    expect(model.explanation.value).not.toBeNull()

    model.startOver()

    expect(model.explanation.value).toBeNull()
    expect(model.text.value).toBe("")
    expect(model.gaps.value).toHaveLength(0)
    expect(model.error.value).toBeNull()
  })

  it("does not submit a blank explanation", async () => {
    const model = useExplain(RANGE)
    model.text.value = "   "

    expect(await model.submit()).toBeNull()
    expect(api.explainRange).not.toHaveBeenCalled()
  })

  // "Disable the submit while in flight so two clicks cannot be two model calls":
  // the button is disabled in the page, and the store refuses the second call too,
  // because a disabled attribute is not a lock.
  it("does not start a second submit while the first is in flight", async () => {
    let release: (value: ExplanationResponse) => void = () => {}
    api.explainRange.mockReturnValue(
      new Promise<ExplanationResponse>((resolve) => {
        release = resolve
      }),
    )
    const model = useExplain(RANGE)
    model.text.value = TEXT

    const first = model.submit()
    const second = model.submit()

    expect(api.explainRange).toHaveBeenCalledTimes(1)
    release(STORED)
    expect(await second).toBeNull()
    expect(await first).toBe("exp-1")
  })

  // S4: the refusal is the server's sentence, verbatim, and the threshold stays
  // in the server — a copied constant is a constant that drifts. What the reader
  // wrote is the one thing this page must never lose.
  it("keeps the text and shows the server's sentence when it is refused", async () => {
    api.explainRange.mockRejectedValue(new ApiError(TOO_SHORT, 400))
    const model = useExplain(RANGE)
    model.text.value = TEXT

    expect(await model.submit()).toBeNull()

    expect(model.errorMessage.value).toBe(TOO_SHORT)
    expect(model.text.value).toBe(TEXT)
    expect(model.canRetry.value).toBe(false)
  })

  // S5: a range too wide to explain is refused with a sentence naming the split.
  // It renders like any other refusal, so it never becomes a spinner.
  it("renders the wide-range refusal as the server's sentence", async () => {
    const refusal =
      "this range is 30 pages and explaining a range is limited to 12: explain one section at a time"
    api.explainRange.mockRejectedValue(new ApiError(refusal, 400))
    const model = useExplain(RANGE)
    model.text.value = TEXT

    await model.submit()

    expect(model.errorMessage.value).toBe(refusal)
  })

  // A 500 means the model failed and nothing was stored. Trying again is another
  // model call, so it is a button the reader presses, never a silent retry.
  it("says nothing was stored after a model failure and offers a retry", async () => {
    api.explainRange.mockRejectedValue(new ApiError("internal server error", 500))
    const model = useExplain(RANGE)
    model.text.value = TEXT

    await model.submit()

    expect(model.errorMessage.value).toContain("No se guardó nada")
    expect(model.canRetry.value).toBe(true)
    // Nothing happened on its own: the first failure is the only call.
    expect(api.explainRange).toHaveBeenCalledTimes(1)

    await model.retry()

    expect(api.explainRange).toHaveBeenCalledTimes(2)
  })

  // S9: a deep link reads what the run stored, and never pays for the model again.
  it("reads a stored explanation by id without submitting", async () => {
    const model = useExplain(RANGE)

    expect(await model.load("exp-1")).toBe(true)

    expect(api.fetchExplanation).toHaveBeenCalledWith("exp-1")
    expect(api.explainRange).not.toHaveBeenCalled()
    expect(model.explanation.value?.id).toBe("exp-1")
    expect(model.gaps.value).toHaveLength(1)
  })

  // S9, the stale link: a 404 is not a refusal of the reader's words, so the page
  // says the explanation is gone and offers the form again.
  it("offers the form when a stale link 404s", async () => {
    api.fetchExplanation.mockRejectedValue(new ApiError("not found", 404))
    const model = useExplain(RANGE)

    expect(await model.load("gone")).toBe(false)

    expect(model.explanation.value).toBeNull()
    expect(model.errorMessage.value).toContain("ya no está guardada")
    expect(model.canRetry.value).toBe(false)
  })

  it("says the document or range is unknown when a submit 404s", async () => {
    api.explainRange.mockRejectedValue(new ApiError("not found", 404))
    const model = useExplain(RANGE)
    model.text.value = TEXT

    await model.submit()

    expect(model.errorMessage.value).toContain("no existe")
    expect(model.canRetry.value).toBe(false)
  })

  // S7: discarding is a state, so the row is the answer's — read it back from
  // there, not from what the page hoped the write did.
  it("discards a gap and reopens it, rendering the state the server stored", async () => {
    const model = useExplain(RANGE)
    await model.load("exp-1")
    const gap = storedGap(model)

    await model.setGapState(gap, "discarded")

    expect(api.setGapState).toHaveBeenCalledWith("g1", "discarded")
    expect(model.stateOf(gap)).toBe("discarded")
    expect(model.isDiscarded(gap)).toBe(true)

    api.setGapState.mockResolvedValue(REOPENED)

    await model.setGapState(gap, "open")

    expect(api.setGapState).toHaveBeenLastCalledWith("g1", "open")
    expect(model.stateOf(gap)).toBe("open")
    expect(model.isDiscarded(gap)).toBe(false)
  })

  it("toggles a gap from its own state", async () => {
    const model = useExplain(RANGE)
    await model.load("exp-1")
    const gap = storedGap(model)

    await model.toggleGap(gap)
    expect(api.setGapState).toHaveBeenLastCalledWith("g1", "discarded")

    api.setGapState.mockResolvedValue(REOPENED)

    await model.toggleGap(gap)
    expect(api.setGapState).toHaveBeenLastCalledWith("g1", "open")
  })

  // The failed write must not move the row: only an accepted answer can.
  it("leaves the row as it was when a gap write is refused", async () => {
    api.setGapState.mockRejectedValue(
      new ApiError("state must be discarded or open", 400),
    )
    const model = useExplain(RANGE)
    await model.load("exp-1")
    const gap = storedGap(model)

    expect(await model.setGapState(gap, "discarded")).toBe(false)

    expect(model.stateOf(gap)).toBe("open")
    expect(model.errorMessage.value).toBe("state must be discarded or open")
    expect(model.saving.value).toBe(false)
  })

  // S8: the wire has no score, and the page does not invent one.
  it("carries no comprehension score", async () => {
    const model = useExplain(RANGE)
    await model.load("exp-1")

    expect(Object.keys(model.explanation.value ?? {})).toEqual([
      "id",
      "documentId",
      "fromPage",
      "toPage",
      "gaps",
    ])
    expect(Object.keys(storedGap(model))).toEqual([
      "id",
      "point",
      "page",
      "quote",
      "state",
    ])
  })
})

/** The domain shape of one wire gap, without a store: the label needs no state. */
function storedGapAfter(dto: ExplanationResponse) {
  const gap = dto.gaps[0]
  if (!gap) throw new Error("the fixture has no gap")
  return {
    id: gap.gap_id,
    point: gap.point,
    page: gap.page,
    quote: gap.quote,
    state: gap.state,
  }
}
