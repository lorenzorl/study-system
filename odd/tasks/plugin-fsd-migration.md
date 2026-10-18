# Plugin FSD migration

**Status:** in progress
**Created:** 2026-10-17
**Contract:** `apps/obsidian-plugin/ARCHITECTURE.md` — governing. It holds the
layers, the dependency rule, the slice conventions, the Obsidian boundary, the
file-by-file mapping, and the six migration phases. This document does not
restate it.

## Specs

**S1 — The request.** "Quiero aplicar una arquitectura fsd, me recomiendas
programar el plugin desde cero? y tambien usar la pestana como vista persistente,
que me recomiendas?" Decided: migrate incrementally, do not rewrite; the chat is
a persistent main-area tab (`study-chat`).

**S2 — The contract is the source of truth.** Every move follows
`apps/obsidian-plugin/ARCHITECTURE.md` §5 (target tree), §6 (file-by-file
mapping), §7 (decomposition) and §11 (phases). Where the mapping is silent, the
worker asks rather than inventing a placement.

**S3 — Invariants.** No rewrite of working logic (sync, flashcard parsing,
review/FSRS). No new runtime dependencies. No new devDependencies: enforcement is
the in-repo boundary test. The 94 tests stay green at every phase; temporary
re-export shims at the old paths are deleted in phase 6.

## Tasks

- [x] T1 — Phase 1 Foundation: create `shared/` and `app/`; move the Obsidian
      boundary, the mock, `useSwipe`; extract `shared/api/client.ts`; move
      `main.ts`/`study-view.ts`/`App.vue`/router; update the esbuild entry,
      tsconfig `paths`, and the Vite/Vitest aliases; keep re-export shims.
      (S2, S3)
- [x] T2 — Phase 2 Entities: split `types.ts`/`validation.ts` by entity, move the
      stores, `markdown-parser`, the api segments and the entity cards, and
      colocate the tests. (S2, S3)
- [x] T3 — Phase 3 Features: extract `create-topic`, `create-concept`,
      `sync-note`, `configure-api-base`, `select-topic`, `select-concept`,
      `review-card`. (S2, S3)
- [x] T4 — Phase 4 Widgets + pages: split `DashboardView` and rename the views to
      pages. (S2, S3)
- [ ] T5 — Phase 5 New slices: scaffold `chat`/`document`/`search` and register
      the `study-chat` tab from `app/`. (S2, S3)
- [ ] T6 — Phase 6 Enforcement + cleanup: add the boundary test, delete the shims
      and `src/vue-app/`. (S2, S3)

## Log

**L1 (2026-10-17, request)** — "Quiero aplicar una arquitectura fsd, me
recomiendas programar el plugin desde cero? y tambien usar la pestana como vista
persistente, que me recomiendas?" Then, after Phase 0: "primero haz el comit
despues comienza con la fase 1".

**L2 (2026-10-17, Phase 1 verified and committed)** — implemented by a bounded
writer and re-verified by the parent. `npm test` was **94 passed (3 files)**
before and after; `npm run build` (tsc + esbuild) exited 0; `npx tsc --noEmit
-skipLibCheck` was clean. Moved with `git mv`: `main.ts`, `study-view.ts`,
`App.vue` and `router/` into `app/`; `useObsidian.ts` (with its test) and the
Obsidian mock into `shared/obsidian/`; `useSwipe.ts` into `shared/lib/`. New:
`shared/api/client.ts` holds the base-URL config and the HTTP core, extracted
from `services/api.ts`, which keeps its endpoints and re-exports the client
surface for Phase 2. Configs updated: esbuild entry to `src/app/main.ts`,
tsconfig `paths`, the `@` alias in Vite and Vitest, and the Vitest `obsidian`
alias to the relocated mock. Re-export shims left at
`src/vue-app/composables/{useObsidian,useSwipe}.ts`, the only old paths with
remaining importers. The parent verified the `@/` alias at Vitest runtime with a
throwaway probe test (passed, then deleted), because no existing test exercised
it. No behavior change and no dependency change.

**L3 (2026-10-17, Phase 2 verified and committed)** — implemented by a bounded
writer and re-verified by the parent. `npm test` stayed at **94 passed**, now
across **10 files** instead of 3; `npm run build` exited 0 and `npx tsc --noEmit
-skipLibCheck` was clean. Six entity slices were created — `topic`, `concept`,
`flashcard`, `review`, `metrics`, `resource` — with `types.ts` and
`validation.ts` split per §7c, the endpoints split out of `services/api.ts` per
§7d, and `markdown-parser`, `useReviewStore`, `useMetricsStore` and the four
entity cards moved into their slices. `ConceptCard` now takes `flashcardCount` as
a prop, so `entities/concept` no longer reaches into the flashcard state. The
67-case `types.test.ts` was split into eight colocated suites; six re-export
shims were left at the old paths.

