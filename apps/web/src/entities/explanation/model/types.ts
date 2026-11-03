/**
 * The wire of a range explanation, and the page's own view of it (S1, S2, S4, S8).
 *
 * The response is stored, not generated: `POST` answers the explanation it kept and
 * `GET /api/study/explanations/{id}` answers the same shape without calling the
 * model, so one mapping serves both and a reload cannot disagree with the run that
 * produced it.
 *
 * There is no comprehension score anywhere in these types. A number on an open
 * response is noisy and invites writing for the grader (S8), so the wire does not
 * carry one and the domain shape cannot invent it.
 */

/** What a stored gap can be. `carded` says a card exists and only U3's route may set it. */
export type GapState = "open" | "discarded" | "carded"

/** The two states `PUT /api/study/gaps/{gapID}/state` accepts. */
export type SettableGapState = "open" | "discarded"

/** One gap as the handler writes it: the point, the page that states it, the quote. */
export interface RangeGapResponse {
  gap_id: string
  point: string
  page: number
  quote: string
  state: GapState
}

/** The body `POST /api/study/explanation` and `GET /api/study/explanations/{id}` share. */
export interface ExplanationResponse {
  explanation_id: string
  document_id: string
  from_page: number
  to_page: number
  gaps: RangeGapResponse[]
}

/** `POST /api/study/explanation`: the reader's words, verbatim (S1). */
export interface ExplainRangeRequest {
  document_id: string
  from_page: number
  to_page: number
  text: string
}

/** `PUT /api/study/gaps/{gapID}/state`. */
export interface SetGapStateRequest {
  state: SettableGapState
}

/** A gap in the shape the page renders: addressable, with its state (S2). */
export interface RangeGap {
  id: string
  point: string
  page: number
  /** Verbatim in the book's language: a translated quote no longer proves the page. */
  quote: string
  state: GapState
}

/** The stored explanation, resolved once from the wire. The reader's text is not
 * part of it: the read route answers the gaps, and the words are re-typed or kept by
 * the page that submitted them. */
export interface Explanation {
  id: string
  documentId: string
  fromPage: number
  toPage: number
  gaps: RangeGap[]
}

/** Which request produced a failure. The page words each one differently. */
export type ExplanationFailureSource = "explain" | "load" | "state"

/**
 * A failed request, kept with the status that decided it.
 *
 * A plain message is not enough here: the page has to tell a refusal (400, the
 * server's own sentence) from a stale link (404 on a read), an unknown document
 * (404 on a submit) and a model failure (500, nothing stored, another call to
 * retry). Those are five distinguishable states, and the status is the only thing
 * that distinguishes them without copying the server's thresholds into the client.
 */
export interface ExplanationFailure {
  /** The server's `error` sentence when it sent one, the client's own otherwise. */
  message: string
  /** The HTTP status, or `null` when no answer came back at all. */
  status: number | null
  source: ExplanationFailureSource
}
