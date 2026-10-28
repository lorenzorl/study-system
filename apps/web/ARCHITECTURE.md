# Web App — Architecture Contract

## 1. Status and scope

This is the **governing Feature-Sliced Design (FSD) contract for the web app** at `apps/web`. The layers, the dependency rule, the slice and segment conventions, and the platform boundary in §2–§4 are the rules the suite enforces today; `src/architecture.test.ts` is the guard. §6, §7 and §11 keep the file-by-file mapping and the phase plan of the migration that produced this app: they were written before any file moved, while the SPA still lived in `apps/obsidian-plugin`, and they are history.

Current shape: a Vue 3 + Pinia + Vue Router SPA under `src/`, sliced by the layers below and served by the Go binary at `/`. There is still **no ESLint or boundary linter** in the repository; the boundary test in §10 is what enforces the dependency rule.

**Scope reduced (2026-10-20, historical): the plugin became flashcards-only.**
The chat, the LLM configuration and the document corpus were removed, together
with the slices listed in §14. The launcher later returned `entities/resource`
(§15) and the web app captures documents into the corpus from
`features/capture-material`, so that reduction describes the plugin, not the
app. The layers, the dependency rule, the segment conventions and the platform
boundary in §2–§4 are unchanged and still governing; §5, §7b and §8 record the
reduced tree. §6, §7 and §11 keep the original phase-0 mapping and plan as
history: they describe where the pre-migration files landed and how the
migration ran, not what exists today.

## 2. Layers and the dependency rule

FSD layers used here (there is **no `processes` layer** — it is deprecated and is not used):

| Layer | Role | Platform-aware? |
| --- | --- | --- |
| `app` | Integration/bootstrap: entry point, router, root component, app shell. | No |
| `pages` | Route-level screens. | No |
| `widgets` | Self-contained compositional UI blocks used by pages. | No |
| `features` | User interactions that bring value (create, sync, submit, select). | No |
| `entities` | Domain models, their stores, api segments and cards. | No |
| `shared` | Reusable, domain-free infrastructure (`api` client, `lib`, `config`) and the `ui` primitives. | No |

**The dependency rule** — imports may only point downward:

```text
app → pages → widgets → features → entities → shared
```

- A lower layer never imports a higher layer.
- **No cross-imports between slices of the same layer** (e.g. `entities/topic` must not import `entities/concept`; `features/create-topic` must not import `features/create-concept`).
- A slice may import multiple lower-layer slices, but only through their public API (`index.ts`).

No layer imports a platform SDK: §4 is the rule that says so, and why it is the
one boundary the app still needs.

## 3. Slice and segment conventions

- **Slice**: a folder in a layer named in **kebab-case** (`entities/topic`, `features/create-topic`). Each slice is a single unit of the layer's meaning.
- **Public API**: every slice exposes exactly one entry point, `index.ts`. Only what that file re-exports is public.
- **Segments** (internal folders of a slice): `ui/` (components), `model/` (state, stores, types, validation), `api/` (server calls), `lib/` (pure helpers), `config/` (constants/configuration).
- **Import rule**: a slice may only be imported through its `index.ts`. Deep imports into a slice's segment (`entities/topic/model/types`) are forbidden.
- **Tests are colocated** with the slice they test (`entities/review/model/useReviewStore.test.ts`), except ambient/infra tests which live next to the file under test.
- **`shared` has segments, not slices.** Its unit is the segment itself
  (`shared/api`, `shared/lib`, `shared/config`, `shared/ui`), so one segment may
  reach another by its public path (`@/shared/lib/utils`) — the same shape every
  other layer uses to reach a lower one. The same-layer-sibling rule is about
  *slices* and does not apply here (§10).

### 3.1 Styling: the token bridge and `shared/ui`

`shared/ui/` holds the shadcn-vue primitives (`button`, `input`, `label`, `card`,
`dialog`). They are domain-free infrastructure, importable by any layer, and each
is reached by its folder: `@/shared/ui/button`. Nothing else in the app renders
them yet — the migration of existing pages is a separate, later decision.

**`index.html` still owns the palette.** Its `:root` block declares
`--text-normal`, `--background-primary`, `--interactive-accent`, … and remains
the only place a colour value is written. shadcn's tokens (`--background`,
`--foreground`, `--primary`, `--ring`, …) are declared in `src/app/styles.css` as
**references** onto those variables, never as new values, so a primitive using
`bg-background` and a page using `var(--background-primary)` cannot disagree, and
the `prefers-color-scheme` block in `index.html` keeps serving both without a
second source of truth.

`src/app/styles.css` is the Tailwind v4 entry **and** the config: there is no
`tailwind.config.js` and no `postcss.config.js`. It imports
`tailwindcss/theme.css` and `tailwindcss/utilities.css` but **not
`tailwindcss` itself**, because the plugin's own entry also pulls in preflight.
Preflight would flatten `h1`-`h6` to `font-size: inherit` and strip list markers,
re-styling the 24 pre-existing components whose `<style>` blocks never re-declare
those; `index.html` already owns the reset. Verified on the built bundle: no
universal-selector rule is emitted.

To add a primitive:

```bash
pnpm dlx shadcn-vue@2.8.2 add <name>   # components.json sends it to src/shared/ui
```

