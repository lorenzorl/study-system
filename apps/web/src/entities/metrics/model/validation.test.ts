import { describe, it, expect } from "vitest"
import { isValidStudyMetrics } from "./validation"
import type { StudyMetrics } from "./types"

// ─── StudyMetrics Validation ───────────────────────────────────────────────

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
