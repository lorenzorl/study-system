import type { Resource } from "./types"

/**
 * The launcher's deck action, expressed as a route instead of a callback
 * (`PRESENTATION-UI.md` §4.1: the card's primary action opens the deck tab).
 *
 * Kept as a pure decision so the "which kind gets which surface" rule is not
 * buried in the click handler: the launcher asks, this answers, and the caller
 * only navigates. Non-deck kinds return `null` — flashcards keep opening the
 * review modal and exams are still unrouted.
 */
export interface DeckRoute {
  name: "presentation"
  params: { deckId: string }
}

export function deckRouteFor(resource: Resource): DeckRoute | null {
  if (resource.kind !== "deck") return null

  return { name: "presentation", params: { deckId: resource.id } }
}