**Defect to guard against:** the generator writes `import { cn } from "cn"` — the
unrelated `cn` package on npm, not `aliases.utils` — and adds it as a dependency.
After every `add`, rewrite the import and never `pnpm add cn`:

```bash
grep -rl 'from "cn"' src/shared/ui | xargs sed -i '' 's|from "cn"|from "@/shared/lib/utils"|g'
```

### 3.2 The visual safety net

`e2e/smoke.spec.ts` renders the eight routes against the **production bundle**
(`vite preview`) and compares one full-page screenshot per route. It exists
because nothing else here can see a colour: the unit suite runs `renderToString`
in a `node` environment and asserts text, so a migrated control could lose its
border and every test would still pass.

`/api/**` is stubbed through `page.route` from fixtures that mirror the entity
wire types (`ResourceResponse`, `TopicResponse`/`ConceptDTO`, `DueCard`), so every
page renders its populated state and the suite needs no Go binary. A screenshot
of an error state is not a regression test.

Baselines live in `e2e/__screenshots__/` and are **committed**; the report and the
run artifacts (`playwright-report/`, `test-results/`) are not. The path template
carries `{platform}`, because font rasterisation differs between macOS and Linux
and a baseline from one would fail on the other for reasons unrelated to the code.
The Linux set has to be generated once on Linux and committed — run the suite in
the CI container with `--update-snapshots` and commit what it produces.

Until that one-time step, **CI runs the smoke with `--ignore-snapshots`**. It still
proves every route renders with no console error; it does not compare pixels it has
no baseline for. The first version of this wired the plain command into CI, which
turned the web job from green to deterministically red on every run: a gate that
always fails gates nothing. A native review caught it (`R3-ci-missing-linux-baselines`)
after it had passed 200 unit tests and ten green local screenshots.

Accepting a visual change is deliberate: `pnpm exec playwright test
--update-snapshots`, then read the PNG diff. A baseline updated without looking is
a regression captured as the new truth.

## 4. The platform boundary

The app imports **no platform or SDK package at all**. It runs in the browser and
the Go binary serves it, so there is no host framework to adapt to and nothing to
port. The boundary that used to be a rule about where `obsidian` may be imported
is now a rule that it is imported nowhere, and that is stronger: it states the
requirement — the app has no host — instead of abstracting one that does not
exist.

`src/architecture.test.ts` enforces it. An import of `"obsidian"` from any file
fails the `obsidian-boundary` rule, and `unlayered-module` fails when a file
outside the layers (an ambient declaration, the guard itself) imports into them.
`pages`, `widgets`, `features`, `entities` and `shared` therefore stay testable
under vitest with no SDK mock.

No `shared/platform` port replaces the old boundary. With a single platform a
port would be a fiction, and the guard is cheaper than one. A second platform is
the moment to revisit that, not before.

**Historical.** While the UI lived inside the Obsidian plugin, the `obsidian`
package could be imported by only two places: `src/app/` (the plugin entry and
its `ItemView`) and `src/shared/obsidian/` (the single boundary module).
Everything else had to be testable without the mock. The import sites, the mock
and the vitest alias were deleted with the plugin; only the rule survived, in the
stronger form above.

## 5. Target tree (historical)

This is the tree the extraction was executed against. It is kept because §6, §7
and §11 refer to the paths below, not as a description of the app: the plugin
directory is gone, and the app has since gained the slices recorded in §15 and in
`PRESENTATION-UI.md` §9. `+` marks a slice that did not exist when the contract
was written (see §8). Segments are shown where the slice is non-trivial.

