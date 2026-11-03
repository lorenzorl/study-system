import { beforeEach, describe, expect, it, vi } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import { useExplanationStore } from "@/entities/explanation"
import type {
  AcceptGapResponse,
  Explanation,
  ExplanationFailure,
  RangeGapResponse,
  SetGapStateRequest,
} from "@/entities/explanation"
import { ApiError } from "@/shared/api/client"
import ExplainPage from "./ExplainPage.vue"

// The repo's vitest environment is `node`: no jsdom, no @vue/test-utils. Rendering
// through `vue/server-renderer` is how the states are pinned without a dependency,
// and `onMounted` does not run in SSR — so the deep-link read is seeded by calling
// the same `load` the mount calls, which is also what makes "a reload never pays
// for the model" assertable: the POST mock stays untouched.
//
// The api module is mocked through `vi.hoisted` instead of an import of its path:
// `entities/explanation/api/explanation-api` is a segment, and the boundary guard
// reads every specifier in this file (`ARCHITECTURE.md` §10).
const api = vi.hoisted(() => ({
  explainRange: vi.fn(),
  fetchExplanation: vi.fn(),
  setGapState: vi.fn(),
  acceptGap: vi.fn(),
}))

vi.mock("@/entities/explanation/api/explanation-api", () => api)

/** `POST /api/study/explanation` and `GET /api/study/explanations/{id}` share it. */
const STORED = {
  explanation_id: "exp-1",
  document_id: "doc-1",
  from_page: 15,
  to_page: 23,
  gaps: [
    {
      gap_id: "g1",
      point: "El prototipo ata modelo e implementación.",
      page: 18,
      quote: "That crude prototype forged the essential link.",
      state: "open" as const,
    },
    {
      gap_id: "g2",
      point: "El lenguaje ubicuo no aparece como acuerdo explícito.",
      page: 21,
      quote: "The team spoke the language of the domain, not of the code.",
      state: "open" as const,
    },
  ],
}

/** The same answer in the domain shape the page renders. */
const EXPLANATION: Explanation = {
  id: "exp-1",
  documentId: "doc-1",
  fromPage: 15,
  toPage: 23,
  gaps: STORED.gaps.map((gap) => ({
    id: gap.gap_id,
    point: gap.point,
    page: gap.page,
    quote: gap.quote,
    state: gap.state,
  })),
}

/** The refusal S4 names, verbatim from the server (`api.md`). */
const TOO_SHORT =
  "this range is 9 pages and your explanation covers 62 characters: explain one section, or say more"

const DISCARDED: RangeGapResponse = {
  ...STORED.gaps[0]!,
  state: "discarded",
}

/** `POST /api/study/gaps/{gapID}/card`: the front is the reader's, the back the book's. */
const CARDED: AcceptGapResponse = {
  gap_id: "g1",
  question: "¿Qué ata el prototipo?",
  answer: "That crude prototype forged the essential link. (p.18)",
  concept_id: "con-1",
  topic_id: "top-1",
  state: "carded",
}

/**
 * The `disabled` *attribute*, not the string `disabled`
 * (`shadcn-migration.md` L16): the primitives ship `disabled:` utility classes.
 */
const DISABLED = /(?:^|\s)disabled(?=[\s>=/])/

interface Seed {
  /** The explanation on screen; `null` is "not loaded". */
  explanation?: Explanation | null
  loading?: boolean
  saving?: boolean
  error?: ExplanationFailure | null
  /** The URL to mount at, so a deep link and a fresh form differ. */
  path?: string
}

type Store = ReturnType<typeof useExplanationStore>

async function render(
  seed: Seed = {},
  prepare?: (store: Store) => Promise<unknown>,
): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)

  const store = useExplanationStore()
  store.explanation = seed.explanation === undefined ? null : seed.explanation
  store.loading = seed.loading ?? false
  store.saving = seed.saving ?? false
  store.error = seed.error ?? null
  if (prepare) await prepare(store)

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: "/explain/:documentId/:from/:to",
        name: "explain",
        component: ExplainPage,
      },
      { path: "/today", name: "today", component: { template: "<div />" } },
    ],
  })
  await router.push(seed.path ?? "/explain/doc-1/15/23")
  await router.isReady()

  const app = createSSRApp(ExplainPage)
  app.use(pinia)
  app.use(router)

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

function buttonLabelled(html: string, label: string): string | undefined {
  return buttons(html).find((chunk) => chunk.includes(label))
}

