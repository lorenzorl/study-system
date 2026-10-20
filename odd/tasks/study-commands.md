# Step 10 — The study context on the command line (CLI, stage 3)

**Status:** done
**Created:** 2026-10-20
**Depends on:** step 8 (`internal/cli`), step 9 (the data root), step 7
(`composition.NewStudy`)

## Specs

**S1 — The request.** "haz la siguiente fase" — stage 3 of the plan the user
accepted: "Etapa 3 — comandos de `study` (`due`, `review`, `topics`, `concepts`)."

**S2 — Constraints inherited from the system.** No new dependencies. The study use
cases are the ones the server already mounts: this step adds a command surface, not
a rule. The HTTP API is unchanged. Read commands must not create a database.

## Why

`study` was the one context with no command surface. Everything it knows — what is
due, what a topic holds, what a review did to the schedule — was reachable only
over HTTP, which means only from the plugin. Stage 2 gave the project a command
surface; this is the half of the domain that was still missing from it.

## Design: nothing new below the transport

Each command is a flag set and a print loop over the use case the server calls:

| Command | Use case | Opens |
| --- | --- | --- |
| `due` | `GetDueCards` | read-only |
| `review <id> -grade N` | `SubmitReview` | read-write |
| `topics` | `ListConcepts` (the tree) | read-only |
| `concepts [topic]` | `ListConcepts`, filtered by name or id | read-only |

Three decisions worth stating:

- **The read commands open read-only**, so they cannot create a database as a side
  effect of looking. Answering "0 cards due" for a file that is not there is the
  same lie `search` refuses to tell.
- **A topic name that matches nothing is an error, not an empty list**, and the
  check happens before anything is printed, so a typo never leaves a table header
  above an error. A topic that *exists* with no concepts is a different case and
  answers zero.
- **The output is the same shape as `search`**: a table when there are rows, then a
  count line. Scripts get a stable last line.

## Tasks

- [x] T1 — `due` and `review`, the read and the one write (S1)
- [x] T2 — `topics` and `concepts`, with name/id filtering (S1)
- [x] T3 — tests through `Run` against real migrated databases (S2)
- [x] T4 — README, verification on the real database, commit (S2)

## Log

**L1 (2026-10-20, implemented)** — `gofmt -l`, `go build ./...`, `go vet ./...`
clean; `go test ./... -count=1` ok in 15 packages, zero failures, 8 new tests.

The tests build a migrated database under a temporary root with
`composition.NewStudy`, seed a topic, a concept and a flashcard through the same use
cases the server uses, and then drive the command surface through `Run`. So the
wiring, the flags, the output and the state change are all exercised rather than
mocked: `review` schedules the card out of the `due` list, a grade outside 1-4 is
rejected by the use case's own rule, and a misspelled topic is refused.

Verified by running the built binary against the repository's real database:

| Command | Output |
| --- | --- |
| `topics` | the three topics with their concept counts, then `3 topics` |
| `concepts DDD` | `Aggregates` with `manual/DDD/Aggregates.md`, then `1 concepts` |
| `concepts` | all three concepts, then `3 concepts` |
| `due` | `0 cards due` |
| `concepts nope` | `concepts: no topic named "nope"`, exit 1, **no header on stdout** |

One defect was found by that last check and fixed before committing: the header row
was printed before the "no such topic" error was returned, so an error left an empty
table on stdout. The command now collects the rows first.

**Risk: none** — every new path is exercised through `Run` by the suite, and the
entrypoint, the server and the HTTP API are untouched.
