# Step 8 — One command with subcommands (CLI, stage 2)

**Status:** in progress
**Created:** 2026-10-20
**Depends on:** step 7 (`internal/composition`), `platform/config`, `platform/db`

## Specs

**S1 — The request.** "sigue con la siguiente etapa"

**S2 — Stage 2 as defined and accepted in the step 7 plan.** "`cmd/cli` con
subcomandos (`serve`, `search`, `reindex`, `embed`, `list`) migrando los modos
actuales. Sin tocar `study`."

**S3 — Constraints inherited from the system.** No new dependencies. Every
command keeps its behaviour: the same flags, the same output on stdout, the same
work. `cmd/server` keeps working while the CLI exists. The package tests stay
green. Stage 4 (path resolution) is **not** part of this step: `.env` and `data/`
are still resolved from the working directory.

## Why

`cmd/ingest` already has five modes behind one flag set, so it is a CLI with
flags where the natural shape is subcommands. With `serve` in the same binary,
one entrypoint replaces two, and the flag surface stops being "which combination
of booleans is legal" and becomes a list you can read.

## Design: the command layer is its own package

- `internal/server` holds the serve loop that `cmd/server/main.go` kept inline:
  mount both contexts, apply CORS, listen, shut down, drain the processor,
  **return** the error instead of calling `log.Fatal` from inside a goroutine.
- `internal/cli` holds the dispatch and every subcommand. `Run` takes
  `(ctx, args, stdout, stderr)` and returns an exit code, so the whole command
  surface is testable without a process.
- `cmd/server/main.go` becomes ~30 lines: flags, config, `server.Run`.
- `cmd/ingest` is deleted: its five modes are subcommands now, and keeping a
  second command surface would mean maintaining the same behaviour twice.

## Deliberate wording changes

The old binary prefixed a fatal error with `ingest:`. The new one prefixes it with
the subcommand name (`search: ...`, `convert: ...`), which is the only way a
single binary can say which command failed. **stdout is unchanged** for every
command, which is the part a script consumes; that is what the differential checks.

## Tasks

- [x] T1 — `internal/server`: extract the serve loop; `cmd/server` becomes a thin wrapper (S3)
- [x] T2 — `internal/cli`: dispatch, usage, exit codes; `cmd/cli` (S2)
- [x] T3 — subcommands `list`, `search`, `reindex`, `embed`, migrated from `cmd/ingest` (S2, S3)
- [x] T4 — subcommand `convert`, the PDF run (S2, S3)
- [x] T5 — `serve`; delete `cmd/ingest` (S2)
- [x] T6 — dispatch tests, README, differential verification, commits (S3)

## Log

**L1 (2026-10-20, request)** — "sigue con la siguiente etapa"

**L2 (2026-10-20, scope)** — stage 2 as accepted in the step 7 plan: one binary
with `serve`, `search`, `reindex`, `embed`, `list`, migrating the current modes,
without touching the `study` context. Stage 4 (paths) stays out: the CLI keeps
resolving `.env` and `data/` from the working directory, so this step changes the
shape of the entrypoint and nothing about where it reads.

**L3 (2026-10-20, implemented)** — `gofmt -l`, `go build ./...`, `go vet ./...`
clean; `go test ./... -count=1` ok in **15 packages** (was 14: `internal/cli` is
new), zero failures, 8 new dispatch tests.

The differential against the `cmd/ingest` binary it replaces, same inputs, same
database copy, comparing stdout and exit code:

| Old | New | Result |
| --- | --- | --- |
| `ingest -list` | `list` | identical |
| `ingest -search figure -limit 3` | `search figure -limit 3` | identical |
| `ingest -pdf … -dry-run` | `convert -pdf … -dry-run` | identical plan |
| `ingest -reindex` (private copy) | `reindex` (private copy) | identical counts |
| `ingest -embed` without a key | `embed` without a key | both exit 1 |
| — | `serve -port 18098` | `200` on `/api/concepts`, `503` on upload without a key, same listen/shutdown log lines as `cmd/server` |

One real defect was found by that differential and fixed before committing: the
flag package stops parsing at the first positional, so `search figure -limit 3`
turned the flag into part of the query and answered with the default limit — a
wrong result that looked normal. `intersperse` reorders the arguments and inserts
the `--` separator, and the dispatch now answers `-h` on stdout.

**Slice boundary, recorded because the second commit is over the ~400-line
guidance:** the `internal/cli` package lands in one commit (~700 lines including
tests). Splitting it would produce a commit whose `commands()` table references
functions that do not exist yet, and a package that does not compile is worse
than a large review than a reviewer can read in file order (`cli.go`, then
`rag.go`, then `convert.go`, then `serve.go`).
