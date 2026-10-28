import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import { useSyncNote } from "@/features/sync-note"
import { useTopicsStore } from "@/entities/topic"
import DashboardPage from "./DashboardPage.vue"

// The repo's vitest environment is `node`: no jsdom, no @vue/test-utils. Rendering
// through `vue/server-renderer` is how the states are pinned without a dependency.
// `onMounted` does not run in SSR, so each test seeds the store before rendering —
// that is also how the open panel is reachable at all.

type Store = ReturnType<typeof useSyncNote>

function renderPage(seed: (store: Store) => void = () => {}): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)
  seed(useSyncNote())

  const app = createSSRApp(DashboardPage)
  app.use(pinia)
  app.use(
    createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/", name: "dashboard", component: { template: "<div />" } },
        { path: "/topics", name: "topics", component: { template: "<div />" } },
        { path: "/study/due", name: "due-cards", component: { template: "<div />" } },
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
      ],
    }),
  )

  return renderToString(app)
}

describe("DashboardPage render", () => {
  it("offers the sync action in the topic header, with no panel until it opens", async () => {
    const html = await renderPage()

    expect(html).toContain("Sincronizar nota")
    // The form is an overlay: absent until the action opens it.
    expect(html).not.toContain('role="dialog"')
  })

  it("renders the paste form once the sync action opens it", async () => {
    const html = await renderPage((store) => store.openPanel())

    expect(html).toContain('role="dialog"')
    expect(html).toContain("Tema")
    expect(html).toContain("Concepto")
    expect(html).toContain("<textarea")
  })

  // PRESERVE: the trigger and the overlay were added around the list, not into
  // it, so the topics the page loaded still render as they did.
  it("still lists the topics it loaded", async () => {
    const html = await renderPage(() => {
      useTopicsStore().topics = [{ id: "topic-ddd", name: "DDD", concepts: [] }]
    })

    expect(html).toContain("DDD")
    expect(html).toContain("0 conceptos")
  })

  it("offers the topic action in the section header, by name and by title", async () => {
    const html = await renderPage()

    expect(html).toContain('aria-label="Agregar tema"')
    expect(html).toContain('title="Agregar tema"')
  })

  it("offers a retry when the topics request failed", async () => {
    const html = await renderPage(() => {
      useTopicsStore().error = "No se pudo cargar."
    })

    expect(html).toContain("No se pudo cargar.")
    expect(html).toContain("Reintentar")
  })

  // INVARIANT: the one case the migration is expected to turn red — the header
  // action, the retry and the sync trigger all render through the shared/ui button.
  it("renders its controls through the shared/ui button", async () => {
    for (const html of [
      await renderPage(),
      await renderPage(() => {
        useTopicsStore().error = "No se pudo cargar."
      }),
    ]) {
      const rendered = html.split(/(?=<button)/).filter((chunk) => chunk.startsWith("<button"))
      expect(rendered.length).toBeGreaterThan(0)
      for (const button of rendered) {
        expect(button).toContain('data-slot="button"')
      }
    }
  })
})
