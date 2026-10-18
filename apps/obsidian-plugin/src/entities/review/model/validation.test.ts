import { describe, it, expect } from "vitest"
import {
  isValidDueCard,
  isValidGrade,
  validateGrade,
  isValidReviewRequest,
} from "./validation"
import type { DueCard, ReviewRequest } from "./types"

// ─── DueCard Validation ──────────────────────────────────────────────────────

describe("isValidDueCard", () => {
  const validCard: DueCard = {
    flashcard_id: "fc-1",
    question: "What is DDD?",
    answer: "Domain-Driven Design",
    concept_title: "DDD Introduction",
    topic_name: "Software Architecture",
    next_review: "2026-06-24T10:00:00Z",
  }

  it("returns true for a valid DueCard", () => {
    expect(isValidDueCard(validCard)).toBe(true)
  })

  it("returns false when flashcard_id is empty", () => {
    expect(isValidDueCard({ ...validCard, flashcard_id: "" })).toBe(false)
  })

  it("returns false when question is empty", () => {
    expect(isValidDueCard({ ...validCard, question: "" })).toBe(false)
  })

  it("returns false when answer is empty", () => {
    expect(isValidDueCard({ ...validCard, answer: "" })).toBe(false)
  })

  it("returns false when concept_title is empty", () => {
    expect(isValidDueCard({ ...validCard, concept_title: "" })).toBe(false)
  })

  it("returns false when topic_name is empty", () => {
    expect(isValidDueCard({ ...validCard, topic_name: "" })).toBe(false)
  })

  it("returns false when next_review is empty", () => {
    expect(isValidDueCard({ ...validCard, next_review: "" })).toBe(false)
  })
})

// ─── Grade Validation ────────────────────────────────────────────────────────

describe("isValidGrade", () => {
  it("returns true for grade 1", () => {
    expect(isValidGrade(1)).toBe(true)
  })

  it("returns true for grade 2", () => {
    expect(isValidGrade(2)).toBe(true)
  })

  it("returns true for grade 3", () => {
    expect(isValidGrade(3)).toBe(true)
  })

  it("returns true for grade 4", () => {
    expect(isValidGrade(4)).toBe(true)
  })

  it("returns false for grade 0", () => {
    expect(isValidGrade(0)).toBe(false)
  })

  it("returns false for grade 5", () => {
    expect(isValidGrade(5)).toBe(false)
  })

  it("returns false for negative grade", () => {
    expect(isValidGrade(-1)).toBe(false)
  })

  it("returns false for non-integer grade", () => {
    expect(isValidGrade(2.5)).toBe(false)
  })
})

describe("validateGrade", () => {
  it("does not throw for valid grades 1-4", () => {
    expect(() => validateGrade(1)).not.toThrow()
    expect(() => validateGrade(2)).not.toThrow()
    expect(() => validateGrade(3)).not.toThrow()
    expect(() => validateGrade(4)).not.toThrow()
  })

  it("throws for grade 0", () => {
    expect(() => validateGrade(0)).toThrow("Invalid grade")
  })

  it("throws for grade 5", () => {
    expect(() => validateGrade(5)).toThrow("Invalid grade")
  })
})

// ─── ReviewRequest Validation ────────────────────────────────────────────────

describe("isValidReviewRequest", () => {
  const validReq: ReviewRequest = {
    flashcard_id: "fc-1",
    grade: 3,
    duration_ms: 5000,
  }

  it("returns true for a valid request", () => {
    expect(isValidReviewRequest(validReq)).toBe(true)
  })

  it("returns false when flashcard_id is empty", () => {
    expect(isValidReviewRequest({ ...validReq, flashcard_id: "" })).toBe(false)
  })

  it("returns false when grade is invalid (0)", () => {
    expect(isValidReviewRequest({ ...validReq, grade: 0 })).toBe(false)
  })

  it("returns false when grade is invalid (5)", () => {
    expect(isValidReviewRequest({ ...validReq, grade: 5 })).toBe(false)
  })

  it("returns false when duration_ms is negative", () => {
    expect(isValidReviewRequest({ ...validReq, duration_ms: -1 })).toBe(false)
  })

  it("returns true when duration_ms is zero", () => {
    expect(isValidReviewRequest({ ...validReq, duration_ms: 0 })).toBe(true)
  })
})
