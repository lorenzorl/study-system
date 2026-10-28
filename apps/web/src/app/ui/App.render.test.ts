import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createMemoryHistory, createRouter } from "vue-router"
import App from "./App.vue"

// The shell is the only place that decides where the reader can go, so it is
// pinned the same way the pages are: rendered through `vue/server-renderer`,
// because the repo's vitest environment is `node` (no jsdom).

const stub = { template: "<div />" }

async function renderAt(path: string): Promise<string> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", name: "dashboard", component: stub },
      { path: "/topics", name: "topics", component: stub },
      { path: "/present/:deckId", name: "presentation", component: stub },
    ],
  })

  await router.push(path)
  await router.isReady()

  const app = createSSRApp(App)
  app.use(router)

  return renderToString(app)
}

/**
 * The href of the one link carrying the active class, or `undefined`.
 *
 * Read from the rendered markup instead of the component's state: the test is
 * about what a reader sees, and the attribute order RouterLink emits is not
 * something this suite should freeze.
 *
 * `aria-current="page"` rather than the styling class: that is what a screen
 * reader announces, and it survives a change of CSS approach. The class that
 * paints the underline is an implementation detail of the nav.
 */
function activeHref(html: string): string | undefined {
  for (const match of html.matchAll(/<a\s([^>]*)>/g)) {
    const attributes = match[1]
    if (/aria-current="page"/.test(attributes)) {
      return /href="([^"]*)"/.exec(attributes)?.[1]
    }
  }

  return undefined
}

describe("app shell", () => {
  it("names the app and offers both top-level destinations", async () => {
    const html = await renderAt("/")

    expect(html).toContain("Estudio")
    expect(html).toContain('href="/"')
    expect(html).toContain('href="/topics"')
    expect(html).toContain("Recursos")
    expect(html).toContain("Temas")
  })

  it("marks the current section, and only the current one", async () => {
    const resources = await renderAt("/")
    expect(activeHref(resources)).toBe("/")
    expect(resources.match(/aria-current="page"/g)).toHaveLength(1)

    const topics = await renderAt("/topics")
    expect(activeHref(topics)).toBe("/topics")
    expect(topics.match(/aria-current="page"/g)).toHaveLength(1)
  })

  it("offers a way back only where the trail is not obvious", async () => {
    expect(await renderAt("/")).not.toContain("Volver")
    expect(await renderAt("/topics")).not.toContain("Volver")
    expect(await renderAt("/present/deck-atencion")).toContain("Volver")
  })

  // INVARIANT: the one case the migration is expected to turn red. The way back is
  // a plain button, so it renders through the shared/ui primitive.
  it("renders the way back through the shared/ui button", async () => {
    expect(await renderAt("/present/deck-atencion")).toContain('data-slot="button"')
  })
})
