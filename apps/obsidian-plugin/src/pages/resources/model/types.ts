/**
 * Local view model for the resource launcher (`PRESENTATION-UI.md` §2, §8.6).
 *
 * This is deliberately *not* `entities/resource`: the entity owns the wire shape
 * of `GET /api/resources`, while the launcher's model owns how a card is chosen,
 * filtered and rendered. Promoting it is deferred until a second surface reads it.
 */

import type { ResourceType } from "@/entities/resource"

export type ResourceKind = "flashcards" | "deck" | "exam" | "material"

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

/**
 * A stored study resource: a book, a note, an article or a video. All four map to
 * one kind because the launcher does the same thing with each — it opens the
 * source — and the type is a label in the status line, not a fifth surface.
 */
export interface MaterialResource extends ResourceBase {
  kind: "material"
  /** The stored type, rendered as the card's label. */
  resourceType: ResourceType
  /** The URI the primary action opens. */
  sourceUri: string
  /** Whole days since creation; `null` when the server's timestamp is unreadable. */
  addedDaysAgo: number | null
}

/** One card in the launcher; each kind reports its own status (§2). */
export type Resource =
  | FlashcardResource
  | DeckResource
  | ExamResource
  | MaterialResource

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