```text
apps/obsidian-plugin/
├── ARCHITECTURE.md
├── esbuild.config.mjs                 # entry updated to src/app/main.ts
├── esbuild-vue-plugin.mjs             # unchanged
├── package.json                       # unchanged
├── manifest.json                      # unchanged
├── vite.config.ts                     # + @/ resolve alias
├── vitest.config.ts                   # obsidian alias updated, + @/ resolve alias
├── tsconfig.json                      # + paths["@/*"], include unchanged
├── dev/                               # dev-only harness (outside FSD layers)
│   ├── index.html                     # unchanged
│   └── main.ts                        # imports updated to @/
└── src/
    ├── shims-vue.d.ts                 # ambient *.vue declaration (outside layers)
    ├── app/                           # Obsidian integration layer
    │   ├── main.ts
    │   ├── study-view.ts
    │   ├── settings-store.ts          # + settings persistence over the plugin's data.json
    │   ├── settings-tab.ts            # + PluginSettingTab with the backend base URL
    │   ├── router/index.ts
    │   └── ui/App.vue
    ├── pages/
    │   ├── dashboard/{index.ts,ui/DashboardPage.vue}
    │   ├── topic/{index.ts,ui/TopicPage.vue}
    │   ├── concept/{index.ts,ui/ConceptPage.vue}
    │   ├── flashcard-session/{index.ts,ui/FlashcardSessionPage.vue}
    │   ├── due-cards/{index.ts,ui/DueCardsPage.vue}
    │   └── feynman/{index.ts,ui/FeynmanPage.vue}
    ├── widgets/
    │   ├── metrics-bar/{index.ts,ui/MetricsBar.vue}
    │   ├── daily-review/{index.ts,ui/DailyReviewCard.vue}
    │   ├── topic-list/{index.ts,ui/TopicList.vue}
    │   └── concept-list/{index.ts,ui/ConceptList.vue}
    ├── features/
    │   ├── create-topic/{index.ts,model/useCreateTopic.ts,ui/CreateTopicForm.vue}
    │   ├── create-concept/{index.ts,model/useCreateConcept.ts,ui/CreateConceptForm.vue}
    │   ├── sync-note/{index.ts,model/useSyncNote.ts,ui/SyncNoteButton.vue}
    │   ├── select-topic/{index.ts,model/useTopicSelection.ts}
    │   ├── select-concept/{index.ts,model/useConceptSelection.ts}
    │   └── review-card/{index.ts,model/useReviewSession.ts}
    ├── entities/
    │   ├── topic/{index.ts,model/{types.ts,validation.ts,useTopicsStore.ts},api/topic-api.ts,ui/TopicCard.vue}
    │   ├── concept/{index.ts,model/{types.ts,validation.ts},api/concept-api.ts,ui/ConceptCard.vue}
    │   ├── flashcard/{index.ts,model/{types.ts,validation.ts,useFlashcardCache.ts},api/flashcard-api.ts,lib/markdown-parser.ts,ui/FlashcardCard.vue}
    │   ├── review/{index.ts,model/{types.ts,validation.ts,useReviewStore.ts},api/review-api.ts,ui/DueCardItem.vue}
    │   └── metrics/{index.ts,model/{types.ts,validation.ts,useMetricsStore.ts}}
    └── shared/
        ├── api/client.ts
        ├── config/{index.ts,settings.ts,settings.test.ts}    # + global settings state and its persistence port
        ├── lib/{guards.ts,useSwipe.ts}
        └── obsidian/{index.ts,useObsidian.ts,useObsidian.test.ts,__mocks__/obsidian.ts}
```

> **Retired after this contract.** `features/configure-api-base` was replaced by
> `features/configure-llm`: the global settings are edited from the plugin's own
> `PluginSettingTab` and persisted in its `data.json`, and the API base URL moved
> out of `localStorage` into that same store (`shared/config`). The chat slices
> changed shape too: `features/ask-chat`, `widgets/chat-panel` and
> `entities/chat-message` were replaced by `features/send-message`,
> `widgets/chat-thread` and `entities/chat`, because a chat is a note and the
> prompt it uses belongs to that note. The rows below that name the old slices
> record where their work landed.

## 6. File-by-file mapping (historical)

One row per file as it stood before the migration. `Layer / slice` is the parent slice in the target tree. Files that cannot be assigned to a layer are flagged explicitly.

### 6a. `src/` — hand-written source and tests (30 files)

