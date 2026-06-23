import { describe, it, expect } from "vitest"
import {
  isValidFlashcard,
  isValidTopic,
  isValidConceptSummary,
  isValidStudyMetrics,
  nextIndex,
  prevIndex,
  isValidDueCard,
  isValidGrade,
  validateGrade,
  isValidReviewRequest,
  isValidResourceType,
  isValidSyncResourceRequest,
} from "../validation"
import type {
  Flashcard,
  Topic,
  ConceptSummary,
  StudyMetrics,
  DueCard,
  ReviewRequest,
  SyncResourceRequest,
} from "../types"
import { parseFlashcards } from "../vue-app/services/markdown-parser"

// ─── Parser Tests ──────────────────────────────────────────────────────────

describe("parseFlashcards", () => {
  it("extracts a basic :: delimited flashcard", () => {
    const md = "¿Qué es DDD?::Domain-Driven Design"
    const result = parseFlashcards(md)
    expect(result).toHaveLength(1)
    expect(result[0]).toMatchObject({
      obsidian_id: "card-1",
      question: "¿Qué es DDD?",
      answer: "Domain-Driven Design",
    })
  })

  it("extracts obsidian_id from <!-- id: --> comment near Q&A line", () => {
    const md =
      "<!-- id: abc123 -->\n¿Qué es DDD?::Domain-Driven Design"
    const result = parseFlashcards(md)
    expect(result).toHaveLength(1)
    expect(result[0]).toMatchObject({
      obsidian_id: "abc123",
      question: "¿Qué es DDD?",
      answer: "Domain-Driven Design",
    })
  })

  it("extracts multiple flashcards from markdown", () => {
    const md = [
      "¿Qué es DDD?::Domain-Driven Design",
      "",
      "¿Qué es un Agregado?::Un cluster de objetos",
    ].join("\n")
    const result = parseFlashcards(md)
    expect(result).toHaveLength(2)
  })

  it("skips lines without :: delimiter", () => {
    const md = "This is a regular paragraph.\nAnother line without delimiter.\n¿Pregunta?::Respuesta"
    const result = parseFlashcards(md)
    expect(result).toHaveLength(1)
  })

  it("returns empty array for empty input", () => {
    expect(parseFlashcards("")).toEqual([])
  })

  it("finds id comment within 5-line lookback window", () => {
    const md = [
      "<!-- id: far-id -->",
      "",
      "",
      "",
      "¿Pregunta lejana?::Respuesta lejana",
    ].join("\n")
    const result = parseFlashcards(md)
    expect(result).toHaveLength(1)
    expect(result[0].obsidian_id).toBe("far-id")
  })

  it("generates line-number id when no id comment present", () => {
    const md = "Q1::A1\nQ2::A2"
    const result = parseFlashcards(md)
    expect(result).toHaveLength(2)
    expect(result[0].obsidian_id).toBe("card-1")
    expect(result[1].obsidian_id).toBe("card-2")
  })

  it("skips blank :: lines where question or answer is empty after trim", () => {
    const md = " :: \nQ1::A1"
    const result = parseFlashcards(md)
    expect(result).toHaveLength(1)
    expect(result[0].question).toBe("Q1")
  })

  it("trims whitespace from question and answer", () => {
    const md = "  ¿Pregunta?  ::  Respuesta  "
    const result = parseFlashcards(md)
    expect(result).toHaveLength(1)
    expect(result[0].question).toBe("¿Pregunta?")
    expect(result[0].answer).toBe("Respuesta")
  })
})

// ─── Flashcard Validation (new types) ──────────────────────────────────────

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

// ─── Topic Validation ─────────────────────────────────────────────────────

describe("isValidTopic", () => {
  const validTopic: Topic = {
    id: "test-topic",
    name: "Test Topic",
    concepts: [
      {
        id: "test-concept",
        title: "Test Concept",
        file_path: "test/concept.md",
      },
    ],
  }

  it("returns true for a valid topic", () => {
    expect(isValidTopic(validTopic)).toBe(true)
  })

  it("returns false when id is empty", () => {
    expect(isValidTopic({ ...validTopic, id: "" })).toBe(false)
  })

  it("returns false when name is empty", () => {
    expect(isValidTopic({ ...validTopic, name: "" })).toBe(false)
  })

  it("returns true when concepts array is empty", () => {
    expect(isValidTopic({ ...validTopic, concepts: [] })).toBe(true)
  })
})

// ─── ConceptSummary Validation ─────────────────────────────────────────────

