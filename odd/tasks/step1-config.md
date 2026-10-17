# Step 1 — Platform config, .env support, and secret hygiene

**Status:** done
**Created:** 2026-10-17
**Depends on:** step 0 (bounded contexts + `platform/` skeleton)

## Why

Two problems, one root cause: nothing owned configuration.

1. `httpx` read `STUDY_CORS_ORIGINS` straight from the environment, so the
   middleware owned a policy decision. Config should be resolved in exactly one
   place, and the middleware should be a pure function of its input.
2. There was no committed `.gitignore` at the repo root, so `data/study.db`,
   `.env`, and build artifacts were all stageable. The LLM API key for the coming
   RAG pipeline would have landed in a file that `git add .` commits.

The `DefaultAllowedOrigins` list added in step 0 was also unverified. Making it
config-driven is what turns that assumption into something testable.

## Delivered

- `internal/platform/config`: a single `Load()` that reads env vars, applies
  defaults, validates, and returns a typed `Config`.
- Optional `.env` file support, zero dependencies, real environment wins.
- `httpx.CORS` became a middleware factory over an explicit origin list. The
  study router stopped knowing that CORS exists.
- A committed root `.gitignore`.
- `data/.gitkeep` restored (it was deleted in the working tree).

## Not delivered, deliberately

`apps/go-middleware/.env.example` was **not created**: Pi's safety policy blocks
writes to `.env*` paths, and renaming the file to dodge that pattern was not a
call to make unilaterally. The user chose to keep the README as the single source
of truth for configuration. All three variables are documented there, including
the reserved-but-unread LLM key placeholders.

## Tasks

- [x] T1 — Task file (this document)
- [x] T2 — `platform/config`: `Config`, `Load`, validation, `.env` parser, tests
- [x] T3 — `httpx.CORS` became a factory over `[]string`; defaults moved to config
- [x] T4 — `study/http/router.go` stopped applying CORS
- [x] T5 — `cmd/server/main.go` consumes `Config`
- [x] T6 — Root `.gitignore` (the `.env.example` half dropped, see above)
- [x] T7 — Verify: build, vet, tests, and a runtime smoke test of the real server
- [x] T8 — Update the backend README

## Verification

| Check | Result |
| --- | --- |
| `gofmt -l` | clean |
| `go vet ./...` | clean |
| `func Test` (backend) | 110 -> **134** (24 new, all in `platform/config`) |
| `npm test` / `npm run build` | untouched, still green |
| `os.Getenv` in `httpx` | **0** |

Smoke test against the real binary, all green:

| Check | Result |
| --- | --- |
| Port from `STUDY_PORT` | `server listening on :18234` |
| DSN from `STUDY_DB_DSN` | database created at the configured path |
| `POST /api/topics` | 201 with a well-formed body |
| `GET /api/concepts` | reflects the created topic |
| CORS, allowlisted origin | `Access-Control-Allow-Origin` echoed |
| CORS, unknown origin | header absent |
| Preflight from unknown origin | 403 |
| `git check-ignore` `.env`, `data/study.db`, `*.db-wal`, `bin/` | all ignored |
| `git check-ignore` `.env.example`, `data/.gitkeep` | correctly not ignored |

Note: the first smoke run returned 400 on `POST /api/topics`. That was a shell
escaping bug in the test command, not a server defect; re-running with a clean
body file returned 201.

## Design notes

- `LoadFrom(path, lookup)` takes the environment lookup as a parameter, so tests
  are hermetic and never mutate the process environment.
- `ParsePort` is exported so the `-port` flag applies exactly the same bounds
  check as `STUDY_PORT`, instead of duplicating it.
- Invalid configuration aborts the boot with a message naming the variable. A bad
  port or an origin list made only of separators never falls back silently.

## Follow-ups

Recorded in [`backlog.md`](backlog.md), the single source of truth. Two entries
this step carried were closed by step 2: the migration mechanism moved to
`platform/db`, and the SQLite PRAGMAs were enabled.
