# Plugin FSD migration

**Status:** complete — T1-T6 done; all six phases verified, committed and reviewed.
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
- [x] T5 — Phase 5 New slices: scaffold `chat`/`document`/`search` and register
      the `study-chat` tab from `app/`. (S2, S3)
- [x] T6 — Phase 6 Enforcement + cleanup: add the boundary test, delete the shims
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

**L8 (2026-10-17, Phase 1 and Phase 2 native reviews)** — the two remaining
phases were reviewed after a **temporary detached checkout**, because the
provider builds the candidate as `base-ref .. HEAD`: with HEAD at Phase 4, any
base before Phase 3 pulled Phases 2+3+4 into one candidate and exceeded the
budget. Phase 2 (`811ad31`, base `2f596f4`, lineage `review-8fd560d1e7128d0f`, 59
files) and Phase 1 (`2f596f4`, base `60c9117`, lineage `review-123f2444c64fb802`,
26 files) both came back **approved**, and both acknowledgements burned their
authority. The branch returned to `master` afterwards.

Non-blocking advisories, recorded as later work:

- Phase 2: `R2-metrics-dead-torefs` (readability, SUGGESTION)
  `entities/metrics/model/useMetricsStore.ts:25-31`; `R2-shim-export-surface`
  (readability, WARNING) `src/types.ts:3-8`; `R2-topic-validation-duplication`
  (readability, SUGGESTION) `entities/topic/model/validation.ts:9-13`;
  `R3-001` (reliability, SUGGESTION)
  `entities/review/model/useReviewStore.ts:35-61`.
- Phase 1: `R3-001` (reliability, WARNING)
  `shared/obsidian/useObsidian.ts:40-42`; `R3-002` (reliability, SUGGESTION)
  `vitest.config.ts:11`.

All four FSD phases are now reviewed and approved. Fourteen advisories were
raised across the four reviews; none is blocking, and none reopened a candidate.

**L9 (2026-10-17, Phase 5 verified and committed)** — the nine slices §8
reserves now exist, plus the persistent chat tab. Measured from
`apps/obsidian-plugin`: `npm test` = **94 passed (10 files)**, unchanged, because
Phase 5 adds no test; `npm run build` exited **0** and the bundle contains the
chat view (`grep -c study-chat main.js` = 4), so every new `.vue` is reachable
from `src/app/main.ts` and gets compiled; `npx tsc --noEmit -skipLibCheck` exited
**0**. `grep -rln 'from "obsidian"' src/` lists exactly `app/main.ts`,
`app/study-view.ts`, `app/study-chat-view.ts` and
`shared/obsidian/useObsidian.ts`. `grep -rnE 'from "@/pages/' src/widgets
src/features src/entities src/shared` is **empty**, and `grep -rn "vue-app/"
src/ dev/` is **empty** (the Phase 2/3 shim *files* still exist; nothing imports
through them). 25 files added, 2 source files and 3 docs changed, 938 added source
lines.

What was built: `entities/chat-message` (model only), `entities/document` and
`entities/search-result` (model + `api/`), `features/ask-chat`,
`features/upload-document`, `features/search-corpus`, `widgets/chat-panel`,
`widgets/search-results`, `pages/chat`, and `app/study-chat-view.ts` — an
`ItemView` with view type `study-chat` that mounts `pages/chat` and is opened with
`workspace.getLeaf("tab")` from a new `Open Study Chat` command, mirroring
`StudyView`. No dependency was added and no existing behavior changed.

Two deviations from the reserved tree, both written into the contract in this
commit (§5, §8): the new `document` and `search-result` entities carry an `api/`
segment because those two endpoints already exist and every other entity keeps
its server calls there (§3/§7d); `chat-message` does not, because its endpoint
does not exist. `shared/api/client.ts` gained `requestMultipart` (FormData, no
Content-Type header, so the browser writes the boundary) with the old
error-handling block extracted verbatim into a private `parseJsonResponse` that
`request` now calls. That extraction touches code every entity API shares and no
committed test covers it (§ High risk item 6), so its behavior was probed with a
throwaway vitest file: 5 cases — JSON Content-Type sent, body parsed, `ApiError`
carrying the body's `error` and status, the default message when the body has no
`error`, and a multipart call with the FormData body and no Content-Type header.
All 5 passed and the probe was deleted, so the committed suite stays at 94.

