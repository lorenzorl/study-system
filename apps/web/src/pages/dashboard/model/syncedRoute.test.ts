import { describe, expect, it } from "vitest"
import { flashcardsRouteFor } from "./syncedRoute"

describe("flashcardsRouteFor", () => {
  it("maps a synced concept to the flashcards route of its concept", () => {
    expect(
      flashcardsRouteFor({ conceptId: "concept-agregados", topicId: "topic-ddd" }),
    ).toEqual({
      name: "flashcards",
      params: { topicId: "topic-ddd", conceptId: "concept-agregados" },
    })
  })

  it("refuses a concept whose topic the reload could not resolve", () => {
    expect(
      flashcardsRouteFor({ conceptId: "concept-agregados", topicId: null }),
    ).toBeNull()
  })
})
