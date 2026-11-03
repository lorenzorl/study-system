# Reading coverage and the study plan (W1 -> W2 -> W3)

Three slices from `backlog.md` §"Designed, not started", built in order on
`feat/reading-coverage-and-plan`: coverage (`W1`), a persisted plan (`W2`), and the
projection that answers *what do I do today* (`W3`), plus its one web surface.

## Specs

**S1 — Coverage is a row per range, with two states.** `reading_progress` stores one
row per `(document_id, from_page, to_page)` with `state` (`read` or `unread`),
`created_at` and `updated_at`. It is keyed to page ranges and **not** to
`document_sections` ids, because sections are derived data rebuilt on reprocess; it is
attached to `documents`, not to `resources`; and it declares no foreign key, like
`range_explanations`, because the two contexts share a file and not a schema.

Backlog, verbatim: *"a `reading_progress` row per `(document_id, from_page, to_page)`
with the two states and a timestamp. Keyed to page ranges and **not** to
`document_sections` ids"*.

**S2 — Marking a range read is a claim, not a proof.** One request marks it. There is
no gate, no second confirmation, no verification of having read, and no scoring.

**S3 — `PUT /api/study/reading` upserts one range.** Body
`{"document_id":"…","from_page":24,"to_page":36,"state":"read"}`, 200 with
`{"document_id":"…","from_page":24,"to_page":36,"state":"read","created_at":"…","updated_at":"…"}`.
Re-marking the same range keeps `created_at` and bumps `updated_at`. `unread` is stored
as a state, not written by deleting the row.

**S4 — `PUT /api/study/reading` refuses with these exact sentences** (`{"error":"…"}`,
status 400): `invalid request body` for a malformed body, `document_id is required`,
`from_page must be at least 1`, `to_page must not be before from_page`, and
`state must be read or unread`.

**S5 — `GET /api/study/reading?document_id=…` lists a document's ranges** as a bare
array ordered by `from_page` then `to_page`, `[]` when there are none; a missing
`document_id` is 400 `document_id is required`.

**S6 — The plan is two tables and the item state is not coverage.** `study_plans`
(`id`, `document_id`, `reading_mode`, `objective`, `source`, `created_at`,
`superseded_at` nullable) and `study_plan_items` (`id`, `plan_id` -> `study_plans` with
`ON DELETE CASCADE`, `position`, `title`, `from_page`, `to_page`, `note`, `state`,
unique `(plan_id, position)`). Item state is `pending` or `skipped` only: *read* is
never stored on the item, it is derived from S1 for the exact range.

**S7 — `POST /api/study/plan` stores what the agent produced.**

```json
{"document_id":"…","reading_mode":"linear","objective":"decidir qué agregado…",
 "source":"sections","items":[{"title":"Capítulo 2","from_page":24,"to_page":36,"note":"…"}]}
```

It writes the plan and its items in one transaction, sets `superseded_at` on the
document's previous active plan and keeps those rows, and answers 201 with the stored
plan: `{"plan_id":"…","document_id":"…","reading_mode":"…","objective":"…","source":"…","created_at":"…","items":[{"position":1,"title":"…","from_page":24,"to_page":36,"note":"…","state":"pending","read":false}]}`.

**S8 — `POST /api/study/plan` refuses with these exact sentences**: 400
`document_id is required`, `reading_mode must be linear, project, survey or consult`,
`items is required`, `item 1: title is required`, `item 1: from_page must be at least 1`,
`item 1: to_page must not be before from_page`,
`item 1: state must be pending or skipped`, `duplicate range 24-36`; 404 `not found` for
a document that does not exist.

**S9 — Item order is the array order.** `position` is assigned 1..N by the server from
the request order; a client-supplied position is ignored.

**S10 — `GET /api/study/plan?document_id=…` returns the active plan** with items
ordered by position and each item's `read` derived from an exact-range `read` row; no
active plan is 404 `not found`.

**S11 — The app stores and projects; it never decides the plan.** It does not classify
the book, reorder items, fill a gap in the list, or require that items cover every
page: a plan is partial and disposable, and a new `POST` replaces it without asking.

**S12 — `skills/study-plan` posts the plan it recommended**, after the reader chose the
first chapter, and its rule *"This skill writes nothing to the server"* becomes *it
writes the plan and nothing else*: no cards, topics or concepts.

**S13 — `GET /api/study/today` is one read-only projection over existing data**, no new
tables:

```json
{"due_count":3,
 "entries":[
  {"kind":"read_not_distilled","document_id":"…","document_title":"DDD","from_page":24,"to_page":36,"read_at":"2026-11-02T10:00:00Z"},
  {"kind":"plan_next","document_id":"…","document_title":"DDD","title":"Capítulo 5","from_page":100,"to_page":118,"note":"…"}]}
```

`read_not_distilled` appears at most once per document — its oldest `read` range with no
`range_explanations` row for the same exact range and no carded gap — and `plan_next`
appears at most once per document: the first `pending` item whose range is not read.
Order: every `read_not_distilled` first (oldest `read_at` first), then `plan_next` by
document title and position. `entries` is `[]`, never `null`. Due reviews are a count,
not an entry: there is no range to name.

**S14 — `/today` is one page and the only new surface.** It shows, in order: the due
count with a link to `/study/review` when it is above zero; each `read_not_distilled`
entry as a sentence naming the material and the range; each `plan_next` entry with its
title, range and note and a `Leído` toggle that calls S3 (`read` on, `unread` off).
Loading, error with `Reintentar`, and nothing-to-do are three distinct states.

