import { beforeEach, describe, expect, it, vi } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import { useTodayStore } from "@/entities/today"
import type { Today, TodayEntry } from "@/entities/today"
import { ApiError } from "@/shared/api/client"
import TodayPage from "./TodayPage.vue"

// The repo's vitest environment is `node`: no jsdom, no @vue/test-utils. Rendering
// through `vue/server-renderer` is how the states are pinned without a dependency,
// and `onMounted` does not run in SSR — so each case seeds the store and that is
// also how loading, error and nothing-to-do are reachable at all.
//
// The api module is mocked through `vi.hoisted` instead of an import of its path:
// `entities/today/api/today-api` is a segment, and the boundary guard reads every
// specifier in this file (`ARCHITECTURE.md` §10).
const api = vi.hoisted(() => ({
  fetchToday: vi.fn(),
  setReading: vi.fn(),
}))

vi.mock("@/entities/today/api/today-api", () => api)

/** `GET /api/study/today` as the handler answers it (S13). */
const PROJECTION = {
  due_count: 2,
  entries: [
    {
      kind: "read_not_distilled",
      document_id: "doc-1",
      document_title: "DDD",
      from_page: 24,
      to_page: 36,
      read_at: "2026-11-02T10:00:00Z",
    },
    {
      kind: "plan_next",
      document_id: "doc-1",
      document_title: "DDD",
      title: "Capítulo 5",
      from_page: 100,
      to_page: 118,
      note: "volver si hace falta",
    },
  ],
}

/** The same answer in the domain shape the page renders. */
const TODAY: Today = {
  dueCount: 2,
  entries: [
    {
      kind: "read_not_distilled",
      documentId: "doc-1",
      documentTitle: "DDD",
      fromPage: 24,
      toPage: 36,
    },
    {
      kind: "plan_next",
      documentId: "doc-1",
      documentTitle: "DDD",
      title: "Capítulo 5",
      fromPage: 100,
      toPage: 118,
      note: "volver si hace falta",
    },
  ],
}

const STORED_ROW = {
  document_id: "doc-1",
  from_page: 100,
  to_page: 118,
  state: "read" as const,
  created_at: "2026-11-02T10:00:00Z",
  updated_at: "2026-11-02T10:00:00Z",
}

interface Seed {
  /** The projection; `null` is "never loaded". */
  today?: Today | null
  loading?: boolean
  error?: string | null
}

type Store = ReturnType<typeof useTodayStore>

