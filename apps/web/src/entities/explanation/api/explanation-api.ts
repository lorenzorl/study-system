import type {
  AcceptGapRequest,
  AcceptGapResponse,
  ExplanationResponse,
  ExplainRangeRequest,
  RangeGapResponse,
  SettableGapState,
} from "../model/types"
import { request } from "@/shared/api/client"

/**
 * Explain a page range in the reader's own words (S1, S4, S5).
 *
 * The model runs synchronously behind this call, so it takes 30 to 60 seconds and
 * the refusal — a range too wide, an explanation too thin — arrives as a 400 with
 * the sentence that says what to do instead. Nothing is scored and nothing is kept
 * from a failed run.
 */
export async function explainRange(
  req: ExplainRangeRequest,
): Promise<ExplanationResponse> {
  return request<ExplanationResponse>("/api/study/explanation", {
    method: "POST",
    body: JSON.stringify(req),
  })
}

/**
 * Read what a run stored, without paying for the model again (S9).
 *
 * This is what makes the page deep-linkable: the id in the URL is enough to render
 * the gaps, and a reload answers with the states already stored — including the gaps
 * the reader discarded.
 */
export async function fetchExplanation(
  id: string,
): Promise<ExplanationResponse> {
  return request<ExplanationResponse>(
    `/api/study/explanations/${encodeURIComponent(id)}`,
  )
}

/**
 * Discard one gap, or return a discarded one to `open` (S7).
 *
 * The answer is the gap as stored, so the row renders the server's state instead of
 * the one the click hoped for. `carded` is deliberately not settable here: only the
 * route that writes the card may say a card exists.
 */
export async function setGapState(
  gapId: string,
  state: SettableGapState,
): Promise<RangeGapResponse> {
  return request<RangeGapResponse>(
    `/api/study/gaps/${encodeURIComponent(gapId)}/state`,
    {
      method: "PUT",
      body: JSON.stringify({ state }),
    },
  )
}

/**
 * Accept one gap into a card: the reader's question as the front, the quote and its
 * page as the back (S6, S10).
 *
 * One call, because the server owns the whole write: it derives the topic and the
 * concept, writes the card through the existing flashcards path, and marks the gap
 * `carded` last. The client sends the front and reads the state; it never names a
 * concept, and it cannot reorder those writes.
 *
 * The refusals are sentences the page renders verbatim: 400 `question is required`,
 * 409 `this gap already has a card` — the accept happened once, and this is not a
 * second card — and 404 for a gap that is gone.
 */
export async function acceptGap(
  gapId: string,
  question: string,
): Promise<AcceptGapResponse> {
  const body: AcceptGapRequest = { question }

  return request<AcceptGapResponse>(
    `/api/study/gaps/${encodeURIComponent(gapId)}/card`,
    {
      method: "POST",
      body: JSON.stringify(body),
    },
  )
}