`features/ask-chat/README.md` documents the chat contract (`POST /api/chat`,
`{ question }` → `{ answer, citations? }`) and states that no server implements
it: `useAskChat.ask` appends the question and reports the gap instead of calling
the network. `pages/chat` composes `widgets/chat-panel` — the chat UI the phase
specifies — plus a corpus section (`upload-document`, `search-corpus`,
`widgets/search-results`), which is what gives the three new corpus slices a
consumer instead of leaving them as unreferenced files. The dashboard tab, the
sync flow, flashcard parsing, FSRS, due cards and Feynman are untouched.

This phase is **one commit** despite ~938 added lines: only the entry-reachable
graph is compiled by esbuild, so splitting the slices away from the `app/` view
would leave a commit whose new SFCs are never compiled — a worse trade than one
larger scaffolding candidate. One pre-existing cast in `app/main.ts`
(`setObsidianApp(null as unknown as App)`) gained a `SAFETY:` comment because the
repository's lens rejects undocumented casts; the line and its runtime behavior
are unchanged, and widening `setObsidianApp` to `App | null` stays a candidate
cleanup.

**L10 (2026-10-17, Phase 5 native review)** — scoped to the Phase 5 commit
(`2f4d666`, base `4818ea8`) by passing `baseRef` + `committedOnly` to START:
`inspect` alone resolves the candidate against the merge-base with
`origin/master` (`6263269`, the whole 30-commit branch), which is the
`lens_context_budget_exceeded` shape; the explicit base froze the candidate to the
29 files of this phase. Lineage `review-4138ec58a1e55bf9`, medium tier, lenses
**risk + readability + reliability**, 29 files / 1014 lines, correction budget
200. Three reviewer runs (`pi_host_relay`), all approved on the last admitted
event; the acknowledgement burned the authority.

Eight findings, all **non-blocking advisories** — none opened a correction, none
reopens this review:

- `R2-001` (readability, WARNING):
  `features/upload-document/ui/UploadDocumentForm.vue:40-48`.
- `R2-002` (readability, WARNING): `shared/api/client.ts:73-74`.
- `R2-003` (readability, WARNING): `shared/api/client.ts:81`.
- `R2-004` (readability, SUGGESTION):
  `entities/document/model/types.ts:3-4`.
- `R3-parse-refactor-untested` (reliability, WARNING):
  `shared/api/client.ts:64-82` — the `parseJsonResponse` extraction has no
  committed test. This is the same gap L9 flagged by hand, so the throwaway probe
  remains the only evidence; promoting it to a permanent suite is the obvious
  follow-up.
- `R3-render-arithmetic-on-scores` (reliability, SUGGESTION):
  `widgets/search-results/ui/SearchResults.vue:20-23`.
- `R3-search-race-no-sequencing` (reliability, SUGGESTION):
  `features/search-corpus/model/useSearchCorpus.ts:19-24` — two overlapping
  searches can land out of order.
- `R3-shared-sync-default-message` (reliability, WARNING):
  `shared/api/client.ts:88` — the shared default message still says "al
  sincronizar" for every endpoint; pre-existing, now reachable from search and
  upload too.

With Phase 5 approved, twenty-three advisories are open across the five reviews;
none is blocking and none reopened a candidate. T6 (boundary test plus shim and
`src/vue-app/` cleanup) is the only task left in the contract.

**L11 (2026-10-17, Phase 6 verified and committed)** — the enforcement guard and
the cleanup. `npm test` is now **101 passed (11 files)**: the 94 are untouched and
the guard adds 7. `npm run build` exited 0 and `npx tsc --noEmit -skipLibCheck`
exited 0.

