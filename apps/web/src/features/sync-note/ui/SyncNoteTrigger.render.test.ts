import { describe, expect, it } from "vitest"
import { createSSRApp } from "vue"
import { renderToString } from "vue/server-renderer"
import { createPinia, setActivePinia } from "pinia"
import { useSyncNote } from "../model/useSyncNote"
import SyncNoteTrigger from "./SyncNoteTrigger.vue"

// The repo's vitest environment is `node`: no jsdom, no @vue/test-utils. Rendering
// through `vue/server-renderer` pins the trigger's state, without a dependency.
// The overlay is the sync store's, so a test seeds it before render.

type Store = ReturnType<typeof useSyncNote>

function render(seed: (store: Store) => void = () => {}): Promise<string> {
  const pinia = createPinia()
  setActivePinia(pinia)
  seed(useSyncNote())

  const app = createSSRApp(SyncNoteTrigger)
  app.use(pinia)

  return renderToString(app)
}

describe("SyncNoteTrigger render", () => {
  it("is the one action the topics page offers to bring a note in", async () => {
    const html = await render()

    expect(html).toContain("Sincronizar nota")
    expect(html).toContain('aria-label="Sincronizar nota"')
  })

  it("reports whether its panel is open", async () => {
    expect(await render()).toContain('aria-expanded="false"')
    expect(await render((store) => store.openPanel())).toContain(
      'aria-expanded="true"',
    )
  })

  // INVARIANT: the one case the migration is expected to turn red — the trigger is
  // a plain button, so it renders through the shared/ui primitive.
  it("is the shared/ui button", async () => {
    const html = await render()

    expect(html.match(/<button/g)).toHaveLength(1)
    expect(html).toContain('data-slot="button"')
  })
})
