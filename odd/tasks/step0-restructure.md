# Step 0 — Restructure into bounded contexts and remove Dify

**Status:** done
**Created:** 2026-10-17
**Scope:** structural refactor only. No behaviour change, no new features.

## Why

The backend used a flat, layer-first layout (`domain/`, `application/`,
`infrastructure/`, `interfaces/`) that assumes a single bounded context (study:
topics -> concepts -> flashcards -> reviews).

A second context (RAG: documents, chunks, embeddings, and ingestion jobs) is
planned. Adding it to the flat layout would mix two unrelated domains in the same
folders. Step 0 makes room before the feature arrives.

Dify is dropped: retrieval will be implemented in this same server. Removing it
now was cheapest, because every later step touches less surface.

## Target layout

```text
internal/
├── platform/          cross-cutting, no business logic
│   ├── apperror/        sentinel errors shared by all contexts
│   └── httpx/           JSON/error writers, CORS middleware
└── study/             bounded context 1 (existing code, moved)
    ├── domain/
    ├── app/
    ├── sqlite/
    └── http/
```

## Invariants

- Zero behaviour change, except where removing Dify made behaviour impossible to
  preserve (recorded below).
- `go vet ./...` stays clean.
- No new dependency.

## Tasks

- [x] T1 — Verify baseline: `go vet` clean, 131 tests green, 95 frontend tests green
- [x] T2 — Move study code into `internal/study/{domain,app,sqlite,http}` (package `application` -> `app`)
- [x] T3 — Extract shared kernel into `internal/platform/{apperror,httpx}`
- [x] T4 — Rewrite imports and package references
- [x] T5 — Remove Dify from the backend (files, fields, routes, env, tests)
- [x] T6 — Remove Dify from the Obsidian plugin (view, types, api, router, button)
- [x] T7 — Split `http/handler.go` (386 lines, 9 handlers) into per-resource files
- [x] T8 — Rewire `cmd/server/main.go`
- [x] T9 — Verify: `go vet`, `go test ./...`, `npm test`, plugin build
- [x] T10 — Document the layout in `apps/go-middleware/README.md`

## Results

| Check | Before | After |
| --- | --- | --- |
| `go vet ./...` | clean | clean |
| `func Test` (backend) | 125 | 110 |
| frontend tests | 95 | 94 |
| `npm run build` | ok | ok |
| Dify references in source | 44 | 0 |

Test delta is fully accounted for: 5 tests in `app`, 6 in `sqlite`, and 4 in
`http` were Dify-specific. No unrelated test was removed.

## Deliberate behaviour changes

1. **`SyncResourceUseCase` lost its `difyDocumentID` parameter, and re-syncing an
   existing `source_uri` is now a no-op.** Previously the second sync updated the
   stored Dify document ID. With that field gone there is nothing left to update,
   so the use case returns the existing resource ID. The test that asserted the
   update was replaced by `TestSyncResourceUseCase_ExistingResourceIsReused`.
2. **`httpx.CORS` no longer sends a wildcard origin by default.** It reflects the
   request origin only when allowlisted, falling back to Obsidian and localhost
   origins. `STUDY_CORS_ORIGINS='*'` restores the old policy without a code
   change, which is the documented rollback path if the allowlist is wrong for a
   given device.

## Follow-ups

Recorded in [`backlog.md`](backlog.md), the single source of truth. Two entries
this step opened were closed by step 2: the migration mechanism moved to
`platform/db`, and the SQLite PRAGMAs were enabled.
