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
- [ ] T2 — Phase 2 Entities: split `types.ts`/`validation.ts` by entity, move the
      stores, `markdown-parser`, the api segments and the entity cards, and
      colocate the tests. (S2, S3)
- [ ] T3 — Phase 3 Features: extract `create-topic`, `create-concept`,
      `sync-note`, `configure-api-base`, `select-topic`, `select-concept`,
      `review-card`. (S2, S3)
- [ ] T4 — Phase 4 Widgets + pages: split `DashboardView` and rename the views to
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
