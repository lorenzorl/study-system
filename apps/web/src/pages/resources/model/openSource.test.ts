import { describe, expect, it } from "vitest"
import { sourceTargetOf } from "./openSource"

describe("sourceTargetOf", () => {
  it("treats an http or https URI as external", () => {
    expect(sourceTargetOf("https://example.com/paper.pdf")).toEqual({
      kind: "external",
      uri: "https://example.com/paper.pdf",
    })
    expect(sourceTargetOf("HTTP://example.com")).toEqual({
      kind: "external",
      uri: "HTTP://example.com",
    })
  })

  it("treats a corpus document or a legacy vault path as a corpus source", () => {
    expect(sourceTargetOf("rag://documents/doc-1")).toEqual({
      kind: "corpus",
      uri: "rag://documents/doc-1",
    })
    expect(sourceTargetOf("obsidian://ddd.md")).toEqual({
      kind: "corpus",
      uri: "obsidian://ddd.md",
    })
    expect(sourceTargetOf("DDD/Clean Architecture.md")).toEqual({
      kind: "corpus",
      uri: "DDD/Clean Architecture.md",
    })
  })

  it("returns null for an empty source, so nothing is opened", () => {
    expect(sourceTargetOf("")).toBeNull()
    expect(sourceTargetOf("   ")).toBeNull()
  })

  it("trims the source it opens", () => {
    expect(sourceTargetOf("  DDD/A.md  ")).toEqual({
      kind: "corpus",
      uri: "DDD/A.md",
    })
  })
})
