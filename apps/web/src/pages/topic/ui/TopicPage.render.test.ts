import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import { useTopicsStore, type Topic } from "@/entities/topic"
import { useConceptSelection } from "@/features/select-concept"
import { useTopicSelection } from "@/features/select-topic"
import TopicPage from "./TopicPage.vue"

// Characterization test, written before the primitive migration: it pins what the
// page shows for a loaded topic, for a failed request and for a cold visit, plus
// the one action in the concept header. Never how the controls are styled.
//
// The cold-visit case is `backlog.md` D1. The page now loads the topics store
// itself on mount (`shared/lib/loadIfEmpty`), so a real cold visit ends up
// rendering the topic — but this suite cannot see that, because the repo's vitest
// environment is `node` (no jsdom, no @vue/test-utils) and renders through
// `vue/server-renderer`, where `onMounted` never runs. What this case therefore
// pins is the paint *before* the load starts, which is a real state the page still
// has (the selection is empty until mount). The mount-time load is pinned in two
// places that can see it: `shared/lib/loadIfEmpty.test.ts` proves the rule itself,
// and the `renders the new-concept form, open` capture enters `/topic/t1` cold and
// would fail without it.
//
// The repo's vitest environment is `node` (no jsdom, no @vue/test-utils), so the
// states are reached by seeding the pinia stores and rendering through
// `vue/server-renderer`; `onMounted` does not run in SSR.

const TOPICS: Topic[] = [
  {
    id: "t1",
    name: "Atención",
    concepts: [{ id: "c1", title: "Filtro atencional", file_path: "atencion/filtro.md" }],
  },
]

interface Seed {
  loading?: boolean
  error?: string | null
  select?: boolean
  /** The store's contents; empty is the cold-visit case. */
  topics?: Topic[]
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

  useTopicSelection().selectTopic(seed.select === false ? null : "t1")
  useConceptSelection().resetConcept()

  const app = createSSRApp(TopicPage, { topicId: "t1" })
  app.use(pinia)
  app.use(
    createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/", name: "dashboard", component: { template: "<div />" } },
        {
          path: "/topic/:topicId",
          name: "topic",
          component: { template: "<div />" },
        },
        {
          path: "/topic/:topicId/concept/:conceptId",
          name: "concept",
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

describe("TopicPage render", () => {
  it("names the topic and lists the concepts it holds", async () => {
    const html = await render()

    expect(html).toContain("Atención")
    expect(html).toContain("Conceptos")
    expect(html).toContain("Filtro atencional")
  })

  it("offers the concept action in the section header, by name and by title", async () => {
    const html = await render()
    const add = buttonLabelled(html, "Agregar concepto")

    expect(add).toContain('aria-label="Agregar concepto"')
    expect(add).toContain('title="Agregar concepto"')
    expect(html).not.toContain("Título del concepto")
  })

  it("offers a retry when the topics request failed", async () => {
    const html = await render({ error: "No se pudo cargar." })

    expect(html).toContain("No se pudo cargar.")
    expect(buttonLabelled(html, "Reintentar")).toBeTruthy()
  })

  // D6: a cold deep link painted "Tema no encontrado." for one frame, before
  // `onMounted` had asked for anything. An empty store that never settled is
  // pending, not missing.
  it("says it is loading when the store never settled, instead of not-found", async () => {
    const html = await render({ topics: [], loaded: false })

    expect(html).toContain("Cargando...")
    expect(html).not.toContain("Tema no encontrado.")
  })

  // `R3-pending-wedge-nonempty-unloaded`: pending is "nothing to render", not "no
  // load has settled". A store that already holds topics has something to show,
  // whatever `loaded` says, or the page spins forever over real data.
  it("shows the topics it has even before a load has settled", async () => {
    const html = await render({ loaded: false })

    expect(html).toContain("Atención")
    expect(html).not.toContain("Cargando...")
  })

  // D1's pre-mount paint: until the selection is set, the page has no topic to
  // name, so it renders the not-found state and its one way out. Reachable for a
  // frame on a cold visit; the mount handler replaces it as soon as it runs (see
  // the file comment for why this suite cannot observe that step).
  it("renders the not-found state, with a way out, before a topic is selected", async () => {
    const html = await render({ select: false })

    expect(html).toContain("Tema no encontrado.")
    expect(buttonLabelled(html, "← Volver")).toBeTruthy()
    expect(html).not.toContain("Conceptos")
  })

  // INVARIANT: the one case the migration is expected to turn red.
  it("renders its controls through the shared/ui button", async () => {
    for (const html of [
      await render(),
      await render({ error: "No se pudo cargar." }),
      await render({ select: false }),
    ]) {
      const rendered = buttons(html)
      expect(rendered.length).toBeGreaterThan(0)
      for (const button of rendered) {
        expect(button).toContain('data-slot="button"')
      }
    }
  })
})
