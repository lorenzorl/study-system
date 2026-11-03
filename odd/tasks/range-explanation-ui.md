# Explain a range, and turn the gaps into cards (UI) — plan

The plan the user asked for after W1-W3 shipped. Nothing here is authorized yet:
two decisions below change the shape, and the specs are what the backlog already
records plus what reading the code made unavoidable.

## What already exists (verified in the code, not assumed)

- `POST /api/study/explanation` works end to end: it stores the reader's text
  verbatim, calls the model synchronously, caps the answer at 7 gaps, refuses a range
  wider than 12 pages or an explanation shorter than 40 characters per page, and
  returns each gap's point, page and quote. Verified on a live run against
  `DDD.pdf` pp. 15-23: six gaps, each with a page and a quote.
- The prompt now asks for the reader's language in `point` and keeps `quote`
  verbatim (commit `6cd47c7`).
- Storage: `range_explanations` + `range_gaps`, gaps in state `open`.

## What is missing before a UI can exist (blockers, found by reading)

1. **A gap has no identifier on the wire.** `ExplainRangeResponse` carries only
   point/page/quote (`internal/study/http/dto.go:162-168`), so a client cannot
   address the gap it wants to accept. The `id` is stored and never exposed.
2. **No route reads a stored explanation.** Only `POST` exists
   (`internal/study/http/router.go:52`), so a reload loses the gaps and the reader
   has to pay for the model again.
3. **Nothing writes `carded` or `discarded`.** The domain names `carded`
   (`domain/range_explanation.go`) and no code path writes it, so the gap list has
   no lifecycle a client can see.
4. **A card needs a concept.** `POST /api/sync/flashcards` requires `concept_id`
   (`apps/go-middleware/internal/study/http/dto.go`, `SyncFlashcardsRequest`), and a
   page range of a book is not a concept. This is the decision below.

## Decisions (answered 2026-11-03)

**D1 — where an accepted gap's card lives: the server.** Decided option A. Options
were:

- **A. A route creates the card.** `POST /api/study/gaps/{gapID}/card` derives the
  topic and concept from the document and stores the card, marking the gap `carded`
  in one transaction. One call from the UI, no naming policy in the client, and the
  gap state cannot disagree with the card. Cost: the server decides naming, so the
  rule has to be written down (something like topic = document title, concept = the
  plan item's title or `pp. 15-23`).
- **B. The UI creates the concept first.** The client calls `POST /api/topics` and
  `POST /api/concepts` on the first accept, then the existing flashcards route. No
  backend change, but the naming rule lives in the client and two of three writes
  can succeed alone.
- **C. The UI only stores and shows.** Accepting a gap copies it and the reader takes
  it to the skill. Cheapest, but the backlog's whole point — "the gaps, then cards
  for the gaps" — becomes manual.

The chosen rule, which the server owns and a client never re-derives:

- the **topic** is the document's title, found or created by name (`UpsertByName`
  already exists and is idempotent);
- the **concept** is the active plan item's title when the explained range is exactly
  that item's range, and `pp. <from>-<to>` otherwise;
- the concept's `file_path` — the column is `NOT NULL UNIQUE`, so a synthetic key is
  required — is `ranges/<document_id>/<from>-<to>`, which makes a second accept of the
  same range reuse the concept instead of failing on the unique constraint;
- the card is written first and `state = 'carded'` last, in that order and for that
  reason: the state is the claim, and a claim written before the card could be true
  of nothing.

**D2 — what happens to the Feynman page: it is replaced.** Decided option A. It is
per concept and its `explain()` returns hardcoded text
(`pages/feynman/ui/FeynmanPage.vue:138-152`). Options were:

- **A. Replace it.** The range explanation is the same feature generalized from
  concept to range, so the placeholder is deleted and `/feynman` redirects to the new
  surface. One explanation surface, and the promise the page already makes becomes
  true.
- **B. Leave it.** Keep the concept page as a stub and add the range page beside it,
  which is two surfaces for one idea and the stub keeps lying.

## Specs (the backlog's W4, quoted, plus the wire the code requires)

**S1 — One range, one explanation, stored verbatim.** Already true. The backlog:
*"the explanation, verbatim — the reader's own words, and re-reading them two weeks
later is itself a retrieval test"*.

**S2 — A gap is addressable and has a state.** The `POST` response carries each gap's
`id` and `state`; `GET /api/study/explanations/{id}` returns the stored explanation
without calling the model; a state change is one request.

