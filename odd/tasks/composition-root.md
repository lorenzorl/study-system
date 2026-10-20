# Step 7 — Shared composition root

**Status:** done
**Created:** 2026-10-20
**Depends on:** step 0-6 (every package it wires), `platform/config`, `platform/db`

## Specs

**S1 — The request.** "Quiero que me digas si podemos agregar que tambien se pueda
usar como una cli y que se pueda hacer una compilacion para ejecutarlo como cli"

**S2 — Stage 1 of that answer (user, 2026-10-20).** The user accepted this
definition, quoted from the proposal: "extraer `internal/composition` y hacer que
`server` e `ingest` la usen. Puro refactor, cero cambio de comportamiento, los 13
paquetes de tests son la red. Es el único paso que toca lo que hoy funciona, y es
el que hace barato todo lo demás."

**S3 — Constraints inherited from the system.** No new dependencies. The two
commands keep their exact behaviour: same flags, same modes, same error messages,
same startup order. The server still boots with no API key, and the upload still
answers 503 in that case. `cmd/ingest` keeps opening read-only for `-search` and
the in-memory fallback for `-dry-run`. The 13 tested packages stay green.

## Why

The wiring lives inline in two `main()` functions and is duplicated where it
matters:

- `slices.Concat(studysqlite.Migrations(), ragsqlite.Migrations())` appears twice.
- The database open/migrate rules appear twice, in two different shapes
  (`cmd/server` inline; `cmd/ingest` behind `openDatabase` plus `readOnlyDSN`).
- The "hybrid when there is a key, lexical when there is not" search rule appears
  twice.
- Model client construction appears twice.

A third entrypoint (stage 2's `cmd/cli`, stage 3's study commands) would be a
third copy. The line count barely moves in this step; what changes is that the
wiring has one definition instead of one per process.

## Delivered

- `internal/composition`: `Migrations`, `OpenDatabase`, `OpenReadOnlyDatabase`,
  `OpenPlanningDatabase`, `Clients`/`NewClients`, `Study`/`NewStudy`,
  `RAG`/`NewRAG` and its factories.
- `cmd/server` and `cmd/ingest` build their contexts through the package.
- Tests for the package against a real migrated in-memory database: the
  composition had none before, and the two `main()` functions still have none.

## Design: composition builds, the entrypoint mounts

The package follows one rule, stated in its doc comment: it turns configuration
and a database into application objects, and it never mounts a transport. Each
entrypoint keeps its own routers, its own `http.Server` or flag parsing, and its
own policy.

That rule is what keeps the two contexts' asymmetry out of the package. The study
router takes exactly its use cases, while the RAG router needs the upload handler,
which needs the store and the "is a key configured" flag — entrypoint policy. Both
routers stay where they are.

Two hazards the design closes rather than documents:

- **Typed-nil interfaces.** A nil `*llm.EmbeddingClient` stored in a
  `domain.Embedder` interface is not `== nil`, so a `nil` check inside a use case
  would pass and the first call would panic. The factories take `Clients`, a
  struct with concrete pointers, and decide on the field. There is no interface
  through which a nil client can arrive.
- **A missing key is not a missing client.** `NewClients` returns the zero
  `Clients` when no key is configured, so "no key" is a value the callers branch
  on, not an error they have to translate. `NewClient` itself still rejects a
  blank key, because that is a programming error at the call site.

`OpenPlanningDatabase` keeps the dry-run rule in one place: read-only when the
file exists, and an empty migrated in-memory schema when it does not, which
answers every lookup with "not found" and writes nothing to disk.

## Tasks

- [x] T1 — `internal/composition`: `Migrations`, the three database openers, and
      `Clients`/`NewClients`, with tests (S3)
- [x] T2 — `internal/composition`: `Study`, `RAG`, and the RAG factories, with
      tests (S3)
- [x] T3 — `cmd/server`: build through composition, identical behaviour (S2, S3)
- [x] T4 — `cmd/ingest`: build through composition, identical behaviour (S2, S3)
- [x] T5 — README, verification, and the work-unit commits (S2)

## Commits

The slice boundary is recorded here because the first commit is larger than one
review sitting, and the feature document is where the reason lives.

