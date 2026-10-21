import { describe, expect, it } from "vitest"
import { deckRouteFor } from "./deckRoute"
import { resourceFixtures } from "./fixtures"

function fixture(id: string) {
  const resource = resourceFixtures.find((candidate) => candidate.id === id)
  if (!resource) throw new Error(`missing fixture: ${id}`)
  return resource
}

describe("deckRouteFor", () => {
  it("maps a deck resource to the presentation route of that deck", () => {
    expect(deckRouteFor(fixture("deck-atencion"))).toEqual({
      name: "presentation",
      params: { deckId: "deck-atencion" },
    })
  })

  it("refuses a resource that is not a deck", () => {
    expect(deckRouteFor(fixture("fc-atencion-selectiva"))).toBeNull()
    expect(deckRouteFor(fixture("exam-pensamiento"))).toBeNull()
  })
})
