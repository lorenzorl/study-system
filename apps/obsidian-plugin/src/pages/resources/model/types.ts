/**
 * Local view model for the resource launcher (`PRESENTATION-UI.md` §2, §8.6).
 *
 * This is deliberately *not* `entities/resource`: promoting the launcher's model
 * to the entity layer is deferred until a data source exists, so the type stays
 * inside the page slice.
 */

export type ResourceKind = "flashcards" | "deck" | "exam"

/** Filter state: every kind, or exactly one of them. */
export type ResourceKindFilter = "all" | ResourceKind

interface ResourceBase {
  id: string
  title: string
  /** Context line on the card (`Atención ›`), and part of the search index. */
  topicTitle: string
  /** Never rendered on the card; searched by `filterResources`. */
  conceptTitle: string
  primaryActionLabel: string
}

export interface FlashcardResource extends ResourceBase {
  kind: "flashcards"
  due: number
  total: number
}

export interface DeckResource extends ResourceBase {
  kind: "deck"
  slides: number
  updatedDaysAgo: number
}

export interface ExamResource extends ResourceBase {
  kind: "exam"
  questions: number
  lastScore: number | null
  of: number | null
}

/** One card in the launcher; each kind reports its own status (§2). */
export type Resource = FlashcardResource | DeckResource | ExamResource

/** The "continue" zone shows at most one item (§6), so this is not a list. */
export interface ContinueResource {
  id: string
  /** The activity being continued, e.g. `Repaso`. */
  kindLabel: string
  title: string
  done: number
  total: number
  primaryActionLabel: string
}
