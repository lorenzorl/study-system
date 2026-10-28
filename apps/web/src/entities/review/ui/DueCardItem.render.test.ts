import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import type { DueCard } from "../model/types"
import DueCardItem from "./DueCardItem.vue"

/**
 * A characterization test for the item the migration touched. The visual net owns
 * how it looks (`due-cards.png`), so this pins what a screenshot cannot see: the
 * whole row is one button, and the question, its concept and its topic all survive
 * the move onto `shared/ui`.
 *
 * SSR wraps interpolated text in fragment markers, so the assertions strip
 * comments first (`shadcn-migration.md` L16).
 */

const CARD: DueCard = {
  flashcard_id: "f1",
  question: "¿Qué es el filtro atencional?",
  answer: "El mecanismo que selecciona qué entra en la conciencia.",
  concept_title: "Filtro atencional",
  topic_name: "Atención",
  next_review: "2026-10-20T10:00:00Z",
}

async function render(): Promise<string> {
  const html = await renderToString(createSSRApp(DueCardItem, { card: CARD }))

  return html.replace(/<!--[\s\S]*?-->/g, "").replace(/\s+/g, " ")
}

describe("DueCardItem render", () => {
  it("renders the question with the concept and topic it belongs to", async () => {
    const html = await render()

    expect(html).toContain("¿Qué es el filtro atencional?")
    expect(html).toContain("Filtro atencional")
    expect(html).toContain("Atención")
  })

  it("is a single button, rendered by the shared/ui primitive", async () => {
    const html = await render()

    expect(html.match(/<button/g)).toHaveLength(1)
    expect(html).toContain('data-slot="button"')
  })
})
