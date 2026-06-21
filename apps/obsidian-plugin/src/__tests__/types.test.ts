import { describe, it, expect } from "vitest";
import { SAMPLE_FLASHCARDS } from "../data";
import { isValidFlashcard } from "../validation";
import type { Flashcard } from "../types";

describe("Flashcard data integrity", () => {
  it("has at least one sample flashcard", () => {
    expect(SAMPLE_FLASHCARDS.length).toBeGreaterThan(0);
  });

  it("every sample flashcard passes validation", () => {
    const results = SAMPLE_FLASHCARDS.map((card) => ({
      id: card.id,
      valid: isValidFlashcard(card),
    }));
    expect(results.every((r) => r.valid)).toBe(true);
  });

  it("every sample flashcard has a non-empty id", () => {
    expect(SAMPLE_FLASHCARDS.every((card) => card.id.trim().length > 0)).toBe(
      true,
    );
  });

  it("every sample flashcard has a non-empty question", () => {
    expect(
      SAMPLE_FLASHCARDS.every((card) => card.question.trim().length > 0),
    ).toBe(true);
  });

  it("every sample flashcard has a non-empty answer", () => {
    expect(
      SAMPLE_FLASHCARDS.every((card) => card.answer.trim().length > 0),
    ).toBe(true);
  });

  it("every sample flashcard id is unique", () => {
    const ids = SAMPLE_FLASHCARDS.map((card) => card.id);
    expect(new Set(ids).size).toBe(ids.length);
  });
});

describe("isValidFlashcard", () => {
  const validCard: Flashcard = {
    id: "test-1",
    question: "What is 2 + 2?",
    answer: "4",
  };

  it("returns true for a valid flashcard", () => {
    expect(isValidFlashcard(validCard)).toBe(true);
  });

  it("returns false when id is empty", () => {
    expect(isValidFlashcard({ ...validCard, id: "" })).toBe(false);
  });

  it("returns false when id is whitespace only", () => {
    expect(isValidFlashcard({ ...validCard, id: "   " })).toBe(false);
  });

  it("returns false when question is empty", () => {
    expect(isValidFlashcard({ ...validCard, question: "" })).toBe(false);
  });

  it("returns false when question is whitespace only", () => {
    expect(isValidFlashcard({ ...validCard, question: "\t\n " })).toBe(false);
  });

  it("returns false when answer is empty", () => {
    expect(isValidFlashcard({ ...validCard, answer: "" })).toBe(false);
  });

  it("returns false when answer is whitespace only", () => {
    expect(isValidFlashcard({ ...validCard, answer: "   " })).toBe(false);
  });

  it("returns true when tags are present (optional field)", () => {
    expect(
      isValidFlashcard({ ...validCard, tags: ["javascript", "basics"] }),
    ).toBe(true);
  });
});
