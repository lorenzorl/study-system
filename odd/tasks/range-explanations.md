# Explain a range, and turn the gaps into cards

**Status:** in progress · **Created:** 2026-11-02 · **Depends on:** backlog `W4` (the
chosen route), the rag pages use case, the OpenRouter client, `platform/db` migrations.

The reader explains a page range in their own words; the app compares that against the
recorded page text and returns what the explanation left out. Each gap carries the page
that states it and a quote, so it becomes a card without anybody guessing.

## Specs

**S1 — The route, chosen by the owner.** From the backlog, `W4`: "vamos por el camino b".
The two routes are not alternatives to weigh again: route A (the agent drafts from the
pages) already exists as `skills/study-system`; **route B is this feature.**

**S2 — The explanation is the reader's, in their own words.** It is stored verbatim: it is
later their own retrieval material.

**S3 — A gap without a page is rejected.** "Cada hueco lleva el punto, la página y una
cita corta. Un hueco sin página se rechaza."

**S4 — No comprehension score.** "no al puntaje": a number on an open response is noisy,
not actionable, and invites writing for the grader. The longitudinal number is the cards
the gaps produced plus the share of reviews passed.

**S5 — The gap list is capped and ranked** (5 to 7). "Contra un capítulo de 28 páginas y
un párrafo, un diff sin tope lista todo, que es lo mismo que no listar nada."

**S6 — An explanation too short to judge is refused, not graded.** "Una explicación
demasiado corta se rechaza, no se califica", with a message that says what to do instead.

**S7 — No card is written without the reader's word.** Accepted gaps become cards through
the existing `POST /api/sync/flashcards`; this feature never writes a card on its own.

**S8 — It belongs to the application, not to an agent.** The endpoint is served by the
server, the tables live in the study schema, and it works with no agent present.

## The contract

`POST /api/study/explanation`

```json
{"document_id": "c6d1e9a3-…", "from_page": 88, "to_page": 114, "text": "…"}
```

`201 Created`

```json
{
  "explanation_id": "…",
  "document_id": "c6d1e9a3-…",
  "from_page": 88,
  "to_page": 114,
  "gaps": [
    {"point": "the boundary is defined by the invariants that must hold in one transaction",
     "page": 89,
     "quote": "Cluster the ENTITIES and VALUE OBJECTS into AGGREGATES …"}
  ]
}
```

#### Refusals

| Case | Response |
| --- | --- |
| `text` missing or blank | `400 {"error":"text is required"}` |
| `text` too short for the range | `400 {"error":"this range is <N> pages and your explanation covers <M> characters: explain one section, or say more"}` |
| range wider than 12 pages | `400 {"error":"this range is <N> pages and explaining a range is limited to 12: explain one section at a time"}` |
| `from_page`/`to_page` out of bounds, or a range above 50 pages | `400`, propagated from the pages use case that already owns those bounds |
| unknown `document_id` | `404 {"error":"not found"}` |
| the range has no recorded page text | `400 {"error":"the range has no recorded text"}` |
| the model fails or times out | `500`, nothing stored |

**Nothing is stored on a refusal.** The explanation row is written only after the gaps come
back, so a refused or failed request leaves the tables exactly as they were.

#### A gap is kept only when its evidence is real

The reader will turn a gap into a card, so a gap is accepted only when all of these hold:

- its `page` lies inside the explained range;
- the `point` carries at least 20 characters and the `quote` at least 15;
- **the `quote` actually appears in the recorded text of the page it cites**, compared after
  lowercasing and collapsing whitespace on both sides. A cited page with no recorded text
  verifies nothing, so its gap is dropped.

A live run stored `{"point":"...","quote":"..."}` because the old rule only checked for
blank text (`L6`). A gap whose evidence is three dots becomes a card with a fake citation,
which is worse than no gap at all.

**Reserved for the next slice, not in v1:** an optional `gaps` field on the same request.
When present, the server uses the gaps the caller computed and makes **no model call**;
validation is identical, which is the whole point. v1 does not expose it (the owner's call
was server -> LLM first), but the use case already takes a resolved gap list, so wiring it
later is a field, a branch and two tests, not a redesign.

## Design decisions

**Where it lives: the study context.** Gaps become cards, and cards are study's. A third
context for one feature would be an architectural statement the feature does not earn.

**Two callers, one validation — the reason the model call is a port.** The use case receives
the range text and a **resolved** gap list; `GapFinder` (the model over OpenRouter) is one
producer of that list. The agent plane will be the other: Pi reads the range itself and
posts the gaps it found through the same endpoint. Neither path may skip the invariants
(page inside the range, quote required, the cap), so the validation is one function used by
both and the model is never inlined into the use case.

**The owner's plan is both callers, built one at a time.** A reader in the web client gets
the server calling the model; an agent gets its own gaps stored. v1 ships the first and
leaves the second to a request field, which is why nothing about the storage or the
validations depends on who computed the gaps.

