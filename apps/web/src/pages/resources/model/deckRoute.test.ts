import { describe, expect, it } from "vitest"
import { deckRouteFor } from "./deckRoute"
import type { Resource } from "./types"

function deck(id: string): Resource {
  return {
    id,
    kind: "deck",
    title: "Atención",
    topicTitle: "Atención",
    conceptTitle: "Atención selectiva",
    primaryActionLabel: "Presentar",
    slides: 12,
    updatedDaysAgo: 2,
  }
}

describe("deckRouteFor", () => {
  it("maps a deck resource to the presentation route of that deck", () => {
    expect(deckRouteFor(deck("deck-atencion"))).toEqual({
      name: "presentation",
      params: { deckId: "deck-atencion" },
    })
  })

  it("refuses a resource that is not a deck", () => {
    expect(
      deckRouteFor({
        id: "fc-atencion-selectiva",
        kind: "flashcards",
        title: "Atención selectiva",
        topicTitle: "Atención",
        conceptTitle: "Atención selectiva",
        primaryActionLabel: "Repasar",
        due: 5,
        total: 8,
      }),
    ).toBeNull()

    expect(
      deckRouteFor({
        id: "exam-pensamiento",
        kind: "exam",
        title: "Pensamiento",
        topicTitle: "Pensamiento",
        conceptTitle: "Sesgo de confirmación",
        primaryActionLabel: "Empezar",
        questions: 10,
        lastScore: null,
        of: null,
      }),
    ).toBeNull()
  })

  it("never routes a material to the deck tab", () => {
    expect(
      deckRouteFor({
        id: "material-clean-arch",
        kind: "material",
        title: "Clean Architecture",
        topicTitle: "DDD",
        conceptTitle: "",
        primaryActionLabel: "Abrir",
        resourceType: "book",
        sourceUri: "obsidian://clean-arch.md",
        addedDaysAgo: 3,
      }),
    ).toBeNull()
  })
})