beforeEach(() => {
  api.explainRange.mockReset()
  api.fetchExplanation.mockReset()
  api.setGapState.mockReset()
  api.acceptGap.mockReset()
  api.explainRange.mockResolvedValue(STORED)
  api.fetchExplanation.mockResolvedValue(STORED)
  api.setGapState.mockResolvedValue(DISCARDED)
  api.acceptGap.mockResolvedValue(CARDED)
})

describe("ExplainPage render", () => {
  // The form: which range it explains, and one place to write.
  it("shows the range and offers the form", async () => {
    const html = await render()

    expect(html).toContain("páginas 15-23")
    expect(html).toContain("<textarea")
    expect(buttonLabelled(html, "Explicar estas páginas")).toBeTruthy()
    expect(
      DISABLED.test(buttonLabelled(html, "Explicar estas páginas") ?? ""),
    ).toBe(false)
  })

  // S2/S4: the gap carries the page that states it and the quote that proves it,
  // and the quote is a citation, not another paragraph of the reader's own words.
  it("renders each gap with its point, its page and its quote", async () => {
    const html = await render({ explanation: EXPLANATION })

    expect(html).toContain("El prototipo ata modelo e implementación.")
    expect(html).toContain("página 18")
    expect(html).toContain("<blockquote")
    expect(html).toContain("That crude prototype forged the essential link.")
    expect(html).toContain("página 21")
  })

  // "Show that it is working: the call takes 30-60 s", and the submit is disabled
  // meanwhile so two clicks cannot be two model calls.
  it("says it is working and disables the submit while the call is in flight", async () => {
    const html = await render({ saving: true })

    expect(html).toContain("30 y 60 segundos")
    expect(
      DISABLED.test(buttonLabelled(html, "Explicar estas páginas") ?? ""),
    ).toBe(true)
    // The text stays on screen while the model works on it.
    expect(html).toContain("<textarea")
  })

  // S9: the deep link reads the stored explanation and never pays for the model.
  it("loads a deep link without submitting it again", async () => {
    const html = await render({ path: "/explain/doc-1/15/23?id=exp-1" }, (store) =>
      store.load("exp-1"),
    )

    expect(api.fetchExplanation).toHaveBeenCalledWith("exp-1")
    expect(api.explainRange).not.toHaveBeenCalled()
    expect(html).toContain("El prototipo ata modelo e implementación.")
    expect(html).not.toContain("<textarea")
  })

  it("says it is loading while the stored explanation is read", async () => {
    const html = await render({ loading: true })

    expect(html).toContain("Cargando")
    expect(html).not.toContain("<textarea")
  })

  // S9: a stale link says so and offers the form again, instead of an empty page.
  it("offers the form again when a stale link 404s", async () => {
    api.fetchExplanation.mockRejectedValue(new ApiError("not found", 404))

    const html = await render({ path: "/explain/doc-1/15/23?id=gone" }, (store) =>
      store.load("gone"),
    )

    expect(html).toContain("ya no está guardada")
    expect(html).toContain("<textarea")
    expect(buttonLabelled(html, "Reintentar")).toBeUndefined()
  })

  // S4: the refusal is the server's sentence, verbatim, and the reader's text is
  // still there — losing it is the worst outcome on this page.
  it("shows the server's refusal sentence with the form still there", async () => {
    const html = await render({
      error: { message: TOO_SHORT, status: 400, source: "explain" },
    })

    expect(html).toContain(TOO_SHORT)
    expect(html).toContain("<textarea")
    // A refusal is fixed by writing more, not by asking the model again.
    expect(buttonLabelled(html, "Reintentar")).toBeUndefined()
  })

  // S5: a range too wide to explain reaches the page as its own sentence, never as
  // a spinner that never ends.
  it("shows the wide-range refusal as a sentence", async () => {
    const refusal =
      "this range is 30 pages and explaining a range is limited to 12: explain one section at a time"

    const html = await render({
      error: { message: refusal, status: 400, source: "explain" },
    })

    expect(html).toContain(refusal)
    expect(html).not.toContain("30 y 60 segundos")
  })

  it("says the document or range is unknown when the submit 404s", async () => {
    const html = await render({
      error: { message: "not found", status: 404, source: "explain" },
    })

    expect(html).toContain("no existe")
    expect(html).not.toContain("not found")
    expect(buttonLabelled(html, "Reintentar")).toBeUndefined()
  })

  // A model failure stores nothing and costs another call to try again, so the page
  // states that and leaves the decision to the reader.
  it("says nothing was stored after a model failure and offers a retry", async () => {
    const html = await render({
      error: { message: "internal server error", status: 500, source: "explain" },
    })

    expect(html).toContain("No se guardó nada")
    expect(html).not.toContain("internal server error")
    expect(buttonLabelled(html, "Reintentar")).toBeTruthy()
    expect(html).toContain("<textarea")
  })

  // S7: a discarded gap stops being actionable and stays readable.
  it("keeps a discarded gap readable and offers to undo it", async () => {
    const html = await render({ explanation: EXPLANATION }, (store) =>
      store.setGapState(EXPLANATION.gaps[0]!, "discarded"),
    )

    expect(api.setGapState).toHaveBeenCalledWith("g1", "discarded")
    expect(html).toContain("Descartada")
    expect(buttonLabelled(html, "Deshacer")).toBeTruthy()
    // The evidence is not deleted with the state.
    expect(html).toContain("That crude prototype forged the essential link.")
  })

  it("sends the state the reader asked for, with the gap's id", async () => {
    const bodies: SetGapStateRequest[] = []
    api.setGapState.mockImplementation((_gapId: string, state: SetGapStateRequest["state"]) => {
      bodies.push({ state })
      return Promise.resolve({ ...STORED.gaps[1]!, state })
    })

    await render({ explanation: EXPLANATION }, async (store) => {
      await store.setGapState(EXPLANATION.gaps[1]!, "discarded")
      await store.setGapState(EXPLANATION.gaps[1]!, "open")
    })

    expect(api.setGapState.mock.calls).toEqual([
      ["g2", "discarded"],
      ["g2", "open"],
    ])
    expect(bodies).toEqual([{ state: "discarded" }, { state: "open" }])
  })

  // S8: no number anywhere near an open response.
  it("never shows a comprehension score", async () => {
    const html = await render({ explanation: EXPLANATION })

    expect(html).not.toMatch(/puntaje|puntos|score|%/i)
  })

  // U3: the card. The front is editable and starts as the gap's point — a statement
  // the reader turns into a question — beside the discard control.
  it("pre-fills each gap's card front with its point and offers the card", async () => {
    const html = await render({ explanation: EXPLANATION })

    expect(html).toContain("Pregunta de la tarjeta")
    expect(html).toContain('value="El prototipo ata modelo e implementación."')
    expect(
      buttons(html).filter((button) => button.includes("Crear tarjeta")),
    ).toHaveLength(EXPLANATION.gaps.length)

    // The way back to the form. Without it, a URL naming a stored explanation would
    // leave the reader unable to explain that range again.
    expect(buttonLabelled(html, "Explicar de nuevo")).toBeDefined()
  })

  // S6: a refusal is the server's sentence, and the row is still a gap the reader can
  // accept — nothing was written, so nothing says it was.
  it("shows the server's refusal sentence and keeps the row un-carded", async () => {
    const html = await render({
      explanation: EXPLANATION,
      error: {
        message: "this gap already has a card",
        status: 409,
        source: "card",
      },
    })

    expect(html).toContain("this gap already has a card")
    expect(html).toContain("El prototipo ata modelo e implementación.")
    expect(html).not.toContain("Convertida en tarjeta")
    expect(buttonLabelled(html, "Crear tarjeta")).toBeTruthy()
  })

  // S6/S7: a carded gap says so, and the state only moves forward — no discard, no
  // second card for the same gap.
  it("marks a carded gap and offers neither discard nor another card", async () => {
    const html = await render({ explanation: EXPLANATION }, (store) =>
      store.acceptGap(EXPLANATION.gaps[0]!, CARDED.question),
    )

    expect(api.acceptGap).toHaveBeenCalledWith("g1", CARDED.question)
    expect(html).toContain("Convertida en tarjeta")
    // One card control left, for the gap that is still open.
    expect(
      buttons(html).filter((button) => button.includes("Crear tarjeta")),
    ).toHaveLength(1)
    expect(
      buttons(html).filter((button) => button.includes("Descartar")),
    ).toHaveLength(1)
  })

  it("says so when the explanation left nothing out", async () => {
    const html = await render({
      explanation: { ...EXPLANATION, gaps: [] },
    })

    expect(html).toContain("No quedaron lagunas")
    expect(html).not.toContain("<blockquote")
  })
})
