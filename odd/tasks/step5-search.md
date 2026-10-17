# Step 5 — Search over the ingested corpus

**Status:** in progress
**Created:** 2026-10-17
**Depends on:** step 4 (`documents`, `ingestion_jobs`, `job_pages`, and the page text
they hold), `platform/db` migrations, `platform/llm` for the embedding stage.

## Specs

Decisions taken by the user on 2026-10-17, quoted verbatim. Where a spec quotes a
choice, the implementation may not quietly re-decide it.

**S1 — Language.** "las consultas del usuario van a ser siempre en espanol, y los
libros van a estar en espanol e ingles".

Consequence, and it drives the whole step: a lexical index answers a Spanish query
against a Spanish book natively, and against an English book only when the query
shares the book's vocabulary (technical terms, proper nouns, code, identifiers).
The semantic stage is therefore not an optional improvement: it is the only thing
that crosses the language boundary, and the embedding model must be multilingual.
Which means: the lexical stage is a real, usable step, but no one should conclude
from it that search is "done".

**S2 — Order.** "Chunking + FTS5 primero, embeddings después".

**S3 — Surface.** "Un comando CLI primero". The HTTP surface for the plugin is a
later slice. No plugin UI: that was decided in step 4 and has not changed.

**S4 — Chunk size.** "≈300 tokens con 15% de solape". Cut on paragraph boundaries,
never across a page boundary, so every chunk cites exactly one page.

**S5 — Measured facts this step builds on** (verified, not assumed):

- The pure-Go driver carries SQLite 3.53.2 with **FTS5**, `bm25()` ranking, and the
  `unicode61 remove_diacritics 2` tokenizer. Verified through the module's own
  driver, not the system `sqlite3`.
- That tokenizer folds accents **in both directions**: `obstaculo` finds
  "obstáculo", `diseno` finds "diseño", `OBSTACULO` finds it too. Case-insensitive,
  and `AND` / `"quoted phrase"` work.
- OpenRouter serves embeddings with the **same key** already configured:
  `POST /api/v1/embeddings`, catalogue at `GET /api/v1/embeddings/models` (33
  models). Prices per token: `voyage-4-lite` US$ 0,00000002 (US$ 0,02 per million),
  `voyage-4` 0,06, `gemini-embedding-2` 0,2, and two free models.
- The corpus ingested so far is 33 pages / 116.068 characters ≈ 29k tokens, about
  880 tokens per page. A 359-page book is therefore ≈ 316k tokens, which costs
  under one cent to embed at `voyage-4-lite` and produces ≈ 4 MB of vectors at
  1024 dimensions. Brute-force cosine over a thousand chunks is milliseconds.
- There is **no local embedding runtime** on the machine (no ollama): the semantic
  stage goes through an API.

## Tasks

### Slice 5.1 — the index, the chunker, and lexical search

- [x] T1 — Migration 101: `chunks` (the source of truth, with document, job, page,
      ordinal) and `chunks_fts` (FTS5 over the chunk text, `chunk_id` unindexed)
- [x] T2 — `rag/domain`: `ChunkPage` as a pure function, with its options and its
      boundary rules
- [x] T3 — `rag/sqlite`: the chunk repository — replace-all-per-document, insert in
      one transaction, and a ranked search that joins back for the citation
- [x] T4 — `rag/app`: `IndexDocument` (rebuild a document's chunks from its recorded
      pages) and `Search` (query in, ranked chunks with document and page out)
- [x] T5 — `cmd/ingest`: `-search` with a limit, printing snippet + document + page +
      score, and `-reindex` for rebuilding the whole index
- [x] T6 — Tests, README, backlog, commit

### Slice 5.2 — embeddings and hybrid retrieval

- [ ] T7 — `platform/llm`: `Embed`, and a multilingual model chosen from measured
      evidence (a Spanish query against an English passage across candidates), not
      from a name
- [ ] T8 — Migration 102: the vector column plus the model and dimension it was
      produced with, so a model change is detectable rather than silent
- [ ] T9 — Cosine in Go and the fusion of the lexical and semantic rankings
- [ ] T10 — Tests including a Spanish query against an English corpus fixture

### Slice 5.3 — the retrieval surface

- [ ] T11 — `rag/http`: `GET /api/rag/search`, with the plugin as the intended
      consumer and no UI

## Contract of slice 5.1

The schema, and why it is shaped this way:

```sql
CREATE TABLE IF NOT EXISTS chunks (
    id          TEXT PRIMARY KEY,
    document_id TEXT NOT NULL REFERENCES documents(id),
    job_id      TEXT NOT NULL REFERENCES ingestion_jobs(id) ON DELETE CASCADE,
    page_number INTEGER NOT NULL CHECK (page_number >= 1),
    ordinal     INTEGER NOT NULL CHECK (ordinal >= 0),
    text        TEXT NOT NULL,
    created_at  TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    chunk_id UNINDEXED,
    text,
    tokenize = 'unicode61 remove_diacritics 2'
);
```

`chunks_fts` is a plain FTS5 table holding a second copy of the text rather than an
external-content table over `chunks`. The copy costs about 1,3 MB for a 359-page
book and buys a delete path with no triggers and no `'delete'`-row ceremony, which
is the part of external-content FTS5 that is easy to get subtly wrong. Indexing is
replace-all-per-document, so the two tables are written together in one
transaction and can never disagree outside it.

Ranking: FTS5's `bm25()` returns **smaller is better** (negative scores). The
repository must order ascending and the command must present the score in a way
that does not invite reading a bigger number as better.

`ChunkPage(text string, pageNumber int, options ChunkOptions) []Chunk`:

- Split on blank lines into paragraphs; accumulate them while the chunk stays
  under `TargetChars` (default 1200 characters, ≈300 tokens).
