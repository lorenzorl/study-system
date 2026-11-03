# The API this skill uses

The endpoints this skill uses, all under `$STUDY_BASE_URL`. Every request carrying
the Cloudflare Access service token needs these headers:

```bash
-H "CF-Access-Client-Id: $STUDY_ACCESS_CLIENT_ID"
-H "CF-Access-Client-Secret: $STUDY_ACCESS_CLIENT_SECRET"
```

If the server is not behind Cloudflare Access (a tailnet, a LAN address), the two
headers are simply absent and the rest of this document is unchanged.

Errors are always `{"error":"<sentence>"}`. Read the sentence; it names the bound
that was missed.

## GET /api/rag/documents

The corpus. Use it to resolve a document id and see how many pages it has.

```json
[{"id":"c506af9d-…","title":"DDD.pdf","created_at":"2026-10-17T22:16:54Z","pages":359,"chunks":1045}]
```

An empty corpus is `[]`.

## GET /api/rag/documents/{id}/sections

The structure the ingestion recovered and stored for that document: one entry per
part, chapter and section, with the page range each one owns. Use it instead of
reassembling a printed table of contents, which arrives mangled by dot leaders.

No query parameters. No cap: a 359-page book has 110 entries.

```json
[
  {"id":"6f9d…","parent_id":"","level":1,"title":"Part I: Putting the Domain Model to Work","start_page":3,"end_page":120,"source":"toc"},
  {"id":"b21c…","parent_id":"6f9d…","level":2,"title":"Crunching Knowledge","start_page":3,"end_page":40,"source":"mixed"}
]
```

- The array is ascending by `start_page`, and a part sorts before the chapter that
  opens on the same page.
- `parent_id` is always present and empty for a top-level entry: the tree is rebuilt
  from it.
- `level` is the document's own nesting, not a fixed vocabulary. 1 is a part, 2 a
  chapter, 3 and beyond a section; a document with no parts starts at 1.
