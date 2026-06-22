import type { Flashcard, Domain, Concept, StudyMetrics } from "./types";

export function isValidFlashcard(card: Flashcard): boolean {
  return (
    isNonEmptyString(card.id) &&
    isNonEmptyString(card.question) &&
    isNonEmptyString(card.answer)
  );
}

export function isValidDomain(domain: Domain): boolean {
  return (
    isNonEmptyString(domain.id) &&
    isNonEmptyString(domain.name) &&
    isNonEmptyString(domain.description) &&
    Array.isArray(domain.concepts) &&
    domain.concepts.every((c) => isValidConcept(c))
  );
}

export function isValidConcept(concept: Concept): boolean {
  return (
    isNonEmptyString(concept.id) &&
    isNonEmptyString(concept.name) &&
    isNonEmptyString(concept.summary) &&
    Array.isArray(concept.flashcards) &&
    concept.flashcards.every((f) => isValidFlashcard(f))
  );
}

export function isValidStudyMetrics(metrics: StudyMetrics): boolean {
  return (
    isNonNegativeInteger(metrics.dailyCardCount) &&
    isRate(metrics.retentionRate) &&
    isNonNegativeInteger(metrics.currentStreak) &&
    isNonNegativeInteger(metrics.totalReviewed)
  );
}

function isNonEmptyString(value: string): boolean {
  return typeof value === "string" && value.trim().length > 0;
}

function isNonNegativeInteger(value: number): boolean {
  return Number.isInteger(value) && value >= 0;
}

function isRate(value: number): boolean {
  return typeof value === "number" && value >= 0 && value <= 1;
}

export function nextIndex(current: number, total: number): number {
  return (current + 1) % total;
}

export function prevIndex(current: number, total: number): number {
  return (current - 1 + total) % total;
}
