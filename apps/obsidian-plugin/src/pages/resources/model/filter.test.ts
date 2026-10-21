import { describe, expect, it } from "vitest"
import { filterResources } from "./filter"
import { resourceFixtures } from "./fixtures"

// ─── Launcher filter ───────────────────────────────────────────────────────

function ids(resources: { id: string }[]): string[] {
  return resources.map((resource) => resource.id)
}

describe("filterResources", () => {
  it("keeps only the resources of the selected kind", () => {
    expect(ids(filterResources(resourceFixtures, "all", ""))).toEqual(
      resourceFixtures.map((resource) => resource.id),
    )
    expect(ids(filterResources(resourceFixtures, "deck", ""))).toEqual([
      "deck-atencion",
      "deck-memoria",
    ])
    expect(ids(filterResources(resourceFixtures, "exam", ""))).toEqual([
      "exam-pensamiento",
      "exam-memoria",
    ])
  })

  it("matches the query against the resource title", () => {
    const result = filterResources(resourceFixtures, "all", "cognitivos")
    expect(result.map((resource) => resource.title)).toEqual([
      "Sesgos cognitivos",
    ])
  })

  it("matches the query against the topic title", () => {
    const result = filterResources(resourceFixtures, "all", "pensamiento")
    expect(result.map((resource) => resource.title)).toEqual([
      "Sesgos cognitivos",
      "Pensamiento",
    ])
  })

  it("matches regardless of accent and case", () => {
    const accented = filterResources(resourceFixtures, "all", "atención")
    const plain = filterResources(resourceFixtures, "all", "ATENCION")

    expect(ids(plain)).toEqual(ids(accented))
    expect(ids(plain)).toEqual([
      "fc-atencion-selectiva",
      "fc-atencion-dividida",
      "deck-atencion",
    ])
  })
})
