import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { useSyncNote } from "../model/useSyncNote"
import SyncNoteForm from "./SyncNoteForm.vue"

// The repo's vitest environment is `node`: no jsdom, no @vue/test-utils. Rendering
// through `vue/server-renderer` pins which fields the panel shows and how a
// refusal looks, without a dependency. Interaction (typing, Escape, focus) is
// verified by hand in the browser harness.

type Store = ReturnType<typeof useSyncNote>

function render(seed: (store: Store) => void = () => {}): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)
  seed(useSyncNote())

  const app = createSSRApp(SyncNoteForm)
  app.use(pinia)

  return renderToString(app)
}

function buttonMatching(html: string, pattern: RegExp): string {
  const match = html.match(pattern)
  expect(match).not.toBeNull()

  return match?.[0] ?? ""
}

/**
 * Whether a rendered tag carries the `disabled` **attribute**.
 *
 * A substring check on "disabled" stopped working the moment the buttons became
 * `shared/ui` primitives: their utility classes are full of `disabled:`
 * variants, so `toContain("disabled")` matched a class and would report an
 * enabled button as disabled.
 */
function isDisabled(tag: string): boolean {
  return /(?:^|\s)disabled(?=[\s>=/])/.test(tag)
}

describe("SyncNoteForm render", () => {
  it("renders nothing until the trigger opens it", async () => {
    const html = await render()

    expect(html).not.toContain('role="dialog"')
    expect(html).not.toContain("<textarea")
  })

  it("asks for the topic, the concept and the markdown once open", async () => {
    const html = await render((store) => store.openPanel())

    expect(html).toContain('role="dialog"')
    expect(html).toContain("Tema")
    expect(html).toContain("Concepto")
    expect(html).toContain("Markdown")
    expect(html).toContain("<textarea")
  })

  it("teaches the card format next to the field", async () => {
    const html = await render((store) => store.openPanel())

    expect(html).toContain("pregunta::respuesta")
  })

  it("keeps the submit disabled while the fields are empty", async () => {
    const html = await render((store) => store.openPanel())

    expect(isDisabled(buttonMatching(html, /<button[^>]*type="submit"[^>]*>/))).toBe(true)
  })

  it("shows the store's own refusal message as-is", async () => {
    const refusal =
      "No se encontraron tarjetas en la nota. Usá el formato pregunta::respuesta."
    const html = await render((store) => {
      store.openPanel()
      store.error = refusal
    })

    expect(html).toContain(refusal)
    expect(html).toContain('role="alert"')
  })
})