| Current path | Target path | Layer / slice | Reason |
| --- | --- | --- | --- |
| `src/main.ts` | `src/app/main.ts` | app | Plugin bootstrap + commands; imports `obsidian`. Integration belongs to `app`. |
| `src/study-view.ts` | `src/app/study-view.ts` | app | `ItemView` that mounts the Vue app; imports `obsidian`. |
| `src/vue-app/App.vue` | `src/app/ui/App.vue` | app | Root layout holding `<router-view>`; only app-level chrome. |
| `src/vue-app/router/index.ts` | `src/app/router/index.ts` | app | Router imports every page; a cross-cutting app concern. |
| `src/vue-app/composables/useObsidian.ts` | `src/shared/obsidian/useObsidian.ts` | shared / obsidian | The sole Obsidian boundary; must be reachable by app only and cannot live in a higher layer. |
| `src/__mocks__/obsidian.ts` | `src/shared/obsidian/__mocks__/obsidian.ts` | shared / obsidian | Test stub for the obsidian package; paired with the boundary. **Decided** (see §13). |
| `src/vue-app/composables/__tests__/useObsidian.test.ts` | `src/shared/obsidian/useObsidian.test.ts` | shared / obsidian | Tests the boundary; colocated with it. |
| `src/vue-app/composables/useSwipe.ts` | `src/shared/lib/useSwipe.ts` | shared / lib | Generic touch composable, no domain and no obsidian. |
| `src/vue-app/services/api.ts` | split → `src/shared/api/client.ts` + `entities/*/api/*` | shared / api + entities | HTTP core is shared; endpoint calls are per-entity (§7d). |
| `src/vue-app/services/markdown-parser.ts` | `src/entities/flashcard/lib/markdown-parser.ts` | entities / flashcard | Parses `question::answer` into flashcards; flashcard domain logic. |
| `src/vue-app/stores/study-store.ts` | split → `entities/*/model/*` + `features/*/model/*` | entities + features | Mixes topic/concept/flashcard state with sync and create actions (§7a). |
| `src/vue-app/stores/review-store.ts` | `src/entities/review/model/useReviewStore.ts` | entities / review | FSRS due-card session state; domain state, no other entity. |
| `src/vue-app/stores/metrics-store.ts` | `src/entities/metrics/model/useMetricsStore.ts` | entities / metrics | Study metrics state; currently a hardcoded mock (§7f). |
| `src/types.ts` | split by entity + shared | entities + shared | Domain types are per-entity; no single home (§7c). |
| `src/validation.ts` | split by entity + `shared/lib` | entities + shared | Validators are per-entity; generic guards are shared (§7c). |
| `src/vue-app/components/TopicCard.vue` | `src/entities/topic/ui/TopicCard.vue` | entities / topic | Renders a `Topic` + concept count; pure entity card. |
| `src/vue-app/components/ConceptCard.vue` | `src/entities/concept/ui/ConceptCard.vue` | entities / concept | Renders a `ConceptSummary`; count becomes a prop (§7e). |
| `src/vue-app/components/FlashcardCard.vue` | `src/entities/flashcard/ui/FlashcardCard.vue` | entities / flashcard | Front/back flip card for a flashcard. |
| `src/vue-app/components/DueCardItem.vue` | `src/entities/review/ui/DueCardItem.vue` | entities / review | Renders a `DueCard` row. |
| `src/vue-app/components/MetricsBar.vue` | `src/widgets/metrics-bar/ui/MetricsBar.vue` | widgets / metrics-bar | Composite block reading the metrics store; wider than one entity card. |
| `src/vue-app/components/DailyReviewCard.vue` | `src/widgets/daily-review/ui/DailyReviewCard.vue` | widgets / daily-review | Due-count CTA block with navigation; not a single entity render. |
| `src/vue-app/views/DashboardView.vue` | split → `pages/dashboard/`, `widgets/*`, `features/*` | pages + widgets + features | Mixes metrics, grid, create, sync and API config (§7b). |
| `src/vue-app/views/TopicView.vue` | `src/pages/topic/ui/TopicPage.vue` | pages / topic | Route screen for a topic; optional concept-list extraction (§7b note). |
| `src/vue-app/views/ConceptView.vue` | `src/pages/concept/ui/ConceptPage.vue` | pages / concept | Route screen for a concept. |
| `src/vue-app/views/FlashcardSession.vue` | `src/pages/flashcard-session/ui/FlashcardSessionPage.vue` | pages / flashcard-session | Route screen; handles both the concept and due-review modes (see note below). |
| `src/vue-app/views/DueCardsView.vue` | `src/pages/due-cards/ui/DueCardsPage.vue` | pages / due-cards | Route screen listing due cards. |
| `src/vue-app/views/FeynmanModule.vue` | `src/pages/feynman/ui/FeynmanPage.vue` | pages / feynman | Route screen; UI is a placeholder (§7f). |
| `src/vue-app/__tests__/review-store.test.ts` | `src/entities/review/model/useReviewStore.test.ts` | entities / review | Tests the review store; colocated. |
| `src/__tests__/types.test.ts` | split → `entities/*/model/validation.test.ts` + `entities/flashcard/lib/markdown-parser.test.ts` | entities + shared | Tests validation + parser across entities; splits with its subjects (§7c). |
| `src/shims-vue.d.ts` | `src/shims-vue.d.ts` (unchanged) | **not a layer** | Ambient `*.vue` module declaration; tooling/type glue, not domain code. |

> **`FlashcardSession.vue` note (uncertain):** one component serves two routes (`flashcards` concept mode and `review` due-review mode) behind `reviewStore.isSessionActive`. It is mapped to one page now. A later split into `pages/flashcards` + `pages/review` is possible but **not part of this contract**; **decided:** it stays one page (see §13).

### 6b. `dev/` (2 files) and referenced configs (4 files)

| Current path | Target path | Layer / slice | Reason |
| --- | --- | --- | --- |
| `dev/index.html` | `dev/index.html` (unchanged) | dev harness (outside layers) | Standalone Vite preview with fallback CSS variables. |
| `dev/main.ts` | `dev/main.ts` (imports updated to `@/`) | dev harness (outside layers) | Vite dev entry; imports views/components directly today. |
| `vitest.config.ts` | `vitest.config.ts` (alias updated) | config | `obsidian` alias → `src/shared/obsidian/__mocks__/obsidian.ts`; add `@/` resolve alias. |
| `esbuild.config.mjs` | `esbuild.config.mjs` (entry updated) | config | `entryPoints: ["src/main.ts"]` → `["src/app/main.ts"]`; `@/` resolves via tsconfig `paths`. |
| `vite.config.ts` | `vite.config.ts` (alias added) | config | Add `@/` resolve alias so the dev harness can import `src/` with `@/`. |
| `tsconfig.json` | `tsconfig.json` (paths added) | config | Add `paths: { "@/*": ["./src/*"] }`; `include` (`src/**/*.{ts,vue}`) is unchanged. |

### 6c. Configs with no moved-path references (unchanged, listed for completeness)

| Current path | Target path | Reason |
| --- | --- | --- |
| `esbuild-vue-plugin.mjs` | unchanged | Generic `.vue` loader; has no hardcoded `src/` path. |
| `package.json` | unchanged | Scripts point at configs (`node esbuild.config.mjs`, `vite`, `vitest run`); no source paths. |
| `manifest.json` | unchanged | Obsidian manifest; no source paths. |

**Files I could not classify as belonging to an FSD layer** (flagged, not guessed): `src/shims-vue.d.ts` (ambient declaration), `dev/index.html` and `dev/main.ts` (dev harness), and the four config files — these are tooling/infra, not layer code. Everything under `src/` that is application behavior was classified.

