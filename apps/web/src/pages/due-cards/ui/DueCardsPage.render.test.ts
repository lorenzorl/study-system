import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import { useReviewStore, type DueCard } from "@/entities/review"
import DueCardsPage from "./DueCardsPage.vue"

// Characterization test, written before the primitive migration: it pins the three
// states the page can show — a list with the way into the session, nothing due, and
// a failed request — and the only action the page itself owns. Never how it looks.
//
// The repo's vitest environment is `node` (no jsdom, no @vue/test-utils), so the
// states are reached by seeding the review store and rendering through
// `vue/server-renderer`; `onMounted` does not run in SSR.

const DUE: DueCard[] = [
  {
    flashcard_id: "f1",
    question: "¿Qué es el filtro atencional?",
    answer: "El mecanismo que selecciona qué entra en la conciencia.",
    concept_title: "Filtro atencional",
    topic_name: "Atención",
    next_review: "2026-10-20T10:00:00Z",
  },
]

interface Seed {
  cards?: DueCard[]
  error?: string | null
}

async function render(seed: Seed = {}): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)

  const review = useReviewStore()
  review.dueCards = seed.cards ?? DUE
  review.error = seed.error ?? null
  review.loading = false

  const app = createSSRApp(DueCardsPage)
  app.use(pinia)
  app.use(
    createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/study/due", name: "due-cards", component: { template: "<div />" } },
        { path: "/study/review", name: "review", component: { template: "<div />" } },
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

describe("DueCardsPage render", () => {
  it("lists what is due, and offers the way into the session", async () => {
    const html = await render()

    expect(html).toContain("1 tarjeta para repasar hoy")
    expect(html).toContain("¿Qué es el filtro atencional?")
    expect(html).toContain("Filtro atencional")
    expect(buttonLabelled(html, "Comenzar repaso")).toBeTruthy()
  })

  it("pluralises the count", async () => {
    const html = await render({
      cards: [...DUE, { ...DUE[0], flashcard_id: "f2" }],
    })

    expect(html).toContain("2 tarjetas para repasar hoy")
  })

  it("says so when nothing is due, and offers no way in", async () => {
    const html = await render({ cards: [] })

    expect(html).toContain("No hay tarjetas para repasar hoy.")
    expect(html).not.toContain("Comenzar repaso")
    expect(buttons(html)).toHaveLength(0)
  })

  it("offers a retry when the request failed", async () => {
    const html = await render({ error: "No se pudo cargar." })

    expect(html).toContain("No se pudo cargar.")
    expect(buttonLabelled(html, "Reintentar")).toBeTruthy()
  })

  // INVARIANT: the one case the migration is expected to turn red. Only the two
  // states that carry a control are asserted here; the empty state renders none at
  // all, which the case above pins.
  it("renders its controls through the shared/ui button", async () => {
    for (const html of [await render(), await render({ error: "No se pudo cargar." })]) {
      const rendered = buttons(html)
      expect(rendered.length).toBeGreaterThan(0)
      for (const button of rendered) {
        expect(button).toContain('data-slot="button"')
      }
    }
  })
})
