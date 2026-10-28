import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import { useFlashcardCache, type Flashcard } from "@/entities/flashcard"
import { useReviewStore, type DueCard } from "@/entities/review"
import { useTopicsStore, type Topic } from "@/entities/topic"
import { useConceptSelection } from "@/features/select-concept"
import { useTopicSelection } from "@/features/select-topic"
import FlashcardSessionPage from "./FlashcardSessionPage.vue"

// Characterization test, written before the primitive migration: it pins the two
// sessions this page can show (concept mode and the FSRS due-review mode), which
// controls each one offers and when the primary ones are unavailable. Never how
// they are styled — the migration replaces every class in the file, and the reader
// depends on the names and the disabled state, not on the CSS.
//
// The repo's vitest environment is `node` (no jsdom, no @vue/test-utils), so the
// states are reached by seeding the pinia stores and rendering through
// `vue/server-renderer`. `onMounted` does not run in SSR, which is why the concept
// and topic selection are seeded here instead of arriving through the route props.

const TOPICS: Topic[] = [
  {
    id: "t1",
    name: "Atención",
    concepts: [{ id: "c1", title: "Filtro atencional", file_path: "atencion/filtro.md" }],
  },
]

const FLASHCARDS: Flashcard[] = [
  {
    id: "f1",
    concept_id: "c1",
    question: "¿Qué es el filtro atencional?",
    answer: "El mecanismo que selecciona qué entra en la conciencia.",
    obsidian_id: "obs-1",
    created_at: "2026-10-01T10:00:00Z",
  },
  {
    id: "f2",
    concept_id: "c1",
    question: "¿Qué es la atención dividida?",
    answer: "Dos tareas a la vez.",
    obsidian_id: "obs-2",
    created_at: "2026-10-02T10:00:00Z",
  },
]

const ONE_DUE_CARD: DueCard[] = [
  {
    flashcard_id: "f1",
    question: "¿Qué es el filtro atencional?",
    answer: "El mecanismo que selecciona qué entra en la conciencia.",
    concept_title: "Filtro atencional",
    topic_name: "Atención",
    next_review: "2026-10-20T10:00:00Z",
  },
]

/** The concept mode the page resolves to once a topic and a concept are chosen. */
function selectConcept(cards: Flashcard[]): void {
  useTopicsStore().topics = TOPICS
  useTopicSelection().selectTopic("t1")
  useConceptSelection().selectConcept("c1")
  useFlashcardCache().setCachedFlashcards("c1", cards)
}

function renderPage(seed: () => void = () => {}): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)
  seed()

  const app = createSSRApp(FlashcardSessionPage, { topicId: "t1", conceptId: "c1" })
  app.use(pinia)
  app.use(
    createRouter({
      history: createMemoryHistory(),
      // `goBackToConcept` and `goBackToDue` only resolve inside a click, so the
      // routes exist to give `useRouter()` something to inject.
      routes: [
        { path: "/", name: "dashboard", component: { template: "<div />" } },
        { path: "/study/review", name: "review", component: { template: "<div />" } },
        { path: "/study/due", name: "due-cards", component: { template: "<div />" } },
        {
          path: "/topic/:topicId/concept/:conceptId",
          name: "concept",
          component: { template: "<div />" },
        },
      ],
    }),
  )

  return renderToString(app)
}

interface RenderedButton {
  /** The opening tag, for attribute assertions. */
  tag: string
  /** What a screen reader announces: `aria-label` when present, else the text. */
  name: string
}

/**
 * Every rendered `<button>`, read from the markup rather than from the component's
 * state, and named the way assistive technology names it.
 *
 * Fragment markers around slot content are an SSR artifact of a slot-rendering
 * primitive (`<!--[-->Cancelar<!--]-->`), so comments are stripped before the
 * text is read; the accessible name would otherwise carry the markers.
 */
function buttons(html: string): RenderedButton[] {
  return [...html.matchAll(/<button([^>]*)>([\s\S]*?)<\/button>/g)].map((match) => {
    const tag = match[1]
    const text = match[2]
      .replace(/<!--[\s\S]*?-->/g, "")
      .replace(/<[^>]*>/g, " ")
      .replace(/\s+/g, " ")
      .trim()
    const label = tag.match(/aria-label="([^"]*)"/)

    return { tag, name: label ? label[1] : text }
  })
}

function buttonNames(html: string): string[] {
  return buttons(html).map((button) => button.name)
}

/**
 * Whether a rendered opening tag carries the `disabled` **attribute**.
 *
 * A substring check on "disabled" is wrong for the reasons the last migration unit
 * recorded: the `shared/ui` button's utility classes are full of `disabled:`
 * variants, so `toContain("disabled")` reports an enabled button as disabled.
 */
function isDisabled(tag: string): boolean {
  return /(?:^|\s)disabled(?=[\s>=/])/.test(tag)
}

function buttonNamed(html: string, name: string): RenderedButton {
  const found = buttons(html).find((button) => button.name === name)
  expect(found, `no button named "${name}" in:\n${buttonNames(html).join(" | ")}`).toBeDefined()

  return found as RenderedButton
}