## 7. Decomposition notes (historical plan; the decisions in §13 still hold)

These files had to be **split, not just moved**.

### 7a. `src/vue-app/stores/study-store.ts`

Split by responsibility into entity model + feature model:

| Current piece | Target |
| --- | --- |
| `topics`, `loadTopics()`, topic DTO→`Topic` mapping | `entities/topic/model/useTopicsStore.ts` |
| `flashcardsMap`, `getCachedFlashcards()` | `entities/flashcard/model/useFlashcardCache.ts` |
| `currentTopicId`, `currentConceptId`, `selectTopic`, `selectConcept`, `currentTopic`, `currentConcept`, `currentFlashcards` | `features/select-topic/model/useTopicSelection.ts` + `features/select-concept/model/useConceptSelection.ts` |
| `syncCurrentNote(...)` | `features/sync-note/model/useSyncNote.ts` |
| `createTopicAction(name)` | `features/create-topic/model/useCreateTopic.ts` (+ `entities/topic/api` `createTopic`) |
| `createConceptAction(topicId, title)` | `features/create-concept/model/useCreateConcept.ts` (+ `entities/concept/api` `createConcept`) |
| `loading`, `error`, `clearError()` | Split per store/feature: each store owns its own `loading`/`error`; `clearError` re-exported from where the call site reads it |

Selection is a feature because it is a stateful user interaction that crosses the topic/concept entities; a feature may read entities but an entity may not read another entity.

### 7b. `src/vue-app/views/DashboardView.vue`

| Current section | Target |
| --- | --- |
| Metrics strip (`<MetricsBar />`) | `widgets/metrics-bar/ui/MetricsBar.vue` |
| Due CTA (`<DailyReviewCard />`) | `widgets/daily-review/ui/DailyReviewCard.vue` |
| Topic grid (`v-for TopicCard`) | `widgets/topic-list/ui/TopicList.vue` |
| Create-topic form (`showNewTopicForm`, `handleCreateTopic`) | `features/create-topic/ui/CreateTopicForm.vue` + `features/create-topic/model/useCreateTopic.ts` |
| Sync button + `syncActiveNote()` | `features/sync-note/ui/SyncNoteButton.vue` + `features/sync-note/model/useSyncNote.ts` |
| API-base setting (`<details>`, `saveApiBase`, `resetApiBaseUrl`) | `app/settings-tab.ts`: one Obsidian `Setting` holding the backend base URL, with the value in `shared/config` pushed to `shared/api/client.ts` by `app/settings-store.ts` |
| Page shell: loading/error/retry, layout, `onMounted` loads | `pages/dashboard/ui/DashboardPage.vue` |

`TopicView.vue` has the same grid shape; extract `widgets/concept-list` there for symmetry (recommended, not strictly required by the parent mapping).

### 7c. `src/types.ts` and `src/validation.ts`

`src/types.ts`:

| Type | Target |
| --- | --- |
| `Topic`, `TopicResponse`, `ConceptDTO`, `CreateTopicRequest`, `CreateTopicResponse` | `entities/topic/model/types.ts` |
| `ConceptSummary`, `SyncConceptRequest`, `SyncConceptResponse`, `CreateConceptRequest`, `CreateConceptResponse` | `entities/concept/model/types.ts` |
| `Flashcard`, `FlashcardInput`, `SyncFlashcardsRequest`, `SyncFlashcardsResponse` | `entities/flashcard/model/types.ts` |
| `StudyMetrics` | `entities/metrics/model/types.ts` |
| `DueCard`, `ReviewRequest`, `ReviewResponse` | `entities/review/model/types.ts` |
| `ResourceType`, `SyncResourceRequest`, `SyncResourceResponse` | `entities/resource/model/types.ts` |

`src/validation.ts` (today **no production consumer** — imported only by `src/__tests__/types.test.ts`; only `parseFlashcards` is used at runtime, so the split has no runtime impact):

| Function | Target |
| --- | --- |
| `isValidFlashcard` | `entities/flashcard/model/validation.ts` |
| `isValidConceptSummary` | `entities/concept/model/validation.ts` |
| `isValidTopic` | `entities/topic/model/validation.ts` |
| `isValidStudyMetrics` | `entities/metrics/model/validation.ts` |
| `isValidDueCard`, `isValidGrade`, `validateGrade`, `isValidReviewRequest` | `entities/review/model/validation.ts` |
| `isValidResourceType`, `isValidSyncResourceRequest` | `entities/resource/model/validation.ts` |
| `isNonEmptyString`, `isNonNegativeInteger`, `isRate` | `shared/lib/guards.ts` |
| `nextIndex`, `prevIndex` | `shared/lib/guards.ts` |

> **Uncertainty — `isValidTopic` ↔ `isValidConceptSummary`:** `isValidTopic` currently calls `isValidConceptSummary` to validate the embedded `concepts` array. Entity `topic` importing entity `concept` would be a forbidden same-layer cross-import. Recommended resolution: `isValidTopic` validates the embedded summary shape locally using the shared `isNonEmptyString` guard, while `entities/concept` keeps `isValidConceptSummary` for its own use. Alternative: treat `ConceptSummary` as part of the topic aggregate and let `entities/topic` own it. Both are defensible; **decided:** `entities/concept` owns `ConceptSummary` and `entities/topic` validates the embedded summary locally (see §13).

