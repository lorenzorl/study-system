import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import PresentationPage from "./PresentationPage.vue"

// Characterization test, written before the primitive migration: the page is a
// full-bleed deck with exactly one host-drawn control, the way out, and this pins
// that it exists, is named for a screen reader and is a button. The deck itself is
// reveal.js's shadow root and is out of scope (`shadcn-migration.md`, scope note);
// it mounts in `onMounted`, so SSR renders only its empty host.

async function render(): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", name: "dashboard", component: { template: "<div />" } },
      {
        path: "/present/:deckId",
        name: "presentation",
        component: { template: "<div />" },
      },
    ],
  })
  await router.push("/present/deck-atencion")
  await router.isReady()

  const app = createSSRApp(PresentationPage)
  app.use(pinia)
  app.use(router)

  const html = await renderToString(app)
  return html.replace(/<!--[\s\S]*?-->/g, "").replace(/\s+/g, " ")
}

describe("PresentationPage render", () => {
  it("offers one way out of the presentation, named for a screen reader", async () => {
    const html = await render()

    expect(html).toContain("Salir")
    expect(html).toContain('aria-label="Salir de la presentación"')
    expect(html.match(/<button/g)).toHaveLength(1)
  })

  // INVARIANT: the one case the migration is expected to turn red.
  it("renders that control through the shared/ui button", async () => {
    expect(await render()).toContain('data-slot="button"')
  })
})
