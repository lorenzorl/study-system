import { describe, expect, it } from "vitest"
import { filterResources } from "./filter"
import type { Resource } from "./types"

// The launcher's list comes from `GET /api/resources`, so these are test-local
// cards, not shipped fixtures. One entry per kind keeps every filter rule covered.
const LAUNCHER_RESOURCES: Resource[] = [
  {
    id: "material-clean-arch",
    kind: "material",
    title: "Clean Architecture",
    topicTitle: "DDD",
    conceptTitle: "",
    primaryActionLabel: "Abrir",
    resourceType: "book",
    sourceUri: "obsidian://clean-arch.md",
    addedDaysAgo: 3,
  },
  {
    id: "fc-atencion-selectiva",
    kind: "flashcards",
    title: "Atención selectiva",
    topicTitle: "Atención",
    conceptTitle: "Atención selectiva",
    primaryActionLabel: "Repasar",
    due: 5,
    total: 8,
  },
  {
    id: "fc-sesgos-cognitivos",
    kind: "flashcards",
    title: "Sesgos cognitivos",
    topicTitle: "Pensamiento",
    conceptTitle: "Sesgo de confirmación",
    primaryActionLabel: "Repasar",
    due: 12,
    total: 30,
  },
  {
    id: "deck-atencion",
    kind: "deck",
    title: "Atención",
    topicTitle: "Atención",
    conceptTitle: "Atención selectiva",
    primaryActionLabel: "Presentar",
    slides: 12,
    updatedDaysAgo: 2,
  },
  {
    id: "exam-pensamiento",
    kind: "exam",
    title: "Pensamiento",
    topicTitle: "Pensamiento",
    conceptTitle: "Sesgo de confirmación",
    primaryActionLabel: "Empezar",
    questions: 10,
    lastScore: null,
    of: null,
  },
]

// ─── Launcher filter ───────────────────────────────────────────────────────

function ids(resources: { id: string }[]): string[] {
  return resources.map((resource) => resource.id)
}

describe("filterResources", () => {
  it("keeps only the resources of the selected kind", () => {
    expect(ids(filterResources(LAUNCHER_RESOURCES, "all", ""))).toEqual(
      LAUNCHER_RESOURCES.map((resource) => resource.id),
    )
    expect(ids(filterResources(LAUNCHER_RESOURCES, "material", ""))).toEqual([
      "material-clean-arch",
    ])
    expect(ids(filterResources(LAUNCHER_RESOURCES, "deck", ""))).toEqual([
      "deck-atencion",
    ])
    expect(ids(filterResources(LAUNCHER_RESOURCES, "exam", ""))).toEqual([
      "exam-pensamiento",
    ])
  })

  it("matches the query against the resource title", () => {
    const result = filterResources(LAUNCHER_RESOURCES, "all", "cognitivos")
    expect(result.map((resource) => resource.title)).toEqual([
      "Sesgos cognitivos",
    ])
  })

  it("matches the query against the topic title", () => {
    const result = filterResources(LAUNCHER_RESOURCES, "all", "pensamiento")
    expect(result.map((resource) => resource.title)).toEqual([
      "Sesgos cognitivos",
      "Pensamiento",
    ])
  })

  it("matches regardless of accent and case", () => {
    const accented = filterResources(LAUNCHER_RESOURCES, "all", "atención")
    const plain = filterResources(LAUNCHER_RESOURCES, "all", "ATENCION")

    expect(ids(plain)).toEqual(ids(accented))
    expect(ids(plain)).toEqual(["fc-atencion-selectiva", "deck-atencion"])
  })
})