### 7d. `src/vue-app/services/api.ts`

| Current export | Target |
| --- | --- |
| `DEFAULT_API_BASE`, `getApiBase`, `setApiBase`, `resetApiBase` | `shared/api/client.ts` (base-URL config) |
| `NetworkError`, `ApiError`, `request<T>()` | `shared/api/client.ts` (HTTP core) |
| `fetchTopics`, `createTopic` | `entities/topic/api/topic-api.ts` |
| `syncConcept`, `createConcept` | `entities/concept/api/concept-api.ts` |
| `syncFlashcards` | `entities/flashcard/api/flashcard-api.ts` |
| `fetchDueCards`, `submitReview` | `entities/review/api/review-api.ts` |
| `syncResource` | `entities/resource/api/resource-api.ts` (defined but currently unused; **the scaffold is kept**, see §13) |

Entity api segments import `request`/errors from `shared/api/client.ts` (downward), never the reverse.

### 7e. `src/vue-app/components/*` — entity card vs widget

| Component | Decision | Why |
| --- | --- | --- |
| `TopicCard.vue` | `entities/topic/ui/` card | Renders one `Topic` and its concept count; pure presentational entity card. |
| `ConceptCard.vue` | `entities/concept/ui/` card | Renders one `ConceptSummary`. It currently reads the flashcard cache via the study store; refactor to receive `flashcardCount` as a **prop** so the concept entity does not import the flashcard entity. |
| `FlashcardCard.vue` | `entities/flashcard/ui/` card | Presents a flashcard's question/answer; no other entity dependency. |
| `DueCardItem.vue` | `entities/review/ui/` card | Renders one `DueCard`; owned by the review entity. |
| `MetricsBar.vue` | `widgets/metrics-bar/` block | Composes several metrics values into a bar; a composite block, not a single entity render. |
| `DailyReviewCard.vue` | `widgets/daily-review/` block | A due-count CTA with navigation; a composite block spanning a count and an action. |

### 7f. `metrics-store.ts` and `FeynmanModule.vue`

- **`metrics-store.ts`** → `entities/metrics/model/useMetricsStore.ts`. It seeds state from a hardcoded `MOCK_METRICS` object and `fetchMetrics()` is a documented no-op stub (comment: “Placeholder for future GET /api/metrics endpoint”). It lands in the metrics entity unchanged; the placeholder stays a placeholder, and wiring a real endpoint is out of scope. `widgets/metrics-bar` reads it through the entity's `index.ts`.
- **`FeynmanModule.vue`** → `pages/feynman/ui/FeynmanPage.vue`. The `explain()` function returns a hardcoded placeholder string (comment: “Mock response — real RAG-powered analysis coming in v2.1”). It lands as a page with no feature/model extraction yet; the RAG feature is deferred.

## 8. Reserved slices — all retired

The slices this contract reserved for the next features (the chat, its widgets
and its features, the corpus, the document and search-result entities, the
upload and search features, `shared/llm`) were implemented and then **removed on
2026-10-20** (§14). Nothing in this section is pending any more: the reserved
slices were removed, and the slices built afterwards are the launcher (§15), the
deck surface and the capture form (`PRESENTATION-UI.md` §9).

**Chat persistent view (removed).** It was a main-area tab registered from `app/`
(an `ItemView` in `src/app/study-chat-view.ts`, view type `study-chat`,
activated with `workspace.getLeaf("tab")`). The file and its view type are gone;
`app/` no longer calls `getLeaf("tab")`.

**Backend status (historical).** The Go service still exposes
`POST /api/rag/documents`, `POST /api/rag/documents/text`,
`POST /api/rag/documents/url`, `GET /api/rag/documents/{id}/status` and
`GET /api/rag/search`, plus its ingestion and study endpoints. The deleted plugin
called none of the RAG ones; the web app calls the document endpoints again from
the launcher and the capture form.

## 9. Naming and imports

- **Slice folders**: kebab-case (`entities/flashcard`, `features/create-topic`).
- **Public API**: every slice has `index.ts`; nothing outside the slice imports a segment path directly.
- **Absolute vs relative**: cross-slice imports use the absolute `@/` alias; imports **inside** a slice (between its own segments) use relative paths.

```ts
// app -> pages
import { DashboardPage } from "@/pages/dashboard";
// app -> shared (a shared segment, not a slice)
import { useSwipe } from "@/shared/lib";
// pages -> widgets
import { MetricsBar } from "@/widgets/metrics-bar";
// pages -> features
import { useCreateTopic } from "@/features/create-topic";
// pages -> entities
import { TopicCard } from "@/entities/topic";
// widgets -> features
import { useSyncNote } from "@/features/sync-note";
// features -> entities
import { useTopicsStore } from "@/entities/topic";
// entities -> shared
import { request } from "@/shared/api/client";
// inside a slice (relative)
import { isValidTopic } from "./validation";
```

