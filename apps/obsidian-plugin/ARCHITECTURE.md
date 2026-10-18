# Obsidian Plugin — Architecture Contract (Phase 0)

## 1. Status and scope

This is the **governing Feature-Sliced Design (FSD) contract for the Obsidian plugin** at `apps/obsidian-plugin`. It is decided **before any file moves**: it fixes the target layers, slice/segment conventions, the Obsidian boundary, the file-by-file mapping, and the migration phases so that later mechanical moves follow one agreed structure instead of re-litigating placement. It is a design artifact only — it changes no code and moves no files. Every mapping below was derived from the actual contents of `src/`, `dev/`, and the build/test configs at the time of writing (`package.json` v2.2.0). Uncertainties are stated explicitly rather than guessed.

Current shape (verified): a Vue 3 + Pinia SPA lives under `src/vue-app/` (`views/`, `components/`, `stores/`, `services/`, `composables/`, `router/`, `App.vue`), with domain types/validation at `src/types.ts` and `src/validation.ts`, and the Obsidian integration at `src/main.ts` + `src/study-view.ts`. There is **no ESLint or boundary linter** in the repository. The test suite is **94 tests** (`it()` count: 67 in `src/__tests__/types.test.ts`, 15 in `src/vue-app/composables/__tests__/useObsidian.test.ts`, 12 in `src/vue-app/__tests__/review-store.test.ts`).

## 2. Layers and the dependency rule

FSD layers used here (there is **no `processes` layer** — it is deprecated and is not used):

| Layer | Role | Obsidian-aware? |
| --- | --- | --- |
| `app` | Integration/bootstrap: plugin entry, `ItemView`, router, root component. | Yes |
| `pages` | Route-level screens. | No |
| `widgets` | Self-contained compositional UI blocks used by pages. | No |
| `features` | User interactions that bring value (create, sync, submit, select). | No |
| `entities` | Domain models, their stores, api segments and cards. | No |
| `shared` | Reusable, domain-free infrastructure (`api` client, `lib`, `obsidian` boundary). | Only `shared/obsidian/` |

**The dependency rule** — imports may only point downward:

```text
app → pages → widgets → features → entities → shared
```

- A lower layer never imports a higher layer.
- **No cross-imports between slices of the same layer** (e.g. `entities/topic` must not import `entities/concept`; `features/create-topic` must not import `features/create-concept`).
- A slice may import multiple lower-layer slices, but only through their public API (`index.ts`).

## 3. Slice and segment conventions

- **Slice**: a folder in a layer named in **kebab-case** (`entities/topic`, `features/create-topic`). Each slice is a single unit of the layer's meaning.
- **Public API**: every slice exposes exactly one entry point, `index.ts`. Only what that file re-exports is public.
- **Segments** (internal folders of a slice): `ui/` (components), `model/` (state, stores, types, validation), `api/` (server calls), `lib/` (pure helpers), `config/` (constants/configuration).
- **Import rule**: a slice may only be imported through its `index.ts`. Deep imports into a slice's segment (`entities/topic/model/types`) are forbidden.
- **Tests are colocated** with the slice they test (`entities/review/model/useReviewStore.test.ts`), except ambient/infra tests which live next to the file under test.

## 4. The Obsidian boundary

The `obsidian` package (an Obsidian-runtime module, not a normal npm dependency) may be imported by **only two places**:

1. `src/app/` — the integration layer (`main.ts`, `study-view.ts`).
2. `src/shared/obsidian/` — the single boundary module.

`pages`, `widgets`, `features` and `entities` must stay **framework-agnostic and testable without the obsidian mock**.

Today `src/vue-app/composables/useObsidian.ts` is that boundary and `src/study-view.ts` / `src/main.ts` are the integration. Verified current `obsidian` importers: `src/main.ts`, `src/study-view.ts`, `src/vue-app/composables/useObsidian.ts` — exactly the three expected, so the rule is true today and must remain true after the move. The vitest `obsidian` alias (`vitest.config.ts`) stays in place so boundary tests can run without the real package.

## 5. Target tree

