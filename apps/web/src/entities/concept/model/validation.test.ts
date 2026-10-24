import { describe, it, expect } from "vitest"
import { isValidConceptSummary } from "./validation"
import type { ConceptSummary } from "./types"

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