`src/architecture.test.ts` walks every `.ts`/`.vue` under `src/`, extracts every
module specifier (`from "…"`, dynamic `import("…")` and side-effect `import "…"`),
and reports violations per rule: upward imports, same-layer siblings, alias
imports that miss a slice root, relative imports that leave their own slice, the
`obsidian` boundary, and files outside the layers that import into them. A seventh
case asserts the walk found the expected files, so the guard cannot pass vacuously.
No dependency was added: the ESLint option stays unused, as decided in §10.

The guard was proven to fire, not merely to be green. Before the cleanup it went
**RED with 24 `unlayered-module` violations**, one per re-export in the eight
shims — the exact set this phase deletes. Each remaining rule was then proven with
a throwaway, otherwise type-valid probe file
(`src/features/__boundary-probe.ts`, one bad import per rule): **5 failed, 2
passed**, one violation per rule and no false positive. The probe was deleted
before the final run, so nothing of it is committed.

Deleted: `src/types.ts`, `src/validation.ts` and the whole `src/vue-app/` tree
(six files, six directories), plus the empty `src/__mocks__` left behind by
Phase 1 — eight files. Nothing referenced any of them: checked with `grep` before
deleting, and `tsc`, the build and the suite confirm it after.
`grep -rn "vue-app/" src/ dev/` is empty; `grep -rnE 'from "@/pages/'
src/widgets src/features src/entities src/shared` is empty; `grep -rln 'from
"obsidian"' src/` still lists only `app/main.ts`, `app/study-view.ts`,
`app/study-chat-view.ts` and `shared/obsidian/useObsidian.ts`. The `src/` top level
is now exactly the §5 target plus the guard: `app/ entities/ features/ pages/
shared/ widgets/ architecture.test.ts shims-vue.d.ts`.

§10 was amended in the same commit to state the implemented rule set, because a
contract that describes its guard loosely invites a weaker one later. With this,
all six phases of the migration are done and the shims no longer exist.

**L12 (2026-10-17, Phase 6 native review)** — scoped to the Phase 6 commit
(`4ce14ed`, base `2b54ca3`) with `baseRef` + `committedOnly`; the eligible host
resolved the consent for this target itself, so no relay was needed. Lineage
`review-a95cf7f9a366fada`, medium tier, lenses **readability + reliability**, 11
files / 334 lines, correction budget 167. Two reviewer runs (`pi_host_relay`), both
approved on the last admitted event; the acknowledgement burned the authority.

Six findings, all **non-blocking advisories** — none opened a correction:

- `R2-001` (readability, WARNING) and `R3-1` (reliability, WARNING):
  `src/architecture.test.ts:73-89` — both point at `importsOf`, the three regexes
  that extract specifiers. The extraction is textual: it does not understand
  `require(`, and a `from "…"` inside a comment or a string would read as an
  import. Neither occurs in `src/` today, and a false positive fails loudly, so
  the risk to keep in mind is the false negative when an unusual import form
  appears.
- `R2-002` (readability, SUGGESTION): `src/architecture.test.ts:115` — the
  `file.split("/") as [Layer | undefined]` cast that recovers `ownLayer`.
- `R3-2` (reliability, WARNING): `src/architecture.test.ts:120-133` — the alias
  branch derives the target unit by destructuring `parts`, while `ownLayer` comes
  from that cast, so the two sides are not derived the same way.
- `R3-3` (reliability, SUGGESTION): `src/architecture.test.ts:199-200` — the scan
  runs once at module load, so the suite reports the tree as it was at collection
  time.
- `R2-003` (readability, WARNING): `odd/tasks/plugin-fsd-migration.md:288-290`.

With all six phases approved, **twenty-eight** advisories are open across the six
reviews (14 from Phases 1-4, 8 from Phase 5, 6 from Phase 6). L10 said
"twenty-three" after Phase 5, which was one too many: 14 + 8 was 22. None is
blocking, none reopened a candidate, and the migration contract now has no open
task.
