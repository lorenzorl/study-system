/**
 * Fill a cache-once source when it is empty, and leave it alone when something
 * already filled it (`backlog.md` D1).
 *
 * The three pages that read the topics store — `TopicPage`, `ConceptPage`,
 * `FeynmanPage` — all need the same rule on mount: load when the store is empty
 * (a deep link, a reload, a bookmark), do nothing when a warmer path already
 * loaded it (`/topics` → topic card). Writing that rule three times is how the
 * fourth page ends up without it, which is exactly the defect: only the `retry`
 * handlers ever asked for the load.
 *
 * It lives in `shared` and takes the two halves of the decision as arguments —
 * "is it loaded" and "load it" — instead of importing a store. `shared` may not
 * import anything upward (ARCHITECTURE §2), and the same constraint is what makes
 * this testable: the rule is proven with plain functions, no pinia, no DOM.
 *
 * The guard is per caller (create it once in a page's setup, not inside the
 * mount handler), and it holds for as long as a load is in flight:
 *
 * - empty and idle → the loader runs;
 * - populated → nothing;
 * - asked again while the first load is in flight → the same promise, one
 *   request. Two mounts cannot duplicate the fetch;
 * - the load failed → the guard re-arms, because the target is still empty and
 *   the next mount has to try again. "At most once while empty" must not become
 *   "at most once ever", which would leave a page broken after one network blip.
 *
 * The failure is not swallowed: it reaches the caller as a rejected promise, the
 * same way `loadTopics()` already reports one through the store's `error`.
 */
export interface LoadIfEmptyOptions {
  /** True while the caller has nothing to render from. */
  isEmpty: () => boolean
  /** The request that fills it. Called at most once per empty episode. */
  load: () => void | Promise<void>
}

/** The mount handler a caller gets back; safe to call on every mount. */
export type LoadIfEmpty = () => Promise<void>

export function createLoadIfEmpty({
  isEmpty,
  load,
}: LoadIfEmptyOptions): LoadIfEmpty {
  let inFlight: Promise<void> | null = null

  return function loadIfEmpty(): Promise<void> {
    if (inFlight) return inFlight
    if (!isEmpty()) return Promise.resolve()

    const started = (async () => {
      await load()
    })()
    inFlight = started

    // Re-arm once the load settles, whether it succeeded or not, so a failed load
    // does not pin the page to its empty state for the life of the app.
    const settle = () => {
      inFlight = null
    }
    started.then(settle, settle)

    return started
  }
}
