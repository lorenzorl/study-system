import type { Flashcard } from "./types"
import { isNonEmptyString } from "@/shared/lib/guards"

export function isValidFlashcard(card: Flashcard): boolean {
  return (
    isNonEmptyString(card.id) &&
    isNonEmptyString(card.concept_id) &&
    isNonEmptyString(card.question) &&
    isNonEmptyString(card.answer) &&
    isNonEmptyString(card.obsidian_id) &&
    isNonEmptyString(card.created_at)
  )
}
