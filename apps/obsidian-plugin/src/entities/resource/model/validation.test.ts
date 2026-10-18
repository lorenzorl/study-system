import { describe, it, expect } from "vitest"
import { isValidResourceType, isValidSyncResourceRequest } from "./validation"
import type { SyncResourceRequest } from "./types"

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
  }

  it("returns true for a valid request", () => {
    expect(isValidSyncResourceRequest(validReq)).toBe(true)
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