- Carry an overlap tail of about `OverlapChars` (default 15% of the target) into
  the next chunk, snapped to a word boundary.
- Never cross a page boundary: a chunk belongs to exactly one page, which is what
  makes a result citable.
- A single paragraph longer than the target is split at word boundaries; a page
  shorter than the target produces one chunk rather than nothing; a whitespace-only
  page produces none.
- Deterministic: the same page always yields the same chunks, in the same order,
  with ordinals starting at 0.

Query building is part of the contract, because FTS5's `MATCH` grammar is a small
query language and a search box is not: the user's text is split into tokens, each
token is turned into a quoted literal term (escaping embedded quotes by doubling
them), the terms are joined with `OR` and ranked by `bm25`. A query whose tokens
are all punctuation, or that matches nothing, returns an empty result rather than
an error. Letting raw input reach `MATCH` would let a stray `"` or `*` change the
query's meaning or raise an error in front of the user. My search stays out of
scope here and is recorded in the log: the lexical stage is where it would be added.

Indexing is derived data with a rebuildable life cycle: `IndexDocument` deletes the
document's chunks and writes them again from the recorded page text, so it is
idempotent and repairs staleness. A settled ingestion run rebuilds the document's
chunks; `-reindex` rebuilds every document.

## Acceptance criteria

1. `gofmt`, `go vet`, all tests green; the frontend untouched.
2. A Spanish query finds Spanish text whether or not the accents match, verified
   through the repository and not only in a probe.
3. Every result carries its document and its page number, and a chunk never spans
   two pages.
4. Indexing the same document twice leaves the same number of rows and the same
   results: it replaces rather than appends.
5. Search needs no API key, no PDF, no network, and no storage root: it reads the
   database and nothing else.
6. A query made only of punctuation, and a query that matches nothing, return an
   empty list and exit 0.
7. The chunker is covered by tests for its boundaries: target size, overlap,
   paragraph splitting, a giant paragraph, a tiny page, and an empty page.

## Out of scope

- Any plugin UI.
- Retrieval-augmented generation (asking a model to answer from the retrieved
  chunks). That is a separate step, and it needs the retrieval quality measured
  first.
- My-search syntax (`NEAR`, prefixes, field filters). Recorded in the log.
- Chunking that crosses pages, or headings inferred from the text: the stored text
  is verbatim and the structure of a book is not known to this step.
- Anything that indexes the generated `.md` in the vault: that file is the
  human-facing artifact, and Obsidian already searches it. The corpus this step
  indexes is the page text in the database.

## Log

**L1 (2026-10-17, request)** — the user asked to design step 5 and to begin
somewhere, having chosen: queries always in Spanish, books in Spanish and English;
chunking plus FTS5 first and embeddings afterwards; a CLI first; and ≈300-token
chunks with 15% overlap.

**L2 (2026-10-17, facts measured before designing)** — FTS5 with accent folding in
both directions was verified through the module's own driver; OpenRouter's
embedding endpoint and its prices were verified through the API; the corpus size
was measured from the real database. The verification probe was deleted, not
committed. An earlier version of that probe reported "unaccented query matched 0
rows", which was a defect in the probe (it inserted into one table and queried
another), not in FTS5. Worth recording because it is the third check of mine this
project that failed before the thing it was checking did.

**L3 (2026-10-17, slice 5.1 verified and committed)** — the slice was verified
against the seven acceptance criteria before committing. Checks, from
`apps/go-middleware`: `gofmt -l ./cmd ./internal` printed nothing; `go build ./...`
and `go vet ./...` were clean; `go test ./... -count=1` was ok in every package;
the new tests were run with `-run -v` and passed. `func Test` count went from 343
at HEAD to 370 in the slice (+27: 26 new tests plus one added migration test).

End-to-end, over a copy of the real `data/study.db` in `/tmp/step5-copia` (the
source database was never opened: its versions stayed 1|2|100 and its mtimes
unchanged). `-reindex` on the copy reported **2 documents, 135 chunks** on the
first run and the same 135 on the second and third. An independent count through
`python3` read `chunks = 135`, `chunks_fts = 135`, no orphan FTS rows, no chunk
referencing an unknown document, job, or page, ordinals contiguous from 0 on all
**33 recorded pages**, and the same content hash
(`c0aac7cb…29fba7`) before and after a reindex. `-search "attention" -limit 5`
returned 5 results with title, page, and snippet, with `STUDY_OPENROUTER_API_KEY`
unset and `STUDY_STORAGE_ROOT=/dev/null/nope` — no key, no PDF, no storage.
Accent folding held through the command on the real corpus: `rodriguez` and
`rodríguez` both found **RODRÍGUEZ MARTÍNEZ** on page 1, and `jurgen` and
`jürgen` both found **Jürgen** on page 11. `-search "..."` and
`-search "??? !!! —"` printed `0 results` with exit 0, and
`-search "zzzznomatchqq"` did the same.

One defect was found and fixed inside the slice: `-search` opened the database
through the planning helper, whose read-only fallback swaps in an empty
in-memory schema when the file cannot be opened. A `STUDY_DB_DSN` that pointed
nowhere therefore printed `0 results` and exit 0 instead of failing. `-search`
now opens read-only directly and exits 1 with `open database for search: …
unable to open database file` when the database is absent. This is the fourth
check in this project that failed before the thing it was checking did, and the
first one that was a defect in the shipped path rather than in the probe.

Recorded out of scope, as promised in the contract: FTS5 query syntax (`NEAR`,
prefixes, field filters, `AND`/`NOT`) is not exposed. The user's text is split
into tokens, each token is a quoted literal, and the terms are joined with `OR`,
so a stray `"`, `*`, or `^` cannot change the query's meaning.
