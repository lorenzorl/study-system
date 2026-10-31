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
