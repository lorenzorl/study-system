import { describe, it, expect } from "vitest"
import { isValidTopic } from "./validation"
import type { Topic } from "./types"

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
