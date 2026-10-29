import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import DailyReviewCard from "./DailyReviewCard.vue"

// Characterization test, written before the primitive migration: the card is a
// single control whose whole point is its label, so the label is what is pinned —
// for a count, for one, for none, and while it is still loading. Never how it looks.
//
// The repo's vitest environment is `node` (no jsdom, no @vue/test-utils), so it
// renders through `vue/server-renderer`; the click itself is the visual net's job.

async function render(props: { dueCount: number; loading?: boolean }): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)

  const app = createSSRApp(DailyReviewCard, props)
  app.use(pinia)
  app.use(
    createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/", name: "dashboard", component: { template: "<div />" } },
        { path: "/study/due", name: "due-cards", component: { template: "<div />" } },
      ],
    }),
  )

  const html = await renderToString(app)
  return html.replace(/<!--[\s\S]*?-->/g, "").replace(/\s+/g, " ")
}

describe("DailyReviewCard render", () => {
  it("invites the review, and says how many cards it holds", async () => {
    expect(await render({ dueCount: 3 })).toContain("Estudiar 3 tarjetas hoy")
  })

  it("uses the singular for one card", async () => {
    expect(await render({ dueCount: 1 })).toContain("Estudiar 1 tarjeta hoy")
  })

  it("says there is nothing to review without inviting one", async () => {
    const html = await render({ dueCount: 0 })

    expect(html).toContain("Sin tarjetas para repasar")
    expect(html).not.toContain("Estudiar")
  })

  it("reports that it is still loading", async () => {
    expect(await render({ dueCount: 3, loading: true })).toContain("Cargando...")
  })

  // INVARIANT: the one case the migration is expected to turn red. The whole card
  // stays one control, and it is the shared/ui button.
  it("is a single button, rendered by the shared/ui primitive", async () => {
    const html = await render({ dueCount: 3 })

    expect(html.match(/<button/g)).toHaveLength(1)
    expect(html).toContain('data-slot="button"')
  })
})
