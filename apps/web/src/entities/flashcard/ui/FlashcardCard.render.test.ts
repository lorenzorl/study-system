import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import FlashcardCard from "./FlashcardCard.vue"

/**
 * The only entity card the visual net cannot see.
 *
 * `resources.png` is blind to the review modal, `/study/review` never reaches a
 * populated session and `/study/due` renders the list item, not this card, so a
 * regression in the card's frame would fail no gate at all
 * (`shadcn-migration.md` L17/L18). That is why this file exists when the other
 * three cards only got the primitive invariant.
 *
 * SSR wraps interpolated text in fragment markers, so the assertions strip
 * comments first (`shadcn-migration.md` L16). The face that is showing is decided
 * by CSS transforms, so both faces are always in the markup; what the markup pins
 * is that they are there and that the label announces the next action.
 */

const CARD = {
  id: "c1",
  question: "¿Qué es el filtro atencional?",
  answer: "El mecanismo que selecciona qué entra en la conciencia.",
}

async function render(): Promise<string> {
  const html = await renderToString(createSSRApp(FlashcardCard, { card: CARD }))

  return html.replace(/<!--[\s\S]*?-->/g, "").replace(/\s+/g, " ")
}

describe("FlashcardCard render", () => {
  it("renders both faces, question first, with the hint for each", async () => {
    const html = await render()

    expect(html).toContain("¿Qué es el filtro atencional?")
    expect(html).toContain("El mecanismo que selecciona qué entra en la conciencia.")
    expect(html).toContain("Tocá para revelar")
    expect(html).toContain("Tocá para ocultar")
  })

  it("is a single button that announces the next action, rendered by the primitive", async () => {
    const html = await render()

    expect(html.match(/<button/g)).toHaveLength(1)
    expect(html).toContain('data-slot="button"')
    // Unflipped: the card offers the answer, which is what a screen reader reads.
    expect(html).toContain('aria-label="Mostrar respuesta"')
  })
})