The parent verified the two rules the contract cares about: `grep` over
`src/entities/` finds **no cross-import between entity slices**, and only
`src/app/main.ts`, `src/app/study-view.ts` and `src/shared/obsidian/useObsidian.ts`
import `obsidian`. `study-store.ts` is untouched, as decided: its decomposition is
Phase 3, so there is never a second source of topic state. The contract was
amended in the same commit to move that decomposition from Phase 2 to Phase 3.

**L4 (2026-10-17, Phase 3 verified and committed)** — implemented by a bounded
writer and re-verified by the parent. `npm test` stayed at **94 passed (10
files)**; `npm run build` exited 0, which also compiles every `.vue` reachable
from `src/app/main.ts` through the esbuild Vue plugin, and `npx tsc --noEmit
-skipLibCheck` was clean.

Created the two entity stores that `study-store.ts` owned
(`entities/topic/model/useTopicsStore.ts`, `entities/flashcard/model/useFlashcardCache.ts`)
and the seven feature slices of §5: `create-topic`, `create-concept`,
`sync-note`, `configure-api-base`, `select-topic`, `select-concept`,
`review-card`. `study-store.ts` was **retired with no shim** — its content is
fully distributed — and the six views plus `main.ts`'s `syncActiveNote` command
were rewired to the new stores and features.

The parent verified the rules again: `grep useStudyStore src/` is **empty**;
`grep` finds **no cross-import between feature slices**; and `obsidian` is still
imported by exactly `app/main.ts`, `app/study-view.ts` and
`shared/obsidian/useObsidian.ts`. Because there are no component tests, the
views' correctness rests on `tsc`, the build, and a call-site audit of every old
`useStudyStore` member. One decision the contract did not cover, recorded for
Phase 4/6: `features/create-topic` and `features/create-concept` reach the vault
through a **dynamic** `import("@/shared/obsidian")` so the Obsidian package is
not pulled into the Vite dev graph; moving that side effect to `app/` is a
candidate cleanup.

**L5 (2026-10-17, Phase 3 native review)** — the review of the whole branch was
rejected in preflight with `lens_context_budget_exceeded` (26 commits exceed the
reviewer context budget; no authority was created). Scoped to the single Phase 3
commit (`eba6227`, base `811ad31`): lineage `review-a105c89128afad50`, medium
tier, lenses **readability + reliability**, 30 files / 2056 lines, correction
budget 200. It came back **approved**, and the acknowledgement burned the
authority. Five findings were **non-blocking advisories**, recorded here as later
work — none opened a correction, and none is a reason to re-review this
candidate:

- `R2-duplicate-create-forms` (readability, WARNING):
  `features/create-concept/ui/CreateConceptForm.vue:31-52` duplicates the
  create-topic form.
- `R2-sync-note-type-names` (readability, SUGGESTION):
  `features/sync-note/model/useSyncNote.ts:20-33`.
- `R2-topic-change-invariant` (readability, SUGGESTION):
  `features/select-topic/model/useTopicSelection.ts:20-22`.
- `R3-001` (reliability, WARNING): `app/main.ts:91-96`.
- `R3-002` (reliability, WARNING):
  `features/sync-note/model/useSyncNote.ts:93-152`.

**L6 (2026-10-17, Phase 4 verified and committed)** — implemented by a bounded
writer and re-verified by the parent. `npm test` stayed at **94 passed (10
files)**; `npm run build` exited 0 (it compiles every reachable `.vue`) and `npx
tsc --noEmit -skipLibCheck` was clean.

Four widget slices were created (`metrics-bar`, `daily-review`, `topic-list`,
`concept-list`) and six page slices (`dashboard`, `topic`, `concept`,
`flashcard-session`, `due-cards`, `feynman`). The two remaining components and
the six views moved into them, the topic and concept grids were extracted into
their widgets, the router and `dev/main.ts` were rewired to `@/pages/*`, and the
emptied `src/vue-app/views/` and `src/vue-app/components/` were deleted.

The parent verified the rules: `grep` for `vue-app/views|vue-app/components` is
empty; nothing under `widgets/`, `features/`, `entities/` or `shared/` imports a
page; and `obsidian` is still imported only by `app/` and `shared/obsidian/`.
Since there are no component tests, the moved pages/widgets were checked with
byte-exact diffs against their originals (five are identical) plus `tsc` and the
build for the two extracted grids. The only DOM difference is the grid container
class names, whose scoped CSS moved with the widgets.

**L7 (2026-10-17, Phase 4 native review)** — scoped to the Phase 4 commit
(`0745426`, base `eba6227`): lineage `review-b6a581694996ea63`, medium tier,
lenses **readability + reliability**, 31 files. It came back **approved** and the
acknowledgement burned the authority. Three findings were **non-blocking
advisories**, recorded here as later work:

- `R2-001` (readability, SUGGESTION):
  `widgets/concept-list/ui/ConceptList.vue:7`.
- `R3-001` (reliability, WARNING):
  `widgets/concept-list/ui/ConceptList.vue:1-35`.
- `R3-002` (reliability, SUGGESTION): `widgets/topic-list/ui/TopicList.vue:2`.

None opened a correction. As with L5, the whole-branch candidate was rejected
with `lens_context_budget_exceeded`, so each review is scoped to a single commit.