| Commit | Holds | Risk |
| --- | --- | --- |
| `078024d` | `internal/composition` and its suite, plus this document. The package is unused by the binaries at this point: it compiles and its tests pass on its own. | none |
| `a07e9a4` | `cmd/server`, now built through composition. | item 6 |
| `1e64d74` | `cmd/ingest`, all five modes, now built through composition. | item 6 |
| `ce6c9e2` | the README section and the layout entry. | none |

Risk item 6 is named on the two command commits because nothing in the suite
executes a `main()`: a wiring mistake would compile and pass every test. That is
what the base-commit differential in L4 exists to cover, and what an independent
verification re-ran.

## Review

RDD is enabled, so the range went through the native review (lineage
`review-aba59c80a4f6dc09`, tier medium, 1,233 changed lines) with three lenses:
risk, reliability, readability.

The first capture admitted `review-risk` and `review-readability` and refused
`review-reliability` **at admission**: the emitted payload opened and closed the
same number of objects and arrays but ended at byte 7,978 without closing the
document. The refused bytes were not resubmitted. A fresh status reoffered that
one slot, it ran once more, and that result was admitted.

**Outcome: approved, authority burned** (`gentle-ai.review-acknowledged/v1`,
revision `sha256:49bb69f6…`). Six findings, all non-blocking: none opened a
correction, none reopens the candidate, and no correction transition was offered
for it. They are recorded in [`backlog.md`](backlog.md) as `C3`.

## Findings

| Id | Lens | Severity | Location |
| --- | --- | --- | --- |
| `R2-001` | readability | WARNING | `internal/composition/rag.go:113-120` |
| `R2-004` | readability | WARNING | `internal/composition/composition.go:78-87` |
| `R3-unused-client-construction` | reliability | WARNING | `internal/composition/clients.go:37-55` |
| `R2-002` | readability | SUGGESTION | `internal/composition/composition_test.go:206` |
| `R2-003` | readability | SUGGESTION | `internal/composition/rag.go:56` |
| `R3-ingest-factory-untested` | reliability | SUGGESTION | `internal/composition/rag.go:63-86` |

The review returned identifiers, lenses, severities, and locations — not the
findings' prose — so this table claims nothing about their content. Two of them
duplicate what the independent verification had already reported: the unused chat
client in `NewClients`, and `Ingest` having no test of its own.

**All six were addressed in the same session**, before the follow-up work that
would have touched the same lines. [`backlog.md`](backlog.md) records what each one
changed, under `C3 (closed)`, together with the caveat that four of them were read
off their line and interpreted.

## Log

**L1 (2026-10-20, request)** — "Quiero que me digas si podemos agregar que tambien
se pueda usar como una cli y que se pueda hacer una compilacion para ejecutarlo
como cli"

**L2 (2026-10-20, answer)** — the parent reviewed the server and found that
`cmd/ingest` is already a CLI with 9 flags and 5 modes, and that
`CGO_ENABLED=0` cross-compiles to static 10 MB binaries for linux/amd64,
darwin/arm64 and windows/amd64 (`file`: "statically linked, stripped"), verified
by building all three and running `-list` against the real store. The gap named
was `internal/composition`: the composition root is inline in two `main()`
functions, so a third entrypoint would duplicate the wiring and the read-only,
in-memory and lexical-vs-hybrid rules. Three options were offered (one binary
with subcommands, a separate `cmd/study-cli`, or nothing); the recommendation was
one binary, delivered in four stages, with the extraction first.

**L3 (2026-10-20, authorization)** — "si ve con la etapa 1", referring to L2's
stage 1: extract `internal/composition` and have `server` and `ingest` use it, as
a pure refactor with no behaviour change (S2).

**L4 (2026-10-20, implemented and diffed against `master`)** — `gofmt -l`,
`go build ./...`, `go vet ./...` clean; `go test ./... -count=1` ok in every
package, with 14 new tests in `internal/composition`.

The refactor was diffed against `6e5eb99` (a worktree of `master`, both binaries
built from each side) rather than judged by reading it:

