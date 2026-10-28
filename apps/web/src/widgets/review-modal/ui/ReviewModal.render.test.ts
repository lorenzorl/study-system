import { describe, expect, it, vi } from "vitest"
import { createSSRApp, defineComponent, h } from "vue"
import { renderToString } from "vue/server-renderer"
import type { ReviewCard } from "../model/useCardSession"

/**
 * The dialog primitive is mocked at the module boundary, and only here.
 *
 * `DialogContent` reaches the document through reka-ui's `Teleport`, and that
 * teleport is mount-gated (`useMounted()`): a server render emits nothing but the
 * portal's placeholder, so without this mock the harness cannot see one element of
 * the panel it is supposed to pin. Mocking the boundary keeps the assertions on
 * this component's own markup — the grades group, the progress counter, the
 * completed/empty states — and leaves the primitive's own aria wiring to the
 * library. The primitive's real frame is covered in the browser harness.
 *
 * Pass-through stubs still mark their slot, so a component that went back to a
 * hand-rolled overlay would fail these tests instead of silently passing them.
 */
vi.mock("@/shared/ui/dialog", () => {
  const through = (tag: string, slot: string) =>
    defineComponent({
      // Props are irrelevant to the assertions; declaring none keeps the stubs
      // from leaking attributes into the markup under test.
      inheritAttrs: false,
      setup(_, { slots }) {
        return () => h(tag, { "data-slot": slot }, slots.default?.())
      },
    })

  return {
    Dialog: through("div", "dialog"),
    DialogContent: through("div", "dialog-content"),
    DialogTitle: through("h2", "dialog-title"),
  }
})

// Imported after the mock, so the component resolves the stubs.
const { default: ReviewModal } = await import("./ReviewModal.vue")

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

    // The grades live in the labelled group, so the test anchors on that group
    // instead of on the button class the migration replaces.
    const group = html.match(
      /<div[^>]*role="group"[^>]*aria-label="Calificar"[^>]*>([\s\S]*?)<\/div>/,
    )
    expect(group).not.toBeNull()

    const buttons = group?.[1].match(/<button[^>]*>/g) ?? []

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

  // What the migration changed, and all it changed: the surface is the primitive's.
  // The panel renders inside `DialogContent`, its heading inside `DialogTitle`, and
  // the component no longer draws a `role="dialog"` box of its own — that box was
  // what carried the manual Escape handler and the manual focus handling.
  it("renders its panel inside the dialog primitive, not in a dialog of its own", async () => {
    const html = await render(CARDS)

    expect(html).toContain('data-slot="dialog-content"')
    expect(html).toContain('data-slot="dialog-title"')
    expect(html).toContain("Repaso · Atención selectiva")
    expect(html).not.toMatch(/role="dialog"/)
    expect(html).not.toContain("aria-modal")
  })
})