**The page text crosses a boundary through a port.** `study/app` declares what it needs
(`RangeText`); the adapter lives in `composition` and wraps the rag pages use case, the
same way `domain.DummyFSRS{}` is injected today. `study` never imports `rag/sqlite`, and
the page-range bounds are **not** duplicated: the adapter lets the rag use case's
`apperror.ErrInvalidInput` surface as a 400.

**Migration is study version 3.** `schema_version` is one table shared by both contexts
with disjoint blocks: study owns 1-99, rag owns 100-199 (`VersionBlockStart = 100`). Applies
`range_explanations` and `range_gaps`. `range_gaps.state` is a TEXT column with no CHECK,
following `ingestion_jobs.status`: the vocabulary will grow.

**Synchronous call, with the latency named.** The model call happens in the request. The
client's default timeout is 120 s and Cloudflare's request ceiling is lower, so a 44-page
chapter is the risk case. v1 ships synchronous and records it; if measured latency climbs
past ~30 s the escape is the ingestion pattern (`NewBackgroundProcessor`), not a longer
timeout.

**The model prompt is bounded and the answer is structured.** It receives the range text
and the explanation, and returns JSON gaps. The use case enforces the invariants the model
cannot be trusted with: drop gaps with no page inside the range, cap the list, require a
quote.

**v1 scope: explanation and gaps only.** No `carded` state machine, no dismissal endpoint,
no UI. Turning a gap into a card uses the existing flashcards endpoint. Those are the next
slice, not this one.

## Tasks

| ID | What | Specs | Route | Commit |
| --- | --- | --- | --- | --- |
| T1 | Study migration v3: `range_explanations`, `range_gaps` | S2, S3, S8 | inline | `feat(study)` |
| T2 | `study/domain`: `RangeExplanation`, `RangeGap`, the state vocabulary | S2, S3, S4 | inline | same |
| T3 | `study/app`: `ExplainRangeUseCase` with the ports (`RangeText`, `GapFinder`), a **resolved gap list** as its input after the source is chosen, the shared invariant validation, the cap, the short-text refusal, the no-page rejection | S3-S6 | inline | same |
| T4 | `study/sqlite`: repositories for both tables | S2, S8 | inline | same |
| T5 | `study/http`: handler, DTO, route; the port adapters and wiring in `composition` + `server` | S1, S7, S8 | inline | same |
| T6 | Model adapter: prompt, JSON parse, timeout, and the failure path | S5, S6 | inline | same |
| T7 | Tests, RED first: valid round-trip; gap without page dropped; 12 gaps capped to 7; short text refused **with nothing stored**; range propagated as 400; unknown document 404; model failure stores nothing | S3-S6 | inline | same |

**Test-first, one RED per rule**, and for every existing route that shares the code touched
(the study router, `POST /api/study/review`, `/api/study/due`), one test proving it still
behaves. A differential check against the base commit covers the shared router and the
shared `schema_version` runner.

## Log

**L1 (user, verbatim).** "ya hice el push, por ahora vamos a comenzar con los cambios de
w4, crea una rama nueva para ellos"

**L2 — branch and starting point.** `feat/range-explanations`, cut from `master` at
`d36fb77` with `origin/master` level with it.

**L3 — what the exploration established, before any source write.**

- `composition.NewStudy(sqlDB)` wires repos into use cases; `server.go` builds both contexts
  and mounts `studyRoutes` at `/api/` and `ragRoutes` at `/api/rag/`.
- `composition.Clients` already carries `Model *llm.Client` (default timeout 120 s), so a
  new client is not needed.
- `platform/db.Migrate` keeps **one** `schema_version` table and both contexts write into
  it with disjoint version blocks (rag: 100-199), so the next study migration is `3` and
  must not renumber anything applied.
- `httpx.WriteErrorFrom` maps `apperror` sentinels to status codes and hides anything else
  as a 500, which is why the refusals wrap `ErrInvalidInput` instead of inventing codes.

**L4 — the two callers (owner decision, 2026-11-02).** "quiero tener las 2 opciones … Por
ahora, solo vamos a hacer el camino de servidor -> llm … pero quiero que todo quede de tal
manera que despues implementar el otro sea mas sencillo". Taken as: build server -> LLM
only; put the model behind `GapFinder`; make the use case consume a resolved gap list so the
agent plane becomes an optional request field plus two tests. **Not** built now: the
`gaps` field, and any branch that skips the model.

**L5 — no extra page cap.** The open question was whether to cap a range at ~20 pages for
latency. Decision: rag's 50-page bound is the only one, because a chapter of this book fits
in 44 and capping would force section-sized explanations on pedagogy rather than on measured
need. The latency risk stays named: if the first real call is slower than ~30 s, the cap
gets set with that number in hand.

