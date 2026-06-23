import type { Flashcard, Topic, ConceptSummary, StudyMetrics } from "./types"
import type { DueCard, ReviewRequest, SyncResourceRequest, ResourceType } from "./types"

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

export function isValidTopic(topic: Topic): boolean {
  return (
    isNonEmptyString(topic.id) &&
    isNonEmptyString(topic.name) &&
    Array.isArray(topic.concepts) &&
    topic.concepts.every((c) => isValidConceptSummary(c))
  )
}

export function isValidConceptSummary(concept: ConceptSummary): boolean {
  return (
    isNonEmptyString(concept.id) &&
    isNonEmptyString(concept.title) &&
    isNonEmptyString(concept.file_path)
  )
}

export function isValidStudyMetrics(metrics: StudyMetrics): boolean {
  return (
    isNonNegativeInteger(metrics.dailyCardCount) &&
    isRate(metrics.retentionRate) &&
    isNonNegativeInteger(metrics.currentStreak) &&
    isNonNegativeInteger(metrics.totalReviewed)
  )
}

function isNonEmptyString(value: string): boolean {
  return typeof value === "string" && value.trim().length > 0
}

function isNonNegativeInteger(value: number): boolean {
  return Number.isInteger(value) && value >= 0
}

function isRate(value: number): boolean {
  return typeof value === "number" && value >= 0 && value <= 1
}

export function nextIndex(current: number, total: number): number {
  return (current + 1) % total
}

export function prevIndex(current: number, total: number): number {
  return (current - 1 + total) % total
}

// ─── DueCard Validation ──────────────────────────────────────────────────────

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

// ─── SyncResourceRequest Validation ──────────────────────────────────────────

const VALID_RESOURCE_TYPES: ReadonlySet<string> = new Set([
  "book",
  "note",
  "article",
  "video",
])

export function isValidResourceType(type: string): type is ResourceType {
  return VALID_RESOURCE_TYPES.has(type)
}

export function isValidSyncResourceRequest(req: SyncResourceRequest): boolean {
  return (
    isNonEmptyString(req.topic_name) &&
    isNonEmptyString(req.resource_title) &&
    isValidResourceType(req.type) &&
    isNonEmptyString(req.source_uri)
  )
}