**S15 — Documentation moves with the code.** `skills/study-system/references/api.md`
documents the four routes with an example and a refusals table, and the middleware
README's API table gains their rows (and the missing `/sections` row).

## Tasks

| Id | Slices | Route | Commit |
| --- | --- | --- | --- |
| T1 | W1 — S1-S5 | inline | `feat(study): record which page ranges were read` — `40e2338` |
| T2 | W2 — S6-S12 | inline | `feat(study): store the plan the agent produces` — `8761dbe`; independent verifier + native review (high-risk item 1) |
| T3 | W3 — S13, S15 | inline (writer) | `feat(study): project what to do today` — `294ba69` |
| T4 | W3 UI — S14 | inline (writer) | `feat(web): open the app on what to do today` — `959529f` |

## Log

**L1 —** *"revisa el backlog, quiero hacer  W1 → W2 → W3, en una rama nueva"*

**L2 — scope choice, verbatim:** *"Vertical completo, con la pantalla Hoy como
superficie"*, with the preview the user selected:

> W1: tabla reading_progress + GET/PUT de rangos leídos
> W2: study_plans + items, POST desde el skill, plan activo por documento
> W3: GET /api/study/today + página /today
>      - item del plan con toggle «leído»
>      - tarjetas debidas
>      - leído sin destilar → link a explicar el rango
> UI: una sola pantalla nueva

**L3 — decision, distillation evidence.** W4's `range_gaps` states are `open` and
`carded`, and nothing writes `carded` yet (the schema says so itself:
`study/domain/range_explanation.go:46-52`). "Read and not distilled" therefore cannot
key on a carded gap without flagging every read range forever. The evidence used is a
`range_explanations` row for the same exact range: running the explanation *is* the
distillation act of the chosen path. Route A (the agent drafts cards directly) leaves
no range-scoped evidence, so a range distilled that way still reads as undigested until
an explanation exists or it stops being the oldest. Recorded because it is a known
false-positive source, not an oversight.

**L4 — decision, one active plan.** A `POST` supersedes the document's previous plan
instead of deleting it: the reader can throw a plan away without losing what it said,
and the projection has exactly one active plan per document to read. Nothing else in
the plan is versioned.

**L5 — decision, cross-context titles.** The study context cannot join
`documents` (rag), so the projection and the plan's 404 both go through a
`Documents` port that `internal/composition` implements over
`rag.DocumentRepo().FindByID`, the same shape as the existing `RangeText` adapter.

**L6 — slice boundaries, and the branch baseline.** Each task is one commit, and the
three backlogs slices are three reviewable units: `40e2338` (W1), `8761dbe` (W2), and
T3/T4 after them. The branch is stacked on eight unpushed commits of `master`, so a
branch-versus-`origin/master` line count is not the size of these slices.

**L7 — an LSP false positive, recorded so it is not re-diagnosed.** `pi-lens`'s `gopls`
reported `undefined: NewSetReadingProgressHandler` and the new test mocks while
`go build ./...`, `go vet ./...` and `go test ./...` were green: its index had been
built before those files existed and never picked them up. Restarting the server and
re-saving the new files cleared it. Evidence over the index; the compiler is the
authority.

**L8 — independent verification of W2 (`8761dbe`), and what it changed.** Three
findings were real and are fixed in `552fc08`:

- S7's single transaction had no test: swallowing the item-insert error left the whole
  suite green. The new contract test makes the second item fail on a duplicate position
  and asserts the rollback; verified by mutation (with the error swallowed, the test
  fails).
- S8's "nothing is stored" was only asserted against mocks. The contract tests digest
  every plan and item row, send all eleven refusals and assert the digest unchanged.
- S10's ordering test inserted items already in position order, so an `ORDER BY
  from_page` regression would have passed it.
- S12 was self-contradictory: the skill's intro still said "writes nothing to the
  server" while its non-negotiable rule said the plan is the only write.

Three suggestions were accepted as they stand:

- `study_plan_items.state` is `TEXT` with no CHECK, following `ingestion_jobs.status`
  and `range_gaps.state`: the vocabulary grows without a migration, and the HTTP path
  rejects an unknown word.
- No database constraint stops a plan with no items; `SavePlanUseCase` refuses `items:
  []` and no app path can create one. A CHECK cannot express "at least one row in
  another table" without a trigger, which this schema does not use.
- A superseded plan is kept but no route reads it (L4). Recovering one is SQL until a
  screen needs it; the alternative — deleting it — is the moral debt W2 explicitly
  refuses.

**L9 — the review gate.** `gentle_review assess` over `master..HEAD` (51 files, 5,716
lines): risk medium, `reviewDue: false`, reason `under_budget`, writer profile
`large`. Per the entry rule no native review is started: it is not due, and the
independent verifier already covered the slice whose Risk line names item 1. Native
review of a smaller candidate remains available whenever someone asks for one.

**L10 — a live run found the gap language, and it is fixed.** Running
`POST /api/study/explanation` on a real Spanish explanation of `Crunching Knowledge`
(pp. 15-23) against the running server returned six correct gaps **in English**: the
book's text is the bulk of the prompt and nothing asked for the reader's language, so
the model answered in the book's. The prompt now says the `point` is written in the
language of the reader's explanation while the `quote` stays verbatim in the book's,
because a translated quote is no longer evidence. Verified on a second live run: the
same explanation returned its six points in Spanish with the English quotes intact.
A prompt instruction is not the model's behaviour, so the test pins the instruction,
and the live run is the evidence that it lands.