describe("FlashcardSessionPage render — concept mode", () => {
  it("says it is loading while the topics load", async () => {
    const html = await renderPage(() => {
      useTopicsStore().loading = true
    })

    expect(html).toContain("Cargando...")
    expect(buttonNames(html)).not.toContain("Reintentar")
  })

  it("keeps the load error readable and offers a retry", async () => {
    const html = await renderPage(() => {
      useTopicsStore().error = "No se pudieron cargar los temas."
    })

    expect(html).toContain("No se pudieron cargar los temas.")
    expect(buttonNames(html)).toContain("Reintentar")
  })

  it("says the concept was not found and offers the way back", async () => {
    const html = await renderPage()

    expect(html).toContain("Concepto no encontrado.")
    expect(buttonNamed(html, "← Volver").tag).toBeTruthy()
  })

  it("explains that the concept has no cards and offers the panel", async () => {
    const html = await renderPage(() => selectConcept([]))

    expect(html).toContain("No hay tarjetas para estudiar. Sincronizá la nota primero.")
    expect(buttonNames(html)).toContain("← Volver al Panel")
  })

  it("opens on the first card, with the previous one unavailable", async () => {
    const html = await renderPage(() => selectConcept(FLASHCARDS))

    expect(html).toContain("1 / 2")
    expect(html).toContain("¿Qué es el filtro atencional?")
    expect(isDisabled(buttonNamed(html, "Tarjeta anterior").tag)).toBe(true)
    expect(isDisabled(buttonNamed(html, "Siguiente tarjeta").tag)).toBe(false)
    expect(buttonNames(html)).toContain("Siguiente tarjeta")
    expect(html).toContain("Teclas ← → para navegar")
  })

  it("offers to finish once the last card is the only card", async () => {
    const html = await renderPage(() => selectConcept([FLASHCARDS[0]]))

    expect(html).toContain("1 / 1")
    // The next control keeps its accessible name (`aria-label`) while its visible
    // text becomes the deck's last-card affordance.
    expect(buttonNamed(html, "Siguiente tarjeta").tag).toBeTruthy()
    expect(html).toContain("Finalizar →")
  })
})

describe("FlashcardSessionPage render — due-review mode", () => {
  it("says it is loading while the due cards load", async () => {
    const html = await renderPage(() => {
      useReviewStore().startSession()
      useReviewStore().loading = true
    })

    expect(html).toContain("Cargando...")
    expect(buttonNames(html)).not.toContain("Reintentar")
  })

  it("keeps the review error readable and offers a retry", async () => {
    const html = await renderPage(() => {
      useReviewStore().startSession()
      useReviewStore().error = "No se pudieron cargar los repasos."
    })

    expect(html).toContain("No se pudieron cargar los repasos.")
    expect(buttonNames(html)).toContain("Reintentar")
  })

  it("says there is nothing due and offers the way back", async () => {
    const html = await renderPage(() => {
      useReviewStore().startSession()
    })

    expect(html).toContain("No hay tarjetas para repasar.")
    expect(buttonNames(html)).toContain("← Volver")
  })

  it("shows the card to review and hides the grades until it is revealed", async () => {
    const html = await renderPage(() => {
      useReviewStore().startSession()
      useReviewStore().dueCards = ONE_DUE_CARD
    })

    expect(html).toContain("1 / 1")
    expect(html).toContain("¿Qué es el filtro atencional?")
    // Grading before the answer is revealed is not offered at all: the four
    // grades only exist behind the flip, which SSR cannot reach.
    for (const grade of ["Otra vez (1)", "Difícil (2)", "Bien (3)", "Fácil (4)"]) {
      expect(buttonNames(html)).not.toContain(grade)
    }
  })

  it("reports a finished review and offers to restart or leave", async () => {
    const html = await renderPage(() => {
      useReviewStore().startSession()
      useReviewStore().dueCards = ONE_DUE_CARD
      useReviewStore().completedCount = 1
    })

    expect(html).toContain("¡Repaso Completado!")
    expect(html).toContain("Repasaste 1 de 1 tarjeta.")
    expect(buttonNames(html)).toEqual(
      expect.arrayContaining(["Repasar de nuevo", "Volver al Panel"]),
    )
  })
})

describe("FlashcardSessionPage — the migrated controls", () => {
  // What the migration changed, and all it changed: the actions render through the
  // `shared/ui` button. Pinning the primitive's own marker is the one honest way to
  // show the hand-rolled `<button>` is gone; the contract tests above show the
  // names and the disabled states survived it.
  //
  // The four grade buttons are NOT covered here and are not meant to be: they keep
  // their hand-rolled markup (see the component), and they only exist after the
  // card is flipped, which SSR cannot reach.
  it("renders the empty state's action through the shared/ui button", async () => {
    const html = await renderPage()
    const rendered = buttons(html)

    expect(rendered).toHaveLength(1)
    for (const button of rendered) expect(button.tag).toContain('data-slot="button"')
  })

  it("renders both navigation actions through the shared/ui button", async () => {
    const html = await renderPage(() => selectConcept(FLASHCARDS))
    // The card is a button of its own (`entities/flashcard`), so the two nav
    // controls are found by name rather than by position.
    for (const name of ["Tarjeta anterior", "Siguiente tarjeta"]) {
      expect(buttonNamed(html, name).tag).toContain('data-slot="button"')
    }
  })
})
