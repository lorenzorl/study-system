import type { DueCard, ReviewRequest } from "./types"
import { isNonEmptyString } from "@/shared/lib/guards"

export function isValidDueCard(card: DueCard): boolean {
  return (
    isNonEmptyString(card.flashcard_id) &&
    isNonEmptyString(card.question) &&
    isNonEmptyString(card.answer) &&
    isNonEmptyString(card.concept_title) &&
    isNonEmptyString(card.topic_name) &&
    isNonEmptyString(card.next_review)
  )
}

// ─── ReviewRequest Validation ────────────────────────────────────────────────

const VALID_GRADES = new Set([1, 2, 3, 4])

export function isValidGrade(grade: number): boolean {
  return Number.isInteger(grade) && VALID_GRADES.has(grade)
}

export function validateGrade(grade: number): void {
  if (!isValidGrade(grade)) {
    throw new Error(`Invalid grade: ${grade}. Must be an integer 1-4.`)
  }
}

export function isValidReviewRequest(req: ReviewRequest): boolean {
  return (
    isNonEmptyString(req.flashcard_id) &&
    isValidGrade(req.grade) &&
    Number.isInteger(req.duration_ms) &&
    req.duration_ms >= 0
  )
}