| Command | Result |
| --- | --- |
| `-list` (real store) | stdout, stderr, exit byte-identical |
| `-search "figure" -limit 5` (real index, no key) | identical, 5 lexical hits |
| `-search` against a missing database | identical error and exit 1 |
| no arguments, and `-pdf` without `-out` | identical apart from `argv[0]` |
| `-pdf … -dry-run` | identical plan line: `3 pages: 0 already recorded, 0 from the text layer, 3 to the model (~4m)` |
| `-reindex` on a private copy | identical: `reindexed 3 documents, 1180 chunks` |
| server: `GET /api/concepts`, `/api/study/due`, `/api/rag/documents`, `/api/rag/search`, an unknown path | identical bodies and status codes |
| server: upload with no key | identical `503` and message |
| server: `OPTIONS` for an allowed and a foreign origin | identical headers |
| server: startup and SIGTERM shutdown log | identical apart from the timestamp |

The only textual differences across the whole comparison are timestamps and the
binary name in `flag` usage.

Two hazards were closed instead of documented. A nil `*llm.EmbeddingClient` stored
in a `domain.Embedder` — and a nil `*BackgroundProcessor` stored in a
`FailureReporter` — is not `== nil`, so a use case's own nil check would pass and
the first call would panic. Both factories take the concrete type (`Clients`, and
the processor in `RAG.Status`) and narrow to the interface themselves, so "no
credential" and "no processor" keep meaning what they say.

**L5 (2026-10-20, independent verification, one defect fixed)** — a
`gentle-ai-verify` run reproduced the build health, confirmed `go.mod` and
`go.sum` are untouched across the range, and re-ran the differential itself on a
byte-identical copy of `data/` rather than trusting L4: six CLI comparisons and
fifteen HTTP probes, including all eight study POST routes, so no handler could
have received the wrong use case. It returned **S2 holds, S3 holds**, and raised
one real defect against L4's claim.

`RAG.Embed` passed `clients.Embedding` straight into a `domain.Embedder` with no
narrowing, so a zero `Clients` produced a typed-nil — exactly the hazard the
package says it removes. It was unreachable (`-embed` checks the key first, and
the server builds `Process` only when the client exists), but a guarantee that is
only true by accident is not a guarantee. `Embed` and `Process` now **refuse**
with an error naming `STUDY_OPENROUTER_API_KEY` instead of narrowing: there is no
lexical fallback for writing a vector, so an embedder-less use case would fail
deep inside a run as a nil dereference rather than at the call that built it. One
test added; the command's own message and exit code are unchanged.

Two further findings were accepted, not fixed. Composition's search test only
proves the lexical path does not panic on an empty corpus; that the hybrid and
lexical branches actually differ is pinned in `rag/app/search_test.go`, where the
behaviour lives. And `-search`/`-embed` now also construct the chat client they
do not use: unobservable, because `config.load` defaults and trims both models and
validates the base URL before any client is built.

**L6 (2026-10-20, the six findings closed in the same session)** — the review's
six advisory findings were all addressed, before the follow-up work that would
have touched the same lines. The search-test gap L5 accepted stays accepted: it
is pinned where the behaviour lives. What changed is documented one by one in
`backlog.md` under `C3 (closed)`.

The one with real design content was the unused client, which is L5's accepted
finding turned into work. `NewClients` split into three role constructors over one
`buildClients` helper: `NewClients` for the server, `NewEmbeddingClients` for
`-search` and `-embed`, `NewTranscriptionClients` for the conversion run — which
never embeds and was paying for an embedding client in the other direction, a
waste the review did not name but the same class. Why the split is safe: a command
wired to the wrong constructor fails loudly, because `Embed` refuses a zero client
with an error naming the variable, rather than silently doing nothing.

The four readability findings were each read off their line and interpreted, so
what was changed is written in `C3 (closed)` and not only in the commit.

18 tests in the package now (was 15). The study wiring test exercises all eight
use cases instead of four: it syncs two cards, sees both due, submits a grade-3
review, and sees the due list shrink to one. `gofmt`, `go build`, `go vet` clean;
`go test ./... -count=1` ok in all 14 packages. The `-search` output was compared
against L4's recorded baseline and matches it hit for hit, and `-embed` without a
key still prints the command's own message and exits 1.


