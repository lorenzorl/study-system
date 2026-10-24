/**
 * The route a successful sync lands the reader on, expressed as a pure decision
 * (`pages/resources/model/deckRoute.ts` set the precedent): the page asks, this
 * answers, and the caller only navigates.
 *
 * The flow also reports which topic the synced concept belongs to, resolved
 * against the reloaded topic list. When that reload could not resolve the name
 * there is no topic id to build this route with, so it refuses instead of
 * navigating to a page with a `null` param.
 */
export interface SyncedConceptRoute {
  name: "flashcards"
  params: { topicId: string; conceptId: string }
}

export function flashcardsRouteFor(result: {
  conceptId: string
  topicId: string | null
}): SyncedConceptRoute | null {
  if (result.topicId === null) return null

  return {
    name: "flashcards",
    params: { topicId: result.topicId, conceptId: result.conceptId },
  }
}
