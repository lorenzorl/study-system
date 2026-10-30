import { describe, expect, it } from "vitest"
import type { DueCard } from "@/entities/review"
import { continueEntryFor } from "./continueEntry"

function card(id: string): DueCard {
  return {
    flashcard_id: id,
    question: `pregunta ${id}`,
    answer: `respuesta ${id}`,
    concept_title: "Filtro atencional",
    topic_name: "Atención",
    next_review: "2026-10-20T10:00:00Z",
  }
}

describe("continueEntryFor", () => {
  it("offers nothing when nothing is due", () => {
    expect(continueEntryFor([])).toBeNull()
  })

  it("offers one review entry whose total is the number of due cards", () => {
    const entry = continueEntryFor([card("f1"), card("f2"), card("f3")])

    expect(entry?.total).toBe(3)
    expect(entry?.done).toBe(0)
    expect(entry?.kindLabel).toBe("Repaso")
    expect(entry?.primaryActionLabel).toBe("Repasar")
  })

  it("keeps one due card as one, not as a list", () => {
    const entry = continueEntryFor([card("f1")])

    expect(entry?.total).toBe(1)
  })
})
