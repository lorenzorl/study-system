import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import { useFlashcardCache, type Flashcard } from "@/entities/flashcard"
import { useTopicsStore, type Topic } from "@/entities/topic"
import { useConceptSelection } from "@/features/select-concept"
import { useTopicSelection } from "@/features/select-topic"
import ConceptPage from "./ConceptPage.vue"

// Characterization test, written before the primitive migration: it pins the two
// actions the page offers, when the flashcards one is unavailable, and the two
// states a failed or cold visit renders. Never how they are styled — the migration
// replaces the markup of every control here.
//
// The repo's vitest environment is `node` (no jsdom, no @vue/test-utils), so the
// states are reached by seeding the pinia stores and rendering through
// `vue/server-renderer`. `onMounted` does not run in SSR, which is why the topic
// and concept selection are seeded instead of arriving through the route props.

const TOPICS: Topic[] = [
  {
    id: "t1",
    name: "Atención",
    concepts: [{ id: "c1", title: "Filtro atencional", file_path: "atencion/filtro.md" }],
  },
]

const CARD: Flashcard = {
  id: "f1",
  concept_id: "c1",
  question: "¿Qué es el filtro atencional?",
  answer: "El mecanismo que selecciona qué entra en la conciencia.",
  obsidian_id: "obs-1",
  created_at: "2026-10-01T10:00:00Z",
}

/**
 * The `disabled` *attribute*, not the string `disabled`
 * (`shadcn-migration.md` L16). The primitives ship `disabled:` utility classes, so
 * `toContain("disabled")` passes against an enabled and a disabled button alike.
 */
const DISABLED = /(?:^|\s)disabled(?=[\s>=/])/

interface Seed {
  loading?: boolean
  error?: string | null
  conceptId?: string | null
  cards?: Flashcard[]
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

  useTopicSelection().selectTopic("t1")
  useConceptSelection().selectConcept(seed.conceptId === undefined ? "c1" : seed.conceptId)
  useFlashcardCache().setCachedFlashcards("c1", seed.cards ?? [])

  const app = createSSRApp(ConceptPage, { topicId: "t1", conceptId: "c1" })
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
          path: "/topic/:topicId/concept/:conceptId/flashcards",
          name: "flashcards",
          component: { template: "<div />" },
        },
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

describe("ConceptPage render", () => {
  it("offers flashcards and Feynman, and says how many cards the concept holds", async () => {
    const html = await render({ cards: [CARD] })

    expect(html).toContain("Filtro atencional")
    expect(html).toContain("atencion/filtro.md")
    expect(html).toContain("Tarjetas")
    expect(html).toContain("1 tarjeta")
    expect(html).toContain("Técnica Feynman")
  })

  it("pluralises the card count, and still offers Feynman with none", async () => {
    const html = await render()

    expect(html).toContain("0 tarjetas")
    expect(buttonLabelled(html, "Técnica Feynman")).not.toMatch(DISABLED)
  })

  it("makes the flashcards action unavailable when there are no cards", async () => {
    const html = await render({ cards: [CARD] })

    expect(buttonLabelled(html, "Tarjetas")).not.toMatch(DISABLED)
    expect(buttonLabelled(await render(), "Tarjetas")).toMatch(DISABLED)
  })

  it("offers a retry when the topics request failed", async () => {
    const html = await render({ error: "No se pudo cargar." })

    expect(html).toContain("No se pudo cargar.")
    expect(buttonLabelled(html, "Reintentar")).toBeTruthy()
    expect(html).not.toContain("Técnica Feynman")
  })

  it("offers the way back to the topic when the concept is not there", async () => {
    const html = await render({ conceptId: null })

    expect(html).toContain("Concepto no encontrado.")
    expect(buttonLabelled(html, "← Volver al Tema")).toBeTruthy()
  })

  // D6: same first frame as `TopicPage` — an empty store that never settled is
  // pending, not a missing concept.
  it("says it is loading when the store never settled, instead of not-found", async () => {
    const html = await render({ topics: [], loaded: false, conceptId: null })

    expect(html).toContain("Cargando...")
    expect(html).not.toContain("Concepto no encontrado.")
  })

  // INVARIANT: the one case the migration is expected to turn red. Every control
  // on this page is a plain button, so all of them render through the shared/ui
  // primitive — in all three states that carry one.
  it("renders its controls through the shared/ui button", async () => {
    for (const html of [
      await render({ cards: [CARD] }),
      await render({ error: "No se pudo cargar." }),
      await render({ conceptId: null }),
    ]) {
      const rendered = buttons(html)
      expect(rendered.length).toBeGreaterThan(0)
      for (const button of rendered) {
        expect(button).toContain('data-slot="button"')
      }
    }
  })
})
