import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { useCaptureMaterial } from "../model/useCaptureMaterial"
import CaptureMaterialForm from "./CaptureMaterialForm.vue"

// The repo's vitest environment is `node`: no jsdom, no @vue/test-utils. Rendering
// through `vue/server-renderer` pins which fields each source shows and how a
// refusal looks, without a dependency. Interaction (typing, Escape, focus) is
// verified by hand in the browser harness.

type Store = ReturnType<typeof useCaptureMaterial>

function render(seed: (store: Store) => void = () => {}): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)
  seed(useCaptureMaterial())

  const app = createSSRApp(CaptureMaterialForm)
  app.use(pinia)

  return renderToString(app)
}

function buttonWithClass(html: string, className: string): string {
  const match = html.match(new RegExp(`<button[^>]*${className}[^>]*>`))
  expect(match).not.toBeNull()

  return match?.[0] ?? ""
}

describe("CaptureMaterialForm render", () => {
  it("offers the three sources and starts on a file upload", async () => {
    const html = await render()

    expect(html).toContain("Archivo")
    expect(html).toContain("Texto")
    expect(html).toContain("URL")
    // Exactly one source is selected at a time.
    expect(buttonWithClass(html, "capture__tab--active")).toContain(
      'aria-pressed="true"',
    )
    expect(html.match(/aria-pressed="true"/g)).toHaveLength(1)

    // One source at a time: the file source's fields, and none of the others.
    expect(html).toContain('type="file"')
    expect(html).toContain('accept="application/pdf"')
    expect(html).not.toContain("<textarea")
    expect(html).not.toContain('type="url"')
  })

  it("asks for the topic every source needs", async () => {
    const html = await render()

    expect(html).toContain("Tema")
    expect(html).toContain("Tema (ej. DDD, Matemáticas)")
  })

  it("keeps the submit disabled while the fields are empty", async () => {
    const html = await render()

    expect(buttonWithClass(html, "capture__submit")).toContain("disabled")
  })

  it("shows the server's own refusal message", async () => {
    const refusal =
      "uploads require the STUDY_OPENROUTER_API_KEY environment variable"
    const html = await render((store) => {
      store.error = refusal
    })

    expect(html).toContain(refusal)
    expect(html).toContain('role="alert"')
  })
})