`+` marks a new slice that does not exist today (see §8). Segments are shown where the slice is non-trivial.

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
    │   ├── study-chat-view.ts         # + chat main-area tab (view type study-chat)
    │   ├── router/index.ts
    │   └── ui/App.vue
    ├── pages/
    │   ├── dashboard/{index.ts,ui/DashboardPage.vue}
    │   ├── topic/{index.ts,ui/TopicPage.vue}
    │   ├── concept/{index.ts,ui/ConceptPage.vue}
    │   ├── flashcard-session/{index.ts,ui/FlashcardSessionPage.vue}
    │   ├── due-cards/{index.ts,ui/DueCardsPage.vue}
    │   ├── feynman/{index.ts,ui/FeynmanPage.vue}
    │   └── chat/{index.ts,ui/ChatPage.vue}            # +
    ├── widgets/
    │   ├── metrics-bar/{index.ts,ui/MetricsBar.vue}
    │   ├── daily-review/{index.ts,ui/DailyReviewCard.vue}
    │   ├── topic-list/{index.ts,ui/TopicList.vue}
    │   ├── concept-list/{index.ts,ui/ConceptList.vue}
    │   ├── chat-panel/{index.ts,ui/ChatPanel.vue}     # +
    │   └── search-results/{index.ts,ui/SearchResults.vue}  # +
    ├── features/
    │   ├── create-topic/{index.ts,model/useCreateTopic.ts,ui/CreateTopicForm.vue}
    │   ├── create-concept/{index.ts,model/useCreateConcept.ts,ui/CreateConceptForm.vue}
    │   ├── sync-note/{index.ts,model/useSyncNote.ts,ui/SyncNoteButton.vue}
    │   ├── configure-api-base/{index.ts,ui/ApiBaseSettings.vue}
    │   ├── select-topic/{index.ts,model/useTopicSelection.ts}
    │   ├── select-concept/{index.ts,model/useConceptSelection.ts}
    │   ├── review-card/{index.ts,model/useReviewSession.ts}
    │   ├── ask-chat/{index.ts,model/useAskChat.ts,ui/AskChatForm.vue}        # +
    │   ├── upload-document/{index.ts,model/useUploadDocument.ts,ui/UploadDocumentForm.vue}  # +
    │   └── search-corpus/{index.ts,model/useSearchCorpus.ts,ui/SearchCorpusForm.vue}        # +
    ├── entities/
    │   ├── topic/{index.ts,model/{types.ts,validation.ts,useTopicsStore.ts},api/topic-api.ts,ui/TopicCard.vue}
    │   ├── concept/{index.ts,model/{types.ts,validation.ts},api/concept-api.ts,ui/ConceptCard.vue}
    │   ├── flashcard/{index.ts,model/{types.ts,validation.ts,useFlashcardCache.ts},api/flashcard-api.ts,lib/markdown-parser.ts,ui/FlashcardCard.vue}
    │   ├── review/{index.ts,model/{types.ts,validation.ts,useReviewStore.ts},api/review-api.ts,ui/DueCardItem.vue}
    │   ├── metrics/{index.ts,model/{types.ts,validation.ts,useMetricsStore.ts}}
    │   ├── resource/{index.ts,model/{types.ts,validation.ts},api/resource-api.ts}
    │   ├── chat-message/{index.ts,model/types.ts}          # +
    │   ├── document/{index.ts,model/types.ts}              # +
    │   └── search-result/{index.ts,model/types.ts}         # +
    └── shared/
        ├── api/client.ts
        ├── lib/{guards.ts,useSwipe.ts}
        └── obsidian/{index.ts,useObsidian.ts,useObsidian.test.ts,__mocks__/obsidian.ts}
```

## 6. File-by-file mapping

One row per current file. `Layer / slice` is the parent slice in the target tree. Files that cannot be assigned to a layer are flagged explicitly.

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

## 7. Decomposition notes

These files must be **split, not just moved**.

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
| API-base settings (`<details>`, `saveApiBase`, `resetApiBaseUrl`) | `features/configure-api-base/ui/ApiBaseSettings.vue` + `shared/api/client.ts` |
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

## 8. New slices for the next features

These slices do not exist today; the tree reserves them. They are **not** implemented by this contract.

| New slice | Layer | Purpose |
| --- | --- | --- |
| `pages/chat` | pages | Chat screen inside the persistent main-area tab. |
| `widgets/chat-panel` | widgets | Conversation panel composed of chat-message entities + ask-chat feature. |
| `features/ask-chat` | features | Send a question, stream/receive an answer. |
| `entities/chat-message` | entities | Chat message model/types. |
| `features/upload-document` | features | Upload a document into the corpus. |
| `entities/document` | entities | Document model/types. |
| `features/search-corpus` | features | Query the RAG corpus. |
| `entities/search-result` | entities | Search result model/types. |
| `widgets/search-results` | widgets | Render search results. |

**Chat persistent view (important):** the chat's persistent view is a **main-area tab registered from `app/`** — an `ItemView` (file `src/app/study-chat-view.ts`, view type `study-chat`) activated with `workspace.getLeaf("tab")`, mirroring today's `StudyView` sidebar pattern (`getRightLeaf`). Only `app/` may call `getLeaf("tab")` and import `obsidian`.

**Backend gap:** the chat backend endpoint **does not exist yet**. `features/ask-chat` must be scaffolded against a documented contract, and its API call cannot be wired to a live server until the endpoint is added. State this in the feature's README and keep it below the `shared/api/client.ts` boundary.

## 9. Naming and imports

- **Slice folders**: kebab-case (`entities/chat-message`, `features/upload-document`).
- **Public API**: every slice has `index.ts`; nothing outside the slice imports a segment path directly.
- **Absolute vs relative**: cross-slice imports use the absolute `@/` alias; imports **inside** a slice (between its own segments) use relative paths.

```ts
// app -> pages
import { DashboardPage } from "@/pages/dashboard";
// app -> shared
import { useObsidian } from "@/shared/obsidian";
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

