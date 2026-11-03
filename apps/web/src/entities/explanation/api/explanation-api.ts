import type {
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