`@/` must resolve in every runner: Vite/Vitest do **not** read tsconfig `paths` on their own, so `vite.config.ts` and `vitest.config.ts` each declare an explicit `resolve.alias` entry mapping `@` → `src`. Vitest runs with `environment: "node"`; component tests render through `vue/server-renderer`, so no jsdom or browser shim is installed.

## 10. Enforcement

**Today there is no linter** (verified: no ESLint config or dependency anywhere in the repo). FSD degrades without enforcement — the dependency rule and the public-API rule are conventions that drift the moment they are unguarded. Two options; the first is a decision the user must make:

| Option | Cost | Notes |
| --- | --- | --- |
| `eslint` + `eslint-plugin-boundaries` (or `@feature-sliced/eslint-config`) | New devDependencies + config | Enforces layer direction and same-layer cross-imports at lint time. **Requires a decision** — it adds dependencies. |
| Minimal in-repo boundary test (no new dependency) | ~1 vitest file | A test that walks `src/`, parses import statements, and asserts each import points to an allowed lower layer (and not to a same-layer sibling). Deterministic, ships with the suite, but is a test, not a linter. |

**Decision (2026-10-17): the in-repo boundary test, with no new dependencies.**
It ships as a vitest file (`src/architecture.test.ts`) that walks `src/`, parses
every relative and `@/` import, and fails when an import points upward, to a
sibling slice of the same layer, or when any file imports `obsidian`. ESLint
remains a possible future upgrade, but the repository adds no dependency now.

**Implemented (2026-10-17, Phase 6):** `src/architecture.test.ts` is that test.
It walks every `.ts`/`.vue` under `src/`, extracts module specifiers, and fails on
six rules: an alias import to a higher layer; a same-layer sibling slice; an alias
import that does not land on a slice root (`@/<layer>/<slice>`, while `shared` is
addressed by segment as `@/shared/<segment>[/<file>]`); a relative import that
leaves the file's own slice; an `obsidian` import anywhere; and any file outside
the layers that imports into them. A seventh case asserts the walk
actually found the expected files, so the guard cannot pass vacuously.

**Revised (2026-10-27): `shared` has segments, not slices.** The same-layer rule
fired before the `shared` exemption could be reached, so the first real
intra-`shared` import (`shared/ui/*` needing `cn` from `shared/lib/utils`)
failed as `same-layer-sibling` — and there was no legal workaround, because a
relative `../../lib/utils` is a `relative-escape`. The guard now exempts `shared`
from the same-layer rule and keeps only the shape check
(`@/shared/<segment>[/<file>]`; bare `@/shared` still fails). Two structural
changes came with it:

- The rule engine was split from import extraction (`scanSpecifiers` takes
  specifiers). Before, the guard read its own file and mistook a fixture — and
  even a comment — that contained a full import statement for a real import of
  `architecture.test.ts`, which sits outside the layers and may import none.
- Eight cases were added that feed synthetic specifiers and assert each rule
  still fires, in both directions. The suite previously asserted only that the
  real tree was clean, which is also what a guard that silently stopped matching
  would report. The file is now 15 tests: 7 on the real tree, 8 on the rules.

## 11. Migration phases (historical)

Each phase is mechanical and keeps the suite at **94 passing tests**. During migration, old paths keep thin re-export shims (`export * from "@/…"`) so untouched importers and tests stay green; shims are deleted in Phase 6.

| Phase | What moves | 94 tests stay green because |
| --- | --- | --- |
| **0 — Contract** | Nothing. This document. | No code touched; 94 green. |
| **1 — Foundation** | Create `shared/` + `app/`; move `useObsidian.ts`, `__mocks__/obsidian.ts`, `useSwipe.ts`; extract `shared/api/client.ts`; move `main.ts`/`study-view.ts` → `app/`; move `App.vue`/`router`. Update esbuild entry, vitest alias, tsconfig `paths`, `@/` aliases. | Files move with re-export shims; `useObsidian.test.ts` moves colocated; assertions unchanged. |
| **2 — Entities** | Split `types.ts`/`validation.ts` by entity; move `review-store`, `metrics-store`, `markdown-parser`, the `api.ts` segments and the entity `ui/` cards. Move `review-store.test.ts`; split `types.test.ts` into colocated suites. **`study-store.ts` is left untouched**: its decomposition into the topic/flashcard entity stores plus the feature models happens in Phase 3, so there is never a second source of topic state. | Test bodies are copied verbatim into per-entity suites; total `it()` count stays 94. |
| **3 — Features** | Extract `create-topic`, `create-concept`, `sync-note`, `configure-api-base`, `select-topic`, `select-concept`, `review-card` from `study-store` and `DashboardView`, and create the `entities/topic/model/useTopicsStore.ts` and `entities/flashcard/model/useFlashcardCache.ts` stores that `study-store.ts` owned. | Feature models are exercised through the same store/api mocks the current tests use; shims keep store names resolvable. |
| **4 — Widgets + pages** | Split `DashboardView` into `pages/dashboard` + `widgets/*` + feature UIs; rename the remaining views to pages. | Page/widget moves keep templates and behavior; no test asserts file paths. |
| **5 — New slices** | Scaffold `chat`/`document`/`search` slices and register the `study-chat` tab from `app/`. | New code is additive and untested until endpoints exist; existing 94 unaffected. |
| **6 — Enforcement + cleanup** | Add the chosen boundary guard (§10); delete all re-export shims and old `src/vue-app/` folders. | Suite stays green after shim removal because every importer now points at `@/`. |