`@/` must resolve in every runner: esbuild resolves it through `tsconfig.json` `paths` (verified: esbuild bundles `@/validation` using `tsconfigRaw.paths`); Vite/Vitest do **not** read tsconfig `paths` on their own, so `vite.config.ts` and `vitest.config.ts` need an explicit `resolve.alias` entry mapping `@` → `src`. The vitest `obsidian` alias stays and points at the relocated mock.

## 10. Enforcement

**Today there is no linter** (verified: no ESLint config or dependency anywhere in the repo). FSD degrades without enforcement — the dependency rule and the public-API rule are conventions that drift the moment they are unguarded. Two options; the first is a decision the user must make:

| Option | Cost | Notes |
| --- | --- | --- |
| `eslint` + `eslint-plugin-boundaries` (or `@feature-sliced/eslint-config`) | New devDependencies + config | Enforces layer direction and same-layer cross-imports at lint time. **Requires a decision** — it adds dependencies. |
| Minimal in-repo boundary test (no new dependency) | ~1 vitest file | A test that walks `src/`, parses import statements, and asserts each import points to an allowed lower layer (and not to a same-layer sibling). Deterministic, ships with the suite, but is a test, not a linter. |

**Decision (2026-10-17): the in-repo boundary test, with no new dependencies.**
It ships as a vitest file (for example `src/architecture.test.ts`) that walks
`src/`, parses every relative and `@/` import, and fails when an import points
upward, to a sibling slice of the same layer, or when a file other than `app/`
and `shared/obsidian/` imports `obsidian`. ESLint remains a possible future
upgrade, but the repository adds no dependency now.

## 11. Migration phases

Each phase is mechanical and keeps the suite at **94 passing tests**. During migration, old paths keep thin re-export shims (`export * from "@/…"`) so untouched importers and tests stay green; shims are deleted in Phase 6.

| Phase | What moves | 94 tests stay green because |
| --- | --- | --- |
| **0 — Contract** | Nothing. This document. | No code touched; 94 green. |
| **1 — Foundation** | Create `shared/` + `app/`; move `useObsidian.ts`, `__mocks__/obsidian.ts`, `useSwipe.ts`; extract `shared/api/client.ts`; move `main.ts`/`study-view.ts` → `app/`; move `App.vue`/`router`. Update esbuild entry, vitest alias, tsconfig `paths`, `@/` aliases. | Files move with re-export shims; `useObsidian.test.ts` moves colocated; assertions unchanged. |
| **2 — Entities** | Split `types.ts`/`validation.ts` by entity; move `study-store` state, `review-store`, `metrics-store`, `markdown-parser`, `api.ts` segments, entity `ui/` cards. Move `review-store.test.ts`; split `types.test.ts` into colocated suites. | Test bodies are copied verbatim into per-entity suites; total `it()` count stays 94. |
| **3 — Features** | Extract `create-topic`, `create-concept`, `sync-note`, `configure-api-base`, `select-topic`, `select-concept`, `review-card` from `study-store` and `DashboardView`. | Feature models are exercised through the same store/api mocks the current tests use; shims keep store names resolvable. |
| **4 — Widgets + pages** | Split `DashboardView` into `pages/dashboard` + `widgets/*` + feature UIs; rename the remaining views to pages. | Page/widget moves keep templates and behavior; no test asserts file paths. |
| **5 — New slices** | Scaffold `chat`/`document`/`search` slices and register the `study-chat` tab from `app/`. | New code is additive and untested until endpoints exist; existing 94 unaffected. |
| **6 — Enforcement + cleanup** | Add the chosen boundary guard (§10); delete all re-export shims and old `src/vue-app/` folders. | Suite stays green after shim removal because every importer now points at `@/`. |

## 12. Non-goals

- **No rewrite of working logic** — sync (`syncCurrentNote`, `syncActiveNote`), flashcard parsing (`parseFlashcards`), and review/FSRS (`review-store`, grade handling) behavior is preserved. This contract relocates and splits files; it does not change what they do.
- **No backend changes** — the Go API and its endpoints are out of scope; the missing chat endpoint is recorded as a gap, not implemented here.
- **No new runtime dependencies** — the runtime stays `vue`, `pinia`, `vue-router`. The only possible additions are devDependency lint tooling, and only if the user decides to adopt it (§10).

## 13. Decisions (confirmed 2026-10-17)

All five open questions are resolved; the contract above is governing.

1. **`ConceptSummary` ownership** — `entities/concept` owns it. `entities/topic`
   validates the embedded summary locally with the `shared/lib` guards, so no
   same-layer cross-import is introduced (§7c).
2. **Obsidian mock location** — `src/shared/obsidian/__mocks__/obsidian.ts`,
   colocated with the boundary; the vitest alias is updated to point at it (§6a).
3. **`FlashcardSession`** — stays **one page** for both the concept and the
   due-review modes. Splitting it is a later option, not part of this contract (§6a).
4. **Boundary enforcement** — the **in-repo boundary test, no new dependencies**
   (§10).
5. **`syncResource` / `entities/resource`** — the scaffold is **kept**: its types
   and validation are tested and the backend endpoint exists, so it is ready for a
   feature that calls it later (§7d).
