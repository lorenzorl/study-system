import type { Flashcard } from "./types";

export function isValidFlashcard(card: Flashcard): boolean {
  return (
    isNonEmptyString(card.id) &&
    isNonEmptyString(card.question) &&
    isNonEmptyString(card.answer)
  );
}

function isNonEmptyString(value: string): boolean {
  return typeof value === "string" && value.trim().length > 0;
}

export function nextIndex(current: number, total: number): number {
  return (current + 1) % total;
}

export function prevIndex(current: number, total: number): number {
  return (current - 1 + total) % total;
}