describe("isValidConceptSummary", () => {
  const validConcept: ConceptSummary = {
    id: "test-concept",
    title: "Test Concept",
    file_path: "test/concept.md",
  }

  it("returns true for a valid concept", () => {
    expect(isValidConceptSummary(validConcept)).toBe(true)
  })

  it("returns false when id is empty", () => {
    expect(isValidConceptSummary({ ...validConcept, id: "" })).toBe(false)
  })

  it("returns false when title is empty", () => {
    expect(isValidConceptSummary({ ...validConcept, title: "" })).toBe(false)
  })

  it("returns false when file_path is empty", () => {
    expect(isValidConceptSummary({ ...validConcept, file_path: "" })).toBe(false)
  })
})

// ─── StudyMetrics Validation (unchanged logic) ─────────────────────────────

describe("isValidStudyMetrics", () => {
  const validMetrics: StudyMetrics = {
    dailyCardCount: 45,
    retentionRate: 0.78,
    currentStreak: 12,
    totalReviewed: 280,
  }

  it("returns true for valid metrics", () => {
    expect(isValidStudyMetrics(validMetrics)).toBe(true)
  })

  it("returns true for zero values", () => {
    expect(
      isValidStudyMetrics({
        dailyCardCount: 0,
        retentionRate: 0,
        currentStreak: 0,
        totalReviewed: 0,
      }),
    ).toBe(true)
  })

  it("returns true for max retention (1.0)", () => {
    expect(
      isValidStudyMetrics({ ...validMetrics, retentionRate: 1.0 }),
    ).toBe(true)
  })

  it("returns false when retentionRate > 1.0", () => {
    expect(
      isValidStudyMetrics({ ...validMetrics, retentionRate: 1.1 }),
    ).toBe(false)
  })

  it("returns false when dailyCardCount is negative", () => {
    expect(
      isValidStudyMetrics({ ...validMetrics, dailyCardCount: -1 }),
    ).toBe(false)
  })
})

// ─── Navigation Helpers ────────────────────────────────────────────────────

describe("nextIndex", () => {
  const total = 5

  it("moves forward from first card", () => {
    expect(nextIndex(0, total)).toBe(1)
  })

  it("wraps from last card to first (circular)", () => {
    expect(nextIndex(4, total)).toBe(0)
  })

  it("moves forward with single card (wraps to same)", () => {
    expect(nextIndex(0, 1)).toBe(0)
  })
})

describe("prevIndex", () => {
  const total = 5

  it("moves backward from second card", () => {
    expect(prevIndex(1, total)).toBe(0)
  })

  it("wraps from first card to last (circular)", () => {
    expect(prevIndex(0, total)).toBe(4)
  })

  it("moves backward with single card (wraps to same)", () => {
    expect(prevIndex(0, 1)).toBe(0)
  })
})

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

// ─── ResourceType Validation ─────────────────────────────────────────────────

describe("isValidResourceType", () => {
  it("returns true for valid types", () => {
    expect(isValidResourceType("book")).toBe(true)
    expect(isValidResourceType("note")).toBe(true)
    expect(isValidResourceType("article")).toBe(true)
    expect(isValidResourceType("video")).toBe(true)
  })

  it("returns false for invalid type", () => {
    expect(isValidResourceType("podcast")).toBe(false)
  })

  it("returns false for empty string", () => {
    expect(isValidResourceType("")).toBe(false)
  })
})

// ─── SyncResourceRequest Validation ──────────────────────────────────────────

describe("isValidSyncResourceRequest", () => {
  const validReq: SyncResourceRequest = {
    topic_name: "DDD",
    resource_title: "Blue Book",
    type: "book",
    source_uri: "https://example.com/book",
    dify_document_id: "dify-123",
  }

  it("returns true for a valid request", () => {
    expect(isValidSyncResourceRequest(validReq)).toBe(true)
  })

  it("returns true when optional dify_document_id is missing", () => {
    const { dify_document_id: _, ...rest } = validReq
    expect(isValidSyncResourceRequest(rest)).toBe(true)
  })

  it("returns false when topic_name is empty", () => {
    expect(isValidSyncResourceRequest({ ...validReq, topic_name: "" })).toBe(false)
  })

  it("returns false when resource_title is empty", () => {
    expect(isValidSyncResourceRequest({ ...validReq, resource_title: "" })).toBe(false)
  })

  it("returns false when type is invalid", () => {
    expect(
      isValidSyncResourceRequest({ ...validReq, type: "podcast" as any }),
    ).toBe(false)
  })

  it("returns false when source_uri is empty", () => {
    expect(isValidSyncResourceRequest({ ...validReq, source_uri: "" })).toBe(false)
  })
})