**L6 — live verification, and the four defects it found.** The contract was run against the
running server with a real explanation of pp. 89-92 (633 characters). What it exposed, in
order of discovery:

1. **The migration never applied on an existing database.** `platform/db` skipped any
   migration whose version was not above `max(version)`, and ingestion's block (100-103) was
   already recorded, so study's 3 was unreachable. Fixed in its own change: a migration is
   skipped by **presence**, not by a maximum, with a test that applies 3 after 100. Without
   it the endpoint would have spent a model call and then failed on a missing table.
2. **The model answered prose, and the adapter only tolerated a code fence.**
   `invalid character 'T' looking for beginning of value`, then `'N'`: the model wrote
   sentences, not JSON. Fixed: locate the first balanced JSON object or array (quotes
   tracked), accept a bare array, include the answer excerpt in the parse error, and retry
   **once** with a corrective turn before failing. Six adapter tests cover it.
3. **The invariants accepted a placeholder.** The model returned
   `{"point":"...","quote":"..."}` and `AcceptGaps` stored it: it checks that point and
   quote are not blank, never that they carry substance, and it never checks the quote
   against the page the gap cites. A gap whose evidence is `"..."` is worse than no gap: it
   becomes a card with a fake citation. **Open decision**, because both fixes change the
   contract: (a) minimum lengths for point and quote, or (b) minimum lengths **and** the
   quote must appear on the cited page's recorded text — which is the only invariant that
   makes a citation trustworthy, and it requires passing the pages into `AcceptGaps`, so the
   agent path gets the same check. The row produced by this run was deleted after
   inspection.
4. **Latency: 61 seconds for a 4-page range** (36 s for the first answer plus 25 s for the
   corrective retry). The risk named in L5 materialised at a quarter of a chapter: a 44-page
   range would sit past the edge proxy's ceiling. The escape is the range cap or the ingestion
   background pattern, and the number is now measured rather than assumed.

**L7 — the owner's call on both open defects (2026-11-02).** "ve por fuerte y facil".

- **Fuerte, for the placeholder gaps:** minimum lengths _and_ the literal-quote check. The
  quote must appear on the page it cites, which is the only invariant that makes a citation
  trustworthy; because the check lives in `AcceptGaps`, the agent plane will receive it
  unchanged. The cheap alternative — lengths only — was rejected: it stops `"..."` and not a
  plausible invented quote.
- **Facil, for the latency:** `MaxExplainRangePages = 12`, refused with a sentence. 61 seconds
  for four pages means a chapter cannot be answered in one synchronous request, and building
  the background pattern now would trade a measured constraint for an unbuilt one. The
  background route stays open in `W4`'s entry and in the design decisions above.

**L8 — the root cause of the placeholder gaps, and what the invariants cost.** After L7 the
endpoint answered `gaps: []` on every live attempt, including a one-page range explained in
one sentence, where the model certainly had something to say. Diagnosed by replaying the
prompt against OpenRouter directly:

1. **The model is a reasoning model.** Its content begins with `Thinking Process:` and, inside
   that thinking, it restates the answer format: `{"gaps":[{"point":"...","quote":"..."}]}`.
2. **The parser took the first JSON in the text**, so it read that template. That is where the
   stored `point="..."` and `quote="..."` came from, and it is why the L7 invariant then
   dropped every gap — a template with three dots fails both the minimum length and the
   literal-quote check — leaving an empty list that read as "you left nothing out".
3. **Fixed by taking the last balanced JSON**, with a test that puts the template before the
   real answer. Live re-run, one page: **7 gaps, and 7 of 7 quotes found verbatim on p. 89**,
   checked independently against the page the API returns.

**Measured latency, for the record:** 12 pages took 89.5 s, 1 page 33-38 s, and the 13-page
refusal 0.6 ms (it never reaches the model). The cap is the right shape but its edge is still
slow; the background pattern is what a used feature needs, and it is now a number rather than
an opinion.

**Known limit, not fixed:** `gaps: []` still cannot be told apart from "the model proposed
gaps and every one was dropped". After this fix the common case returns real gaps, so the
ambiguity is rare rather than permanent, but a `dropped` count in the response is what would
close it. Recorded here instead of guessed at.

**L9 — commit slice boundary.** Two commits on `feat/range-explanations`, not pushed:

| Commit | Unit | Authored lines |
| --- | --- | --- |
| `fix(db)` | the migration runner, by presence instead of maximum | 91 |
| `feat(study)` | the endpoint, its tables, the four live-verification fixes, the tests and this document | 2895 |

The second is far past the ~400-line review unit. It stays one commit on purpose: the
feature, the adapter hardening and the invariants live in the same files, and splitting them
would mean staging hunks by hand in `range_explanations.go` and `explain_range.go`. A pull
request over this branch should say so instead of pretending the slice is small.