**S3 — A gap without a page is rejected.** Already true in the application layer; the
UI never renders one.

**S4 — An explanation too short to judge is refused, not graded.** The page shows the
server's sentence and the threshold (40 characters per page) while the reader types,
instead of failing after a 60-second model call. Backlog: *"explain one section at a
time, or say more"*.

**S5 — A range wider than 12 pages is refused with a sentence naming the split.**
Already true in the application layer; the page can only reach it through a plan item
that is too wide, so it must render the refusal instead of a spinner.

**S6 — Accepting a gap writes a card and marks the gap.** The card's front is the
reader's own question, edited from the gap's point, and the card's back is the quote
with its page: the reader writes the question, the book provides the evidence. The
card is written through the **existing** flashcards path, so the invariants a card has
one owner — the derived `obsidian_id` and the initial `next_review`. The gap's state is
written **last**, so a failure never claims a card that does not exist, and a retry is
idempotent because the card's identity is derived from the concept and the question.

**S7 — Discarding a gap is a state, not a deletion.** The gap stops being actionable
and stays readable.

**S8 — No comprehension score.** Backlog: *"A number on an open response is noisy,
not actionable"*. The page shows gaps, never a grade.

**S10 — The server names the card, and the naming is idempotent.** Topic by the
document's title, concept by the plan item's title or `pp. <from>-<to>`, concept
`file_path` `ranges/<document_id>/<from>-<to>`. Accepting the same gap twice reuses
both instead of duplicating a topic, a concept or a card.

**S9 — The page is reachable, deep-linked and linked from today.** The
`read_not_distilled` entry in `/today` gains the control it deliberately lacks, and
the URL names the document and the range so a reload re-reads the stored explanation
instead of calling the model again.

## Tasks

| Id | Slices | Spec | Route | Commit |
| --- | --- | --- | --- | --- |
| U1 | gap id + state on the wire, `GET /explanations/{id}`, state route | S2, S7 | inline (small) | `feat(study): make a stored gap addressable` |
| U2 | the explanation page: form, refusals, gaps list, stored reload | S4, S5, S8, S9 | writer | `feat(web): explain a range and read the gaps` |
| U3 | accept → card: topic/concept derivation, one transaction, gap state | S6, S10 | writer | `feat(study): accept a gap into a card` |
| U4 | Feynman retirement (`/feynman` → the range surface, placeholder and its hardcoded test gone) and the link from `/today` | S9 | inline | `feat(web): retire the Feynman placeholder` |

U1 must land before U2: a page cannot show an id it never receives. U3 depends on D1's
answer; U4 has no dependency beyond U2.

## Risks

- **Item 1** for U3: it writes a card and a gap state. Card creation already exists
  and is validated; the new part is the derived concept. It gets an independent
  verifier.
- **Item 3** for U1: the `POST /api/study/explanation` response gains fields. Additive
  only, so no client breaks, and that must stay true — no field renamed, no field
  removed.
- Everything else is additive.

## Log

**L1 —** *"Arma un plan para hacer la ui"*, after authorization to merge W1-W3 and fix
the gap language.

**L2 — what reading the code changed about the plan.** The backlog described W4 as
"the explanation and its gaps" and assumed the app could accept a gap. It cannot yet:
the gap `id` never leaves the server, there is no read route, and no code writes
`carded`. Those three are the reason U1 exists as its own slice instead of being part
of the page.

**L3 — the answers.** D1: the server creates the card (option A of the three). D2:
the Feynman placeholder is replaced, not kept beside the range surface. Both were
asked as closed questions with the consequence of each option in view, and both
answers were the recommendation.

**L4 — what the options hid.** Writing the naming rule exposed one column that
decides the shape: `concepts.file_path` is `NOT NULL UNIQUE`, so "the server derives
the concept" is not one lookup — it is a synthetic stable key per range. Without it,
the second accept of the same range fails on the constraint, which is exactly the
kind of thing a plan has to settle before a writer meets it.

**L5 — the card's wording, decided.** The reader edits the front before accepting:
the gap's point is a statement ("the prototype binds model and implementation"), and a
card whose front asserts cannot be recalled, only recognised. The server writes what
the reader accepted — front from the client, back from the quote plus its page — and
the gap's state last, so nothing claims a card that a failed write never stored.
