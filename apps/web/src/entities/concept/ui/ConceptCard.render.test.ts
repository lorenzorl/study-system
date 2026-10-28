import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import type { ConceptSummary } from "../model/types"
import ConceptCard from "./ConceptCard.vue"

/**
 * A characterization test for the card the migration touched, not a spec of how it
 * looks: the visual net owns the appearance, and it already sees this card inside
 * `topic-new-concept-form.png`. What nothing else pins is the shape the card is
 * allowed to change into — the whole card is one button, so moving it onto
 * `shared/ui` must not turn that button into a div or split the click target.
 *
 * SSR text arrives wrapped in fragment markers (`<!--[-->…<!--]-->`), so the
 * assertions strip comments and collapse whitespace first (`shadcn-migration.md`
 * L16).
 */

const CONCEPT: ConceptSummary = {
  id: "c1",
  title: "Filtro atencional",
  file_path: "atencion/filtro.md",
}

async function render(flashcardCount: number): Promise<string> {
  const html = await renderToString(
    createSSRApp(ConceptCard, { concept: CONCEPT, flashcardCount }),
  )

  return html.replace(/<!--[\s\S]*?-->/g, "").replace(/\s+/g, " ")
}

describe("ConceptCard render", () => {
  it("renders the title, the path and the card count as a badge", async () => {
    const html = await render(3)

    expect(html).toContain("Filtro atencional")
    expect(html).toContain("atencion/filtro.md")

    // The count is a chip, not body text: it renders through the badge primitive,
    // the same one `TopicCard` uses, so the two cards present it the same way.
    const badge = html.match(/<div data-slot="badge"[^>]*>([\s\S]*?)<\/div>/)

    expect(badge?.[1]).toContain("3 tarjetas")
  })

  it("keeps the count singular for a single card", async () => {
    const html = await render(1)

    expect(html).toContain("1 tarjeta")
    expect(html).not.toContain("tarjetas")
  })

  it("is a single button, rendered by the shared/ui primitive", async () => {
    const html = await render(0)

    expect(html.match(/<button/g)).toHaveLength(1)
    expect(html).toContain('data-slot="button"')
  })
})