async function render(
  seed: Seed = {},
  prepare?: (store: Store) => Promise<void>,
): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)

  const store = useTodayStore()
  store.today = seed.today === undefined ? TODAY : seed.today
  store.loading = seed.loading ?? false
  store.error = seed.error ?? null
  if (prepare) await prepare(store)

  const app = createSSRApp(TodayPage)
  app.use(pinia)
  app.use(
    createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/", name: "dashboard", component: { template: "<div />" } },
        {
          path: "/study/review",
          name: "review",
          component: { template: "<div />" },
        },
        // The day links a read range to its explanation; the page only needs the
        // route to exist so the link resolves to a real URL (S9).
        {
          path: "/explain/:documentId/:from/:to",
          name: "explain",
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

function buttonLabelled(html: string, label: string): string | undefined {
  return buttons(html).find((chunk) => chunk.includes(label))
}

/** The projection's `plan_next` entry, the one the toggle acts on. */
function planEntry(): TodayEntry {
  const entry = TODAY.entries[1]
  if (!entry || entry.kind !== "plan_next") {
    throw new Error("the fixture has no plan_next entry")
  }
  return entry
}

beforeEach(() => {
  api.fetchToday.mockReset()
  api.setReading.mockReset()
  api.fetchToday.mockResolvedValue(PROJECTION)
  api.setReading.mockResolvedValue(STORED_ROW)
})

describe("TodayPage render", () => {
  // S14, first item: the due count with the way into the review session, and only
  // when there is something due — "0 para repasar" is a row that says nothing.
  it("shows the due count with a way into the review session", async () => {
    const html = await render()

    expect(html).toContain("Tenés 2 tarjetas para repasar")
    expect(buttonLabelled(html, "Repasar")).toBeTruthy()
  })

  it("drops the due row entirely when nothing is due", async () => {
    const html = await render({
      today: { dueCount: 0, entries: TODAY.entries },
    })

    expect(html).not.toContain("para repasar")
    expect(buttonLabelled(html, "Repasar")).toBeUndefined()
  })

  it("counts a single due card in the singular", async () => {
    const html = await render({
      today: { dueCount: 1, entries: TODAY.entries },
    })

    expect(html).toContain("Tenés 1 tarjeta para repasar")
  })

  // S14, second and third items: both kinds, each with the material it belongs to
  // and its range; only the plan item carries a note and a toggle.
  it("names both kinds of entry with their material and range", async () => {
    const html = await render()

    expect(html).toContain("DDD")
    expect(html).toContain("páginas 24-36")
    expect(html).toContain("Leíste estas páginas y todavía no las destilaste.")
    expect(html).toContain("Capítulo 5")
    expect(html).toContain("páginas 100-118")
    expect(html).toContain("volver si hace falta")
    expect(buttonLabelled(html, "Leído")).toBeTruthy()
  })

  // S9: the read range is the entry that leads somewhere now, and the URL names the
  // document and the range. The plan item still carries only its toggle.
  it("links a read range to its explanation, and leaves the plan item without a link", async () => {
    const html = await render()

    const links = html.split(/(?=<a )/).filter((chunk) => chunk.startsWith("<a "))
    expect(links).toHaveLength(1)
    expect(links[0]).toContain('href="/explain/doc-1/24/36"')
    expect(links[0]).toContain("Destilá estas páginas")
    expect(buttons(html).filter((button) => button.includes("Leído"))).toHaveLength(1)
  })

  // S13: an unresolved title is `""`, and the document id is what the action needs.
  it("falls back to the document id when the material has no title", async () => {
    const html = await render({
      today: {
        dueCount: 0,
        entries: [{ ...TODAY.entries[0]!, documentTitle: "" }],
      },
    })

    expect(html).toContain("doc-1")
  })

  // D6/D2: a failed load is not an empty day, and it is not a spinner either.
  it("shows the failure and a retry instead of an empty day", async () => {
    const html = await render({
      today: null,
      error: "No se pudo conectar al servidor. ¿Sigue corriendo?",
    })

    expect(html).toContain("No se pudo conectar al servidor. ¿Sigue corriendo?")
    expect(buttonLabelled(html, "Reintentar")).toBeTruthy()
    expect(html).not.toContain("Hoy no tenés nada para hacer.")
  })

  it("says it is loading while the first read is in flight", async () => {
    const html = await render({ today: null, loading: true })

    expect(html).toContain("Cargando...")
    expect(html).not.toContain("Hoy no tenés nada para hacer.")
    expect(buttonLabelled(html, "Reintentar")).toBeUndefined()
  })

  it("says there is nothing to do in one sentence, not an empty list", async () => {
    const html = await render({ today: { dueCount: 0, entries: [] } })

    expect(html).toContain("Hoy no tenés nada para hacer.")
    expect(html).not.toContain("<ul")
    expect(buttonLabelled(html, "Reintentar")).toBeUndefined()
  })

  // S14's toggle, in both directions of its own contract: an accepted write leaves
  // it pressed, a refused one leaves it exactly where it was and says why.
  it("leaves the toggle pressed once the server accepted the read", async () => {
    const html = await render({}, async (store) => {
      await store.setRead(planEntry(), true)
    })

    expect(buttonLabelled(html, "Leído")).toContain('aria-pressed="true"')
    expect(html).toContain("Capítulo 5")
  })

  it("keeps the toggle off and shows the error when the write is refused", async () => {
    api.setReading.mockRejectedValue(new ApiError("document_id is required", 400))

    const html = await render({}, async (store) => {
      await store.setRead(planEntry(), true)
    })

    expect(html).toContain("document_id is required")
    expect(buttonLabelled(html, "Leído")).toContain('aria-pressed="false"')
    expect(html).toContain("Capítulo 5")
  })
})