## 12. Non-goals (the migration's)

- **No rewrite of working logic** — sync (`syncCurrentNote`), flashcard parsing (`parseFlashcards`), and review/FSRS (`useReviewStore`, grade handling) behavior was preserved. The migration relocated and split files; it did not change what they do.
- **No backend changes** — the migration left the Go API out of scope. The app now consumes it; the middleware documents its own endpoints in `apps/go-middleware/README.md`.
- **No new runtime dependencies** — the runtime stays `vue`, `pinia`, `vue-router`. The only possible additions are devDependency lint tooling, and only if the user decides to adopt it (§10).

## 13. Decisions (confirmed 2026-10-17)

All five open questions are resolved; the contract above is governing.

Decisions 2 and 5 are now historical: the `obsidian` mock and its alias were
deleted with the plugin (§4), and decision 5 was superseded by §14 and then by
§15.

1. **`ConceptSummary` ownership** — `entities/concept` owns it. `entities/topic`
   validates the embedded summary locally with the `shared/lib` guards, so no
   same-layer cross-import is introduced (§7c).
2. **Obsidian mock location (historical)** — it was
   `src/shared/obsidian/__mocks__/obsidian.ts`, colocated with the boundary; the
   mock and its vitest alias went with the plugin (§4).
3. **`FlashcardSession`** — stays **one page** for both the concept and the
   due-review modes. Splitting it is a later option, not part of this contract (§6a).
4. **Boundary enforcement** — the **in-repo boundary test, no new dependencies**
   (§10).
5. **`syncResource` / `entities/resource` (historical)** — the scaffold was
   **kept** at the time of this contract; it is **removed** by §14, together with
   the rest of the document corpus, and re-added in a new shape by §15.

## 14. Scope reduction (2026-10-20, historical)

This records the plugin's reduction to flashcards-only. The following were
removed and are not to be reintroduced without a new decision; the web app has
since re-added `entities/resource` under §15 and captures documents into the
corpus from `features/capture-material`, while the chat and its slices stay
removed:

| Removed | Layer | Note |
| --- | --- | --- |
| `app/study-chat-view.ts` | app | The `study-chat` main-area tab; its view type and its command are gone. |
| `pages/chat` | pages | The chat screen. |
| `widgets/chat-list`, `widgets/chat-thread`, `widgets/chat-settings` | widgets | Chat composition. |
| `features/create-chat`, `features/select-chat`, `features/edit-chat-prompt`, `features/send-message`, `features/attach-resource`, `features/configure-llm` | features | Chat lifecycle, attachments and the LLM settings form. |
| `entities/chat` | entities | The chat note: prompt, overrides and transcript. |
| `shared/llm` | shared | The provider transport (chat completions + SSE). |
| Chat-only helpers in `shared/obsidian/useObsidian.ts` | shared | `chatsFolderPath`, `readVaultFile`, `writeVaultFile`, `processVaultFile`, `listVaultFiles`, `listVaultMarkdownFiles`, `onVaultFileChange`. |
| `pages/corpus` | pages | The upload and search screen. |
| `widgets/search-results` | widgets | Rendered RAG search results. |
| `features/upload-document`, `features/search-corpus` | features | Uploading a document to the corpus and querying it. |
| `entities/document`, `entities/search-result` | entities | Document and search-result models plus their `api/` segments. |
| `entities/resource` | entities | The `syncResource` scaffold, which no feature ever called. Superseded by §15 on 2026-10-22. |

`shared/config` keeps one field, `apiBase`, and it is still pure. The app has no
settings load at all — the API base is the origin that served the page — so the
`SettingsStore` port has no consumer yet. `normalizeSettings` drops unknown keys,
including the `llm*`, `retrievalTopK`, `historyTurns` and `defaultChatPrompt`
fields an older version stored, which is what lets a settings file written by
that version load without them.

The Go backend was untouched by this reduction: the plugin stopped calling the
endpoints, and the web app now calls the ones it needs.

## 15. `entities/resource` returns, for the launcher (2026-10-22)

§14 removed `entities/resource` as a `syncResource` scaffold and said it is not to
be reintroduced without a new decision. The decision is this one: the launcher
reads the stored resources from `GET /api/resources` instead of rendering
fixtures, so the app needs one owner for that wire shape.

| File | Owns |
| --- | --- |
| `model/types.ts` | `ResourceType` (the four stored values), `ResourceResponse` (the wire), `Resource` (the domain shape: `sourceUri`, `topicTitle`, `createdAt`) |
| `api/resource-api.ts` | `fetchResources()`, one `GET /api/resources` |
| `model/useResourcesStore.ts` | `resources`, `loading`, `error`, `loadResources`, `clearError` |

It is not the old scaffold: the app still has no `syncResource` and no
`SyncResourceRequest` — the middleware owns the write. What comes back is a read
model, and the launcher's view model (`kind`, the per-kind status line, the
primary action) stays inside `pages/resources`: the entity must not know how the
launcher decides to render a card.
