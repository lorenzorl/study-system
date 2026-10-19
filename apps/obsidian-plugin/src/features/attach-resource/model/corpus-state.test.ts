import { describe, expect, it } from "vitest"
import type { DocumentSummary } from "@/entities/document"
import { corpusStateLabel } from "./corpus-state"

function document(overrides: Partial<DocumentSummary>): DocumentSummary {
  return {
    id: "d1",
    title: "DDD",
    createdAt: "2026-10-18T10:00:00Z",
    pages: 0,
    chunks: 0,
    ...overrides,
  }
}

describe("corpusStateLabel", () => {
  it("says it has no server data when the listing did not bring the document", () => {
    // The chat can hold an attachment whose document is not in the listing: the
    // listing is fetched when the pickers open, and the server may be down.
    expect(corpusStateLabel(undefined)).toBe("sin datos del servidor")
  })

  it("reports a document the server has recorded nothing for", () => {
    expect(corpusStateLabel(document({ pages: 0, chunks: 0 }))).toBe(
      "sin procesar",
    )
  })

  it("reports pages without searchable chunks as still being ingested", () => {
    // The consequence a reader needs: with no chunks, a search cannot match
    // inside this document, so attaching it does not yet change the answer.
    const label = corpusStateLabel(document({ pages: 12, chunks: 0 }))

    expect(label).toBe("12 páginas, todavía sin fragmentos buscables")
    expect(label).toContain("sin fragmentos")
  })

  it("reports a fully ingested document by both of its counts", () => {
    expect(corpusStateLabel(document({ pages: 359, chunks: 1200 }))).toBe(
      "359 páginas, 1200 fragmentos",
    )
  })

  it("writes both counts in the singular when the count is one", () => {
    expect(corpusStateLabel(document({ pages: 1, chunks: 1 }))).toBe(
      "1 página, 1 fragmento",
    )
  })

  it("keeps the singular page when only the page count is one", () => {
    expect(corpusStateLabel(document({ pages: 1, chunks: 0 }))).toBe(
      "1 página, todavía sin fragmentos buscables",
    )
  })
})
