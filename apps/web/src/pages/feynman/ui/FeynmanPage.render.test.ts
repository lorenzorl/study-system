import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import { useTopicsStore, type Topic } from "@/entities/topic"
import { useConceptSelection } from "@/features/select-concept"
import { useTopicSelection } from "@/features/select-topic"
import FeynmanPage from "./FeynmanPage.vue"

// Characterization test, written before the primitive migration: it pins what the
// reader gets — the concept's name, the instruction, a place to write, and one
// primary action that is unavailable until there is something to explain. Never how
// any of it is styled.
//
// The repo's vitest environment is `node` (no jsdom, no @vue/test-utils), so the
// states are reached by seeding the pinia stores and rendering through
// `vue/server-renderer`. `onMounted` does not run in SSR, which is why the topic and
// concept selection are seeded; and with no event loop there is no way to type into
// the textarea, so the enabled state of the action is not reachable here.

const TOPICS: Topic[] = [
  {
    id: "t1",
    name: "Atención",
    concepts: [{ id: "c1", title: "Filtro atencional", file_path: "atencion/filtro.md" }],
  },
]

/**
 * The `disabled` *attribute*, not the string `disabled`
 * (`shadcn-migration.md` L16): the primitives ship `disabled:` utility classes.
 */
const DISABLED = /(?:^|\s)disabled(?=[\s>=/])/

async function render(selectConcept = true): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)

  useTopicsStore().topics = TOPICS
  useTopicSelection().selectTopic("t1")
  useConceptSelection().selectConcept(selectConcept ? "c1" : null)

  const app = createSSRApp(FeynmanPage, { topicId: "t1", conceptId: "c1" })
  app.use(pinia)
  app.use(
    createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/", name: "dashboard", component: { template: "<div />" } },
        {
          path: "/topic/:topicId/concept/:conceptId/feynman",
          name: "feynman",
          component: { template: "<div />" },
        },
      ],
    }),
  )

  const html = await renderToString(app)
  return html.replace(/<!--[\s\S]*?-->/g, "").replace(/\s+/g, " ")
}

/** Every opening `<button>` tag with its content, so a label can be matched. */
function buttons(html: string): string[] {
  return html
    .split(/(?=<button)/)
    .filter((chunk) => chunk.startsWith("<button"))
    .map((chunk) => chunk.slice(0, chunk.indexOf("</button>") + "</button>".length))
}

function buttonLabelled(html: string, label: string): string {
  const button = buttons(html).find((chunk) => chunk.includes(label))
  if (!button) throw new Error(`no button labelled ${label} in: ${html}`)
  return button
}

describe("FeynmanPage render", () => {
  it("names the concept, explains the technique and offers a place to write", async () => {
    const html = await render()

    expect(html).toContain("Filtro atencional")
    expect(html).toContain("Explicá este concepto con tus propias palabras")
    expect(html).toContain("<textarea")
    expect(html).toContain('aria-label="Explicá Filtro atencional con tus propias palabras"')
  })

  it("does not offer to explain an empty explanation", async () => {
    const html = await render()

    expect(buttonLabelled(html, "Explicar este concepto")).toMatch(DISABLED)
  })

  it("renders the not-found state when there is no concept to explain", async () => {
    const html = await render(false)

    expect(html).toContain("Concepto no encontrado.")
    expect(html).not.toContain("<textarea")
    expect(buttons(html)).toHaveLength(0)
  })

  // INVARIANT: the one case the migration is expected to turn red.
  it("renders its control through the shared/ui button", async () => {
    const html = await render()

    expect(buttonLabelled(html, "Explicar este concepto")).toContain('data-slot="button"')
  })
})
