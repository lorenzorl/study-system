import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import ReviewModal from "./ReviewModal.vue"
import type { ReviewCard } from "../model/useCardSession"

// The repo's vitest environment is `node`: no jsdom, no @vue/test-utils. Rendering
// through `vue/server-renderer` is the cheapest way to pin the states the keyboard
// contract depends on, without adding a dependency. Interaction (focus, key
// handling) is still verified by hand in the browser harness.

const CARDS: ReviewCard[] = [
  { id: "c1", question: "¿Qué es el filtro atencional?", answer: "Una selección." },
  { id: "c2", question: "¿Qué es la atención dividida?", answer: "Dos tareas." },
]

function render(cards: ReviewCard[], initialIndex?: number): Promise<string> {
  return renderToString(
    createSSRApp(ReviewModal, { cards, title: "Atención selectiva", initialIndex }),
  )
}

describe("ReviewModal render", () => {
  it("disables all four grades before the answer is revealed", async () => {
    const html = await render(CARDS)

    const buttons = html.match(/<button[^>]*review-modal__grade[^>]*>/g) ?? []

    expect(buttons).toHaveLength(4)
    for (const button of buttons) expect(button).toContain("disabled")
    expect(html).toContain("1 / 2")
  })

  it("resumes at the given index", async () => {
    const html = await render(CARDS, 1)

    expect(html).toContain("2 / 2")
  })

  it("never shows a completed session for a deck with no cards", async () => {
    const html = await render([])

    expect(html).not.toContain("Repaso completado")
    expect(html).not.toMatch(/>\s*1 \/ 0\s*</)
  })
})
