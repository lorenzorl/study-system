import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import type { Topic } from "../model/types"
import TopicCard from "./TopicCard.vue"

/**
 * A characterization test for the card the migration touched. The visual net owns
 * how it looks (`topics.png`), so this pins what the pixel comparison cannot see:
 * the whole card is one button, and the count chip is still the card's own text.
 *
 * SSR wraps interpolated text in fragment markers and keeps the template's
 * newlines, so the assertions strip comments and collapse whitespace
 * (`shadcn-migration.md` L16).
 */

const CONCEPT: Topic["concepts"][number] = {
  id: "c1",
  title: "Filtro atencional",
  file_path: "atencion/filtro.md",
}

async function render(concepts: Topic["concepts"]): Promise<string> {
  const topic: Topic = { id: "t1", name: "Atención", concepts }
  const html = await renderToString(createSSRApp(TopicCard, { topic }))

  return html.replace(/<!--[\s\S]*?-->/g, "").replace(/\s+/g, " ")
}

describe("TopicCard render", () => {
  it("renders the topic's name and how many concepts it holds", async () => {
    const html = await render([CONCEPT])

    expect(html).toContain("Atención")
    expect(html).toContain("1 concepto")
    expect(html).not.toContain("conceptos")
  })

  it("pluralises the count", async () => {
    const html = await render([CONCEPT, { ...CONCEPT, id: "c2" }])

    expect(html).toContain("2 conceptos")
  })

  it("is a single button, rendered by the shared/ui primitive", async () => {
    const html = await render([CONCEPT])

    expect(html.match(/<button/g)).toHaveLength(1)
    expect(html).toContain('data-slot="button"')
  })
})
