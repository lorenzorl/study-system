import { describe, expect, it, vi } from "vitest"
import { createLoadIfEmpty } from "./loadIfEmpty"

/**
 * The load-if-empty rule (`backlog.md` D1), tested without pinia.
 *
 * The three pages that read the topics store (`TopicPage`, `ConceptPage`,
 * `FeynmanPage`) need one rule: fill the store on mount when it is empty, and do
 * nothing when something already filled it. That rule lives in
 * `shared/lib/loadIfEmpty.ts` with no store import — the caller passes "is it
 * loaded" and "load it" — precisely so it can be pinned here.
 *
 * The integration is not here and cannot be: this repo's vitest environment is
 * `node` and renders through `vue/server-renderer`, where `onMounted` never runs,
 * so no render test can observe a mount-time load. The route captures in
 * `e2e/smoke.spec.ts` are what prove the wiring.
 */

/** A loader whose completion the test decides, so "in flight" is observable. */
function deferredLoader() {
  let settle: () => void = () => {}
  const load = vi.fn(
    () =>
      new Promise<void>((resolve) => {
        settle = resolve
      }),
  )
  return { load, resolve: () => settle() }
}

describe("createLoadIfEmpty", () => {
  it("loads once when the target is empty", async () => {
    let loaded = false
    const load = vi.fn(async () => {
      loaded = true
    })
    const loadIfEmpty = createLoadIfEmpty({ isEmpty: () => !loaded, load })

    await loadIfEmpty()

    expect(load).toHaveBeenCalledTimes(1)
    expect(loaded).toBe(true)
  })

  it("does not load when the target is already populated", async () => {
    const load = vi.fn()
    const loadIfEmpty = createLoadIfEmpty({ isEmpty: () => false, load })

    await loadIfEmpty()

    expect(load).not.toHaveBeenCalled()
  })

  it("does not load twice when two mounts ask while the first load is in flight", async () => {
    let loaded = false
    const { load, resolve } = deferredLoader()
    const loadIfEmpty = createLoadIfEmpty({ isEmpty: () => !loaded, load })

    const firstMount = loadIfEmpty()
    const secondMount = loadIfEmpty()

    expect(load).toHaveBeenCalledTimes(1)

    const settle = resolve
    loaded = true
    settle()
    await Promise.all([firstMount, secondMount])

    expect(load).toHaveBeenCalledTimes(1)
  })

  it("loads again after a failed load, because the target is still empty", async () => {
    const load = vi.fn(async () => {
      throw new Error("network down")
    })
    const loadIfEmpty = createLoadIfEmpty({ isEmpty: () => true, load })

    // The failure reaches the caller, which is where `loadTopics()` already puts
    // it: the store keeps `error` and the page renders its retry.
    await expect(loadIfEmpty()).rejects.toThrow("network down")
    await expect(loadIfEmpty()).rejects.toThrow("network down")

    expect(load).toHaveBeenCalledTimes(2)
  })
})
