import type { Flashcard, Topic, ConceptSummary, StudyMetrics } from "./types"

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
