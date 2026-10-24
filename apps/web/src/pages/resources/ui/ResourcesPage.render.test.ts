import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import { useResourcesStore } from "@/entities/resource"
import type { Resource } from "@/entities/resource"
import ResourcesPage from "./ResourcesPage.vue"

// The repo's vitest environment is `node`: no jsdom, no @vue/test-utils. Rendering
// through `vue/server-renderer` is how the states are pinned without a dependency.
// `onMounted` does not run in SSR, so each test seeds the store before rendering:
// that is also how the loading and error branches are reachable at all.

const DAY = 86_400_000

type Store = ReturnType<typeof useResourcesStore>

function renderPage(seed: (store: Store) => void): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)
  seed(useResourcesStore())

  const app = createSSRApp(ResourcesPage)
  app.use(pinia)
  app.use(
    createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/", name: "dashboard", component: { template: "<div />" } },
        {
          path: "/present/:deckId",
          name: "presentation",
          component: { template: "<div />" },
        },
      ],
    }),
  )

  return renderToString(app)
}

function material(overrides: Partial<Resource> = {}): Resource {
  return {
    id: "r-clean-arch",
    title: "Clean Architecture",
    type: "book",
    sourceUri: "obsidian://clean-arch.md",
    topicTitle: "DDD",
    // Three days and one hour: a floor that lands on 3 whatever second this runs.
    createdAt: new Date(Date.now() - 3 * DAY - 3_600_000).toISOString(),
    ...overrides,
  }
}

/** The card's primary action element, whatever state it is in. */
function actionButton(html: string): string {
  const match = html.match(/<button[^>]*resource-card__action[^>]*>/)
  expect(match).not.toBeNull()

  return match?.[0] ?? ""
}

describe("ResourcesPage render", () => {
  it("offers the capture action in the filter row, with no panel until it opens", async () => {
    const html = await renderPage(() => {})

    expect(html).toContain("Agregar material")
    // The capture form is an overlay: absent until the action opens it.
    expect(html).not.toContain('role="dialog"')
  })

  it("disables the action for a material that lives in the corpus", async () => {
    const html = await renderPage((store) => {
      store.resources = [material({ sourceUri: "rag://documents/doc-1" })]
    })

    const action = actionButton(html)

    expect(action).toContain("disabled")
    expect(action).toContain("Vive en el corpus: buscalo por su título.")
    expect(html).toContain("Abrir")
  })

  it("keeps the action enabled for a material that is a page on the web", async () => {
    const html = await renderPage((store) => {
      store.resources = [material({ sourceUri: "https://example.com/paper" })]
    })

    const action = actionButton(html)

    expect(action).not.toContain("disabled")
    expect(action).not.toContain("title=")
  })

  it("renders a stored resource as a material card with its type and age", async () => {
    const html = await renderPage((store) => {
      store.resources = [material()]
    })

    expect(html).toContain("Clean Architecture")
    expect(html).toContain("DDD ›")
    expect(html).toContain("Libro · agregado hace 3 días")
    expect(html).toContain("Abrir")
    expect(html).toContain("Material")
    expect(html).toContain("1 recurso")
  })

  it("offers the material filter chip even before anything arrives", async () => {
    const html = await renderPage(() => {})

    expect(html).toContain("Material")
    expect(html).toContain("0 recursos")
  })

  it("shows the empty state when the API returned nothing", async () => {
    const html = await renderPage((store) => {
      store.resources = []
    })

    expect(html).toContain("Todavía no hay recursos.")
    expect(html).not.toContain("No hay recursos que coincidan con el filtro.")
  })

  it("shows the failure message and a retry instead of a grid", async () => {
    const html = await renderPage((store) => {
      store.error = "No se pudo conectar al servidor."
    })

    expect(html).toContain("No se pudo conectar al servidor.")
    expect(html).toContain("Reintentar")
    expect(html).not.toContain("Todavía no hay recursos.")
  })

  it("shows the loading state while the first read is in flight", async () => {
    const html = await renderPage((store) => {
      store.loading = true
    })

    expect(html).toContain("Cargando")
    expect(html).not.toContain("0 recursos")
  })
})
