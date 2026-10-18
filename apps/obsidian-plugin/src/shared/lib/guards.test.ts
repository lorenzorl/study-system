import { describe, it, expect } from "vitest"
import { nextIndex, prevIndex } from "./guards"

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
