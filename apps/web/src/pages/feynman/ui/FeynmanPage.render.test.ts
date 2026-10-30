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

interface Seed {
  selectConcept?: boolean
  /** The store's contents; empty is the cold-visit case. */
  topics?: Topic[]
  loading?: boolean
  error?: string | null
  /** Whether a load has ever settled. `true` unless a test wants the first frame. */
  loaded?: boolean
}

async function render(seed: Seed = {}): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)

  const topics = useTopicsStore()
  topics.topics = seed.topics ?? TOPICS
  topics.loading = seed.loading ?? false
  topics.error = seed.error ?? null
  topics.loaded = seed.loaded ?? true
  useTopicSelection().selectTopic("t1")
  useConceptSelection().selectConcept(
    seed.selectConcept === false ? null : "c1",
  )

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
    const html = await render({ selectConcept: false })

    expect(html).toContain("Concepto no encontrado.")
    expect(html).not.toContain("<textarea")
    expect(buttons(html)).toHaveLength(0)
  })

  // D6, the third loose end of D1: this page had no error state and no retry, so a
  // failed load left it on the not-found paragraph with nothing to press — and the
  // user could not tell "it is not there" from "we could not ask".
  it("keeps a failed load readable and offers a retry", async () => {
    const html = await render({ error: "No se pudo cargar.", topics: [] })

    expect(html).toContain("No se pudo cargar.")
    expect(buttonLabelled(html, "Reintentar")).toBeTruthy()
    expect(html).not.toContain("Concepto no encontrado.")
    expect(html).not.toContain("<textarea")
  })

  it("says it is loading when the store never settled, instead of not-found", async () => {
    const html = await render({ topics: [], loaded: false })

    expect(html).toContain("Cargando...")
    expect(html).not.toContain("Concepto no encontrado.")
  })

  // `R3-feynman-error-overrides-found`: an error left over from an earlier failed
  // load used to win over a concept that does resolve, so the form disappeared
  // while the data was there. The error only wins when there is nothing to show.
  it("shows the concept it has, even when a stale error is still set", async () => {
    const html = await render({ error: "No se pudo cargar." })

    expect(html).toContain("Filtro atencional")
    expect(html).toContain("<textarea")
    expect(html).not.toContain("No se pudo cargar.")
  })

  // INVARIANT: the one case the migration is expected to turn red.
  it("renders its control through the shared/ui button", async () => {
    const html = await render()

    expect(buttonLabelled(html, "Explicar este concepto")).toContain('data-slot="button"')
  })
})
