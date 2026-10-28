import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import CreateConceptForm from "./CreateConceptForm.vue"

// Characterization test, written before the primitive migration: it pins what the
// form promises the reader (which field it asks for, when the submit is available,
// which two actions exist), never how it is styled. The other two forms in this
// unit already had one; this one had nothing, so the migration would have had no
// evidence that its contract survived.

interface Props {
  modelValue?: string
  submitting?: boolean
}

function render(props: Props = {}): Promise<string> {
  const app = createSSRApp(CreateConceptForm, { modelValue: "", ...props })

  return renderToString(app)
}

/**
 * The first `<button>` matching `pattern`, whatever state it is in. The pattern
 * targets a semantic attribute (`type="submit"`), never a styling class: the
 * migration replaces the classes, the semantics are what the reader depends on.
 */
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
 * variants, so `toContain("disabled")` matched a class and reported an enabled
 * button as disabled. The attribute is what a reader and the browser act on, so
 * the assertion looks for the token and not the prefix of a class name.
 */
function isDisabled(tag: string): boolean {
  return /(?:^|\s)disabled(?=[\s>=/])/.test(tag)
}

/** The opening tag of the first element named `name` in the rendered markup. */
function openingTag(html: string, name: string): string {
  const start = html.indexOf(`<${name}`)
  expect(start).toBeGreaterThanOrEqual(0)

  return html.slice(start, html.indexOf(">", start) + 1)
}

describe("CreateConceptForm render", () => {
  it("asks for the concept title", async () => {
    const html = await render()

    expect(html).toContain("Título del concepto")
    expect(html).toContain('type="text"')
  })

  it("keeps the submit disabled while the title is empty", async () => {
    const html = await render()

    expect(isDisabled(buttonMatching(html, /<button[^>]*type="submit"[^>]*>/))).toBe(true)
  })

  it("enables the submit once there is a title", async () => {
    const html = await render({ modelValue: "Agregados" })

    expect(isDisabled(buttonMatching(html, /<button[^>]*type="submit"[^>]*>/))).toBe(false)
  })

  it("offers the two actions by name, whatever their styling", async () => {
    const html = await render({ modelValue: "Agregados" })
    // Fragment markers around slot content are an SSR artifact of the primitive,
    // not content: they are stripped before the name is read.
    const markup = html.replace(/<!--.*?-->/g, "")

    expect(html).toContain("Guardar")
    expect(markup).toMatch(/<button[^>]*type="button"[^>]*>\s*Cancelar\s*<\/button>/)
  })

  it("marks the field and both actions disabled while the request is in flight", async () => {
    const html = await render({ modelValue: "Agregados", submitting: true })

    expect(isDisabled(openingTag(html, "input"))).toBe(true)
    expect(isDisabled(buttonMatching(html, /<button[^>]*type="submit"[^>]*>/))).toBe(true)
    expect(isDisabled(buttonMatching(html, /<button[^>]*type="button"[^>]*>/))).toBe(true)
  })
})