- `source` says how the entry was recovered: `outline` (the PDF's own bookmarks),
  `toc` (the printed contents), `typography`, `pattern` (a numbered heading), or
  `mixed` when one source confirmed another. `toc` and `mixed` are the confident ones.
- **An empty array is normal**, not an error: it means no structure was recovered —
  a novel, an essay, or a scan whose headings the cascade could not find. Fall back
  to the printed contents and say which one the plan used.

Refusals:

| Case | Response |
| --- | --- |
| Unknown document | `404 {"error":"not found"}` |
| Document with no recovered structure | `200 []` |

## GET /api/rag/documents/{id}/pages?from=N&to=M

The text the ingestion run recorded for those pages. This is the material a card is
drafted from, and the thing a citation points at.

Both parameters are required, `from >= 1`, `to >= from`, at most **50 pages** per
call, and `to` may not pass the document's last page.

```json
{
  "document_id": "c506af9d-…",
  "title": "DDD.pdf",
  "total_pages": 359,
  "pages": [
    {"page_number": 25, "status": "extracted", "method": "text", "text": "UBIQUITOUS LANGUAGE …"},
    {"page_number": 26, "status": "transcribed", "method": "model", "text": "…"}
  ]
}
```

- `pages` is ascending and contains **only** pages the run recorded. A missing page
  is a page the run never finished: say so instead of inventing content for it.
- `status` is `extracted`, `transcribed` or `failed`. A `failed` page has an empty
  `text` and an `error` explaining why.
- `transcribed` means a model described the page, which for a page with a diagram is
  a description and not the author's words. Cite it as what it is.

Refusals:

| Case | Response |
| --- | --- |
| `from` or `to` missing | `400 "from is required"` |
| `from=0` | `400 "from must be at least 1"` |
| `from=10&to=5` | `400 "to must be at least from (10)"` |
| 51 pages or more | `400 "at most 50 pages can be requested per call"` |
| `to` past the end | `400 "to must be at most 359, the document's last page"` |
| Unknown document | `404 {"error":"not found"}` |

## GET /api/concepts

The topic → concept tree, which is where a card's destination lives.

```json
[{"id":"887ee4cd-…","name":"DDD","concepts":[{"id":"fde8c096-…","title":"Ubiquitous Language","file_path":"manual/DDD/Ubiquitous Language.md"}]}]
```

## POST /api/concepts

Only when the user asked for a new destination.

```json
{"topic_id":"887ee4cd-…","title":"Ubiquitous Language"}
```

## POST /api/sync/flashcards

Writes cards into one concept. `obsidian_id` is **optional**: when it is absent the
server derives it from the concept and the question, so the same question written
twice updates one card instead of adding a second. Omit it.

```json
{
  "concept_id": "fde8c096-…",
  "cards": [
    {"question": "¿Qué es el lenguaje ubicuo?", "answer": "Un lenguaje compartido …"}
  ]
}
```

```json
{"synced": 1}
```

- An empty question or an empty answer is `400 {"error":"invalid input"}` and writes
  nothing. That validation applies to cards written **without** an id.
- A card with an explicit `obsidian_id` keeps it: that is the note-sync path, and
  this skill does not use it.
- A newly written card gets an initial card state with `next_review` set to now, so
  it is due immediately.

## GET /api/study/due

What the user has to review today. Useful for a closing line ("quedan N para hoy"),
never for choosing what to propose.

```json
[{"flashcard_id":"b64958c5-…","question":"…","answer":"…","concept_title":"Aggregates","topic_name":"DDD","next_review":"2026-10-23T15:11:49Z"}]
```

## PUT /api/study/reading

Mark one page range read or unread. This is the coverage axis, the one beside
retrieval: a card is attempted and graded, while a range is only a position —
where the reader is inside the book.

```json
{"document_id":"0f0b6b0e-…","from_page":24,"to_page":36,"state":"read"}
```

- The unit is the **range the plan recommends**, not the page. A page counter turns
  a chapter into `2/13`, which is neither read nor not read.
- `state` is `read` or `unread`. Marking `unread` keeps the row: it is a state, not
  a deletion, and "not read" stays different from "never looked at".
- Marking the same range again is one row: `created_at` stays, `updated_at` moves.
- **A mark is a claim, not a proof.** Nothing verifies the reader read the pages,
  and nothing should.

`200` answers the stored row:

```json
{"document_id":"0f0b6b0e-…","from_page":24,"to_page":36,"state":"read","created_at":"2026-11-02T10:00:00Z","updated_at":"2026-11-02T10:00:00Z"}
```

| Case | Response |
| --- | --- |
| body is not JSON | `400 invalid request body` |
| `document_id` missing or blank | `400 document_id is required` |
| `from_page` below 1 | `400 from_page must be at least 1` |
| `to_page` before `from_page` | `400 to_page must not be before from_page` |
| `state` is neither `read` nor `unread` | `400 state must be read or unread` |

## GET /api/study/reading?document_id=…

The ranges marked on one document, in page order, `[]` when none is marked.

```json
[{"document_id":"0f0b6b0e-…","from_page":24,"to_page":36,"state":"read","created_at":"2026-11-02T10:00:00Z","updated_at":"2026-11-02T10:00:00Z"}]
```

| Case | Response |
| --- | --- |
| `document_id` missing or blank | `400 document_id is required` |

## POST /api/study/plan

Store the plan you recommended. This is the one write the planning skill makes: the
app stores the plan and projects it, and never classifies a book itself.

```json
{"document_id":"0f0b6b0e-…","reading_mode":"linear","objective":"decidir qué agregado va en qué contexto","source":"sections","items":[{"title":"Capítulo 2","from_page":24,"to_page":36,"note":"primero: nombra el vocabulario"}]}
```

- `reading_mode` is `linear`, `project`, `survey` or `consult`.
- `source` is `sections`, `printed-contents` or `none`: the skeleton the ranges came
  from.
- `items` is in the order you recommend reading; the server keeps that order and
  numbers the items 1..N. A position you send is ignored.
- `state` is `pending` or `skipped`; omit it and the item is `pending`.
- A stored plan **supersedes** the document's previous active plan. The old plan and
  its items are kept, so throwing a plan away never loses what it said.
- `read` in the answer is derived from coverage for the exact range; it is never
  stored on the item.

`201` answers the stored plan:

```json
{"plan_id":"3f1c…","document_id":"0f0b6b0e-…","reading_mode":"linear","objective":"decidir qué agregado va en qué contexto","source":"sections","created_at":"2026-11-03T09:00:00Z","items":[{"position":1,"title":"Capítulo 2","from_page":24,"to_page":36,"note":"primero: nombra el vocabulario","state":"pending","read":false}]}
```

| Case | Response |
| --- | --- |
| body is not JSON | `400 invalid request body` |
| `document_id` missing or blank | `400 document_id is required` |
| `reading_mode` missing or unknown | `400 reading_mode must be linear, project, survey or consult` |
| `items` missing or empty | `400 items is required` |
| an item has no title | `400 item 1: title is required` |
| an item starts below page 1 | `400 item 1: from_page must be at least 1` |
| an item's range is backwards | `400 item 1: to_page must not be before from_page` |
| an item's state is neither `pending` nor `skipped` | `400 item 1: state must be pending or skipped` |
| two items name the same range | `400 duplicate range 24-36` |
| the document is not in the corpus | `404 not found` |

Nothing is stored on a refusal, so a corrected plan starts from a clean slate.

## GET /api/study/plan?document_id=…

The document's active plan, with each item's derived `read` flag. The shape is the
one `POST` answers.

| Case | Response |
| --- | --- |
| `document_id` missing or blank | `400 document_id is required` |
| the document has no active plan | `404 not found` |

## GET /api/study/today

What to do today, across every material: the due count, then the ranges marked read
that nobody has explained, then the next pending chapter of each active plan.

No query parameters: the day is the question, and it covers the whole corpus.

```json
{"due_count":3,
 "entries":[
  {"kind":"read_not_distilled","document_id":"0f0b6b0e-…","document_title":"DDD","from_page":24,"to_page":36,"read_at":"2026-11-02T10:00:00Z"},
  {"kind":"plan_next","document_id":"0f0b6b0e-…","document_title":"DDD","title":"Capítulo 5","from_page":100,"to_page":118,"note":"volver si hace falta"}]}
```

`entries` is `[]`, never `null`. There are two kinds, and each one appears **at most
once per document**:

- `read_not_distilled` — a range marked `read` with no explanation of the **same
  exact range**. The one named is that document's oldest such reading, and `read_at`
  is when that range was last marked read. It carries no `title` and no `note`: a
  reading is not a plan item.
- `plan_next` — the first `pending` item, in position order, of the document's active
  plan whose exact range is not marked read. A `skipped` item is never named, and an
  item whose range is read is already done.

Order: every `read_not_distilled` first, oldest `read_at` first (ties by document
title, then document id); then every `plan_next`, by document title and position.

- Ranges match **exactly, never by overlap**: a read row for 24-35 neither marks the
  item 24-36 read nor distils it.
- Due reviews are the number `due_count`, not an entry: a review has no page range to
  name. It counts what `GET /api/study/due` lists, so show the count with a link to
  `/study/review` when it is above zero.
- The route writes nothing. It projects what `PUT /api/study/reading`,
  `POST /api/study/plan` and `POST /api/study/explanation` already stored.
- A `document_title` that cannot be resolved is `""` and the entry stays: the
  `document_id` is what the action needs.

| Case | Response |
| --- | --- |
| nothing read, nothing planned, nothing due | `200 {"due_count":0,"entries":[]}` |
| a material whose title is no longer in the corpus | `200` with `"document_title":""` |
