import { describe, expect, it } from "vitest"
import type { Resource } from "@/entities/resource"
import {
  addedDaysAgo,
  materialStatus,
  resourceTypeLabel,
  toMaterialResources,
} from "./materials"

const NOW = new Date("2026-03-10T12:00:00.000Z")

function resource(overrides: Partial<Resource> = {}): Resource {
  return {
    id: "r-1",
    title: "Clean Architecture",
    type: "book",
    sourceUri: "obsidian://clean-arch.md",
    topicTitle: "DDD",
    createdAt: "2026-03-07T12:00:00.000Z",
    ...overrides,
  }
}

describe("addedDaysAgo", () => {
  it("counts whole days since the resource was created", () => {
    expect(addedDaysAgo("2026-03-07T12:00:00.000Z", NOW)).toBe(3)
  })

  it("counts a resource created earlier today as today", () => {
    expect(addedDaysAgo("2026-03-10T11:59:00.000Z", NOW)).toBe(0)
  })

  it("never reports a negative age for a timestamp in the future", () => {
    expect(addedDaysAgo("2026-03-11T12:00:00.000Z", NOW)).toBe(0)
  })

  it("returns null for a timestamp it cannot read", () => {
    expect(addedDaysAgo("no es una fecha", NOW)).toBeNull()
  })
})

describe("toMaterialResources", () => {
  it("projects a stored resource into a material card", () => {
    expect(toMaterialResources([resource()], NOW)).toEqual([
      {
        id: "r-1",
        kind: "material",
        title: "Clean Architecture",
        topicTitle: "DDD",
        conceptTitle: "",
        primaryActionLabel: "Abrir",
        resourceType: "book",
        sourceUri: "obsidian://clean-arch.md",
        addedDaysAgo: 3,
      },
    ])
  })

  it("keeps the order the server sent", () => {
    const mapped = toMaterialResources(
      [
        resource({ id: "r-newest", createdAt: "2026-03-09T12:00:00.000Z" }),
        resource({ id: "r-older", createdAt: "2026-03-01T12:00:00.000Z" }),
      ],
      NOW,
    )

    expect(mapped.map((item) => item.id)).toEqual(["r-newest", "r-older"])
  })

  it("maps an empty corpus to an empty list", () => {
    expect(toMaterialResources([], NOW)).toEqual([])
  })
})

describe("materialStatus", () => {
  it("labels the stored type and how long ago the resource was added", () => {
    const [card] = toMaterialResources([resource()], NOW)

    expect(materialStatus(card!)).toBe("Libro · agregado hace 3 días")
  })

  it("says today for a resource added within the day", () => {
    const [card] = toMaterialResources(
      [resource({ createdAt: "2026-03-10T09:00:00.000Z" })],
      NOW,
    )

    expect(materialStatus(card!)).toBe("Libro · agregado hoy")
  })

  it("uses the singular for a one-day-old resource", () => {
    const [card] = toMaterialResources(
      [resource({ createdAt: "2026-03-09T11:00:00.000Z" })],
      NOW,
    )

    expect(materialStatus(card!)).toBe("Libro · agregado hace 1 día")
  })

  it("drops the age when the timestamp is unreadable", () => {
    const [card] = toMaterialResources(
      [resource({ createdAt: "no es una fecha" })],
      NOW,
    )

    expect(materialStatus(card!)).toBe("Libro")
  })
})

describe("resourceTypeLabel", () => {
  it("labels the four stored types", () => {
    expect(resourceTypeLabel("book")).toBe("Libro")
    expect(resourceTypeLabel("note")).toBe("Nota")
    expect(resourceTypeLabel("article")).toBe("Artículo")
    expect(resourceTypeLabel("video")).toBe("Video")
  })
})
