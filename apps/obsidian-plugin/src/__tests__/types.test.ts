import { describe, it, expect } from "vitest";
import { SAMPLE_FLASHCARDS } from "../data";
import { MOCK_DOMAINS, MOCK_METRICS } from "../mock-data";
import {
  isValidFlashcard,
  isValidDomain,
  isValidConcept,
  isValidStudyMetrics,
  nextIndex,
  prevIndex,
} from "../validation";
import type { Flashcard, Domain, Concept, StudyMetrics } from "../types";

// ─── Existing Flashcard Tests ───────────────────────────────────────────────

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

// ─── New Domain / Concept / StudyMetrics Tests ──────────────────────────────

describe("Mock data integrity", () => {
  it("has at least one domain", () => {
    expect(MOCK_DOMAINS.length).toBeGreaterThan(0);
  });

  it("every mock domain passes isValidDomain", () => {
    expect(MOCK_DOMAINS.every((d) => isValidDomain(d))).toBe(true);
  });

  it("every domain id is unique", () => {
    const ids = MOCK_DOMAINS.map((d) => d.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("every domain has at least one concept", () => {
    expect(MOCK_DOMAINS.every((d) => d.concepts.length > 0)).toBe(true);
  });

  it("every concept within domains passes isValidConcept", () => {
    const allConcepts = MOCK_DOMAINS.flatMap((d) => d.concepts);
    expect(allConcepts.length).toBeGreaterThan(0);
    expect(allConcepts.every((c) => isValidConcept(c))).toBe(true);
  });

  it("every concept within a domain has a unique id", () => {
    for (const domain of MOCK_DOMAINS) {
      const ids = domain.concepts.map((c) => c.id);
      expect(new Set(ids).size).toBe(ids.length);
    }
  });

  it("every concept has at least one flashcard", () => {
    const allConcepts = MOCK_DOMAINS.flatMap((d) => d.concepts);
    expect(allConcepts.every((c) => c.flashcards.length > 0)).toBe(true);
  });

  it("mock metrics pass isValidStudyMetrics", () => {
    expect(isValidStudyMetrics(MOCK_METRICS)).toBe(true);
  });
});

describe("isValidDomain", () => {
  const validDomain: Domain = {
    id: "test-domain",
    name: "Test Domain",
    description: "A test domain for validation.",
    concepts: [
      {
        id: "test-concept",
        name: "Test Concept",
        summary: "A test concept.",
        flashcards: [
          { id: "f1", question: "Q1?", answer: "A1" },
        ],
      },
    ],
  };

  it("returns true for a valid domain", () => {
    expect(isValidDomain(validDomain)).toBe(true);
  });

  it("returns false when id is empty", () => {
    expect(isValidDomain({ ...validDomain, id: "" })).toBe(false);
  });

  it("returns false when name is empty", () => {
    expect(isValidDomain({ ...validDomain, name: "" })).toBe(false);
  });

  it("returns false when description is empty", () => {
    expect(isValidDomain({ ...validDomain, description: "" })).toBe(false);
  });

  it("returns false when concepts array is empty", () => {
    expect(isValidDomain({ ...validDomain, concepts: [] })).toBe(false);
  });
});

describe("isValidConcept", () => {
  const validConcept: Concept = {
    id: "test-concept",
    name: "Test Concept",
    summary: "A summary of the test concept.",
    flashcards: [{ id: "f1", question: "Q?", answer: "A" }],
  };

  it("returns true for a valid concept", () => {
    expect(isValidConcept(validConcept)).toBe(true);
  });

  it("returns false when id is empty", () => {
    expect(isValidConcept({ ...validConcept, id: "" })).toBe(false);
  });

  it("returns false when name is empty", () => {
    expect(isValidConcept({ ...validConcept, name: "" })).toBe(false);
  });

  it("returns false when summary is empty", () => {
    expect(isValidConcept({ ...validConcept, summary: "" })).toBe(false);
  });

  it("returns false when flashcards array is empty", () => {
    expect(isValidConcept({ ...validConcept, flashcards: [] })).toBe(false);
  });
});

describe("isValidStudyMetrics", () => {
  const validMetrics: StudyMetrics = {
    dailyCardCount: 45,
    retentionRate: 0.78,
    currentStreak: 12,
    totalReviewed: 280,
  };

  it("returns true for valid metrics", () => {
    expect(isValidStudyMetrics(validMetrics)).toBe(true);
  });

  it("returns true for zero values", () => {
    expect(
      isValidStudyMetrics({
        dailyCardCount: 0,
        retentionRate: 0,
        currentStreak: 0,
        totalReviewed: 0,
      }),
    ).toBe(true);
  });

  it("returns true for max retention (1.0)", () => {
    expect(
      isValidStudyMetrics({ ...validMetrics, retentionRate: 1.0 }),
    ).toBe(true);
  });

  it("returns false when retentionRate > 1.0", () => {
    expect(
      isValidStudyMetrics({ ...validMetrics, retentionRate: 1.1 }),
    ).toBe(false);
  });

  it("returns false when retentionRate < 0", () => {
    expect(
      isValidStudyMetrics({ ...validMetrics, retentionRate: -0.1 }),
    ).toBe(false);
  });

  it("returns false when dailyCardCount is negative", () => {
    expect(
      isValidStudyMetrics({ ...validMetrics, dailyCardCount: -1 }),
    ).toBe(false);
  });

  it("returns false when dailyCardCount is not an integer", () => {
    expect(
      isValidStudyMetrics({ ...validMetrics, dailyCardCount: 1.5 }),
    ).toBe(false);
  });
});

// ─── Navigation Helpers (unchanged logic, kept for completeness) ────────────

describe("nextIndex", () => {
  const total = 5;

  it("moves forward from first card", () => {
    expect(nextIndex(0, total)).toBe(1);
  });

  it("moves forward from middle", () => {
    expect(nextIndex(2, total)).toBe(3);
  });

  it("wraps from last card to first (circular)", () => {
    expect(nextIndex(4, total)).toBe(0);
  });

  it("moves forward with single card (wraps to same)", () => {
    expect(nextIndex(0, 1)).toBe(0);
  });
});

describe("prevIndex", () => {
  const total = 5;

  it("moves backward from second card", () => {
    expect(prevIndex(1, total)).toBe(0);
  });

  it("moves backward from middle", () => {
    expect(prevIndex(3, total)).toBe(2);
  });

  it("wraps from first card to last (circular)", () => {
    expect(prevIndex(0, total)).toBe(4);
  });

  it("moves backward with single card (wraps to same)", () => {
    expect(prevIndex(0, 1)).toBe(0);
  });
});
