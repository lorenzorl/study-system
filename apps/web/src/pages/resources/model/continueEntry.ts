import type { DueCard } from "@/entities/review"
import type { ContinueResource } from "./types"

/**
 * The launcher's single "continue" entry (`PRESENTATION-UI.md` §6: at most one,
 * two would be a list), derived from what is actually due today.
 *
 * This is a decision, not a fetch: `GET /api/study/due` is the only endpoint that
 * reports a resumable session, and the review session is the only surface that
 * resumes it. Keeping the mapping pure is what lets the empty case be a unit test
 * instead of a screenshot of an empty section.
 *
 * `done` is always 0: the entry says how much is pending, and the session counts
 * from the beginning once it opens.
 *
 * There is no id: the zone holds at most one entry and the action is the review
 * route, so an identifier would be a field nothing consumes. The commit that adds a
 * second kind of resume is the commit that adds one.
 */
export function continueEntryFor(
  dueCards: readonly DueCard[],
): ContinueResource | null {
  if (dueCards.length === 0) return null

  return {
    kindLabel: "Repaso",
    title: "Pendientes de hoy",
    done: 0,
    total: dueCards.length,
    primaryActionLabel: "Repasar",
  }
}
