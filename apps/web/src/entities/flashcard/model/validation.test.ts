import { describe, it, expect } from "vitest"
import { isValidFlashcard } from "./validation"
import type { Flashcard } from "./types"

// ─── Flashcard Validation ──────────────────────────────────────────────────

describe("isValidFlashcard", () => {
  const validCard: Flashcard = {
    id: "test-1",
    concept_id: "concept-1",
    question: "What is 2 + 2?",
    answer: "4",
    obsidian_id: "obs-1",
    created_at: "2024-01-01T00:00:00Z",
  }

  it("returns true for a valid flashcard", () => {
    expect(isValidFlashcard(validCard)).toBe(true)
  })

  it("returns false when id is empty", () => {
    expect(isValidFlashcard({ ...validCard, id: "" })).toBe(false)
  })

  it("returns false when concept_id is empty", () => {
    expect(isValidFlashcard({ ...validCard, concept_id: "" })).toBe(false)
  })

  it("returns false when question is empty", () => {
    expect(isValidFlashcard({ ...validCard, question: "" })).toBe(false)
  })

  it("returns false when answer is empty", () => {
    expect(isValidFlashcard({ ...validCard, answer: "" })).toBe(false)
  })

  it("returns false when obsidian_id is empty", () => {
    expect(isValidFlashcard({ ...validCard, obsidian_id: "" })).toBe(false)
  })

  it("returns false when created_at is empty", () => {
    expect(isValidFlashcard({ ...validCard, created_at: "" })).toBe(false)
  })
})
