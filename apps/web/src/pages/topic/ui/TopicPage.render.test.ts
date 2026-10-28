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
// The cold-visit case is `backlog.md` D1, pinned as current behaviour rather than
// fixed here: `TopicPage` never calls `loadTopics()`, so `currentTopic` resolves
// only when something else already loaded the store. This test seeds the store for
// the loaded cases and leaves the selection empty for the cold one, which is
// exactly the state a deep link renders.
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
}

async function render(seed: Seed = {}): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)

  const topics = useTopicsStore()
  topics.topics = TOPICS
  topics.loading = seed.loading ?? false
  topics.error = seed.error ?? null

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

  // D1, pinned as it is: with nothing in the selection a cold visit renders the
  // not-found state, and its only way out is the dashboard.
  it("renders the not-found state, with a way out, for a cold visit", async () => {
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
