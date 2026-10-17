# Step 5 — Search over the ingested corpus

**Status:** done (slices 5.1, 5.2, 5.3 complete; RAG and the plugin UI remain out of
scope)
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

- [x] T7 — `platform/llm`: `Embed`, and a multilingual model chosen from measured
      evidence (a Spanish query against an English passage across candidates), not
      from a name
- [x] T8 — Migration 102: the vector column plus the model and dimension it was
      produced with, so a model change is detectable rather than silent
- [x] T9 — Cosine in Go and the fusion of the lexical and semantic rankings
- [x] T10 — Tests including a Spanish query against an English corpus fixture

### Slice 5.3 — the retrieval surface

- [x] T11 — `rag/http`: `GET /api/rag/search`, with the plugin as the intended
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

## Contract of slice 5.2

Decided 2026-10-17 from the measurement in L4. Where a spec or the 5.1 contract
already fixes a rule, this contract does not repeat it.

**Model.** `google/gemini-embedding-2`, 3072 dimensions, read from
`STUDY_EMBEDDING_MODEL` (default that slug). It was chosen from the measured
ranking in L4 and not from a name: `voyage-4-lite`, the slug the earlier facts
proposed, ranked worst of the multilingual candidates on the real corpus.

**Migration 102** adds one table. `chunks` stays the source of truth; a vector is
derived data with the same rebuildable life cycle.

```sql
CREATE TABLE IF NOT EXISTS chunk_embeddings (
    chunk_id   TEXT PRIMARY KEY REFERENCES chunks(id) ON DELETE CASCADE,
    model      TEXT NOT NULL,
    dimensions INTEGER NOT NULL CHECK (dimensions > 0),
    vector     BLOB NOT NULL,
    created_at TEXT NOT NULL
);
```

The row carries the model and the dimension, so a vector produced by another
model is detectable instead of silently compared against the current one. A blob
whose byte length is not `dimensions * 4` is an error.

**Encoding.** `float32`, little-endian, `dimensions * 4` bytes.

**Ports and pure functions.**

- `domain.Embedder`: `Embed(ctx, texts []string) ([][]float32, error)`.
- `domain.EmbeddingRepository`: replace-all-per-document inside one transaction,
  and a read of every stored vector for one model joined with the chunk's
  citation and text.
- `domain.ChunkRepository` gains a read of one document's stored chunks (id and
  text) so the embedder can be fed without re-reading page outcomes.
- `domain.Cosine(a, b []float32) (float64, error)`.
- `domain.FuseRankings(rankings ...[]string) []string`, Reciprocal Rank Fusion
  with k = 60. RRF fuses ranks, not scores: bm25 is smaller-is-better and
  negative while cosine is larger-is-better in [0,1], so a score-normalising
  fusion would have to invent a common scale.

**`platform/llm`.** A separate `EmbeddingClient` with its own constructor posts
to `/embeddings`, batches inputs, reuses the existing retry/backoff and response
bound, and never touches the chat-completions path.

**`rag/app`.** `EmbedDocument`/`EmbedAll` rebuild a document's vectors from its
stored chunks, replace-all-per-document, and are idempotent. `Search` gains the
semantic stage and the fusion: the lexical bm25 ranking and the semantic cosine
ranking are fused with RRF, and a result carries the stage that found it. When
the configured model has no stored vectors, search falls back to lexical-only
rather than failing, so a machine with no embeddings still searches.

**`cmd/ingest`.** `-embed` rebuilds the embeddings of every document that has
chunks; it is separate from `-reindex` because it is the only step that costs
money and reaches the network. `-search` uses the hybrid ranking when vectors
exist.

**Tests are hermetic.** No test calls the network: a fake `Embedder` returns
deterministic vectors, and the Spanish-query-against-an-English-fixture test uses
it. The live measurement stays in L4.

**Acceptance criteria for 5.2.**

1. `gofmt`, `go vet`, all tests green.
2. A Spanish query finds an English passage it shares no literal token with,
   verified through the repository and the app, not only in a probe.
3. Cosine and the fusion are covered by tests for their boundaries.
4. Indexing the same document's embeddings twice leaves the same rows.
5. The migration carries the model and the dimension, and a vector from another
   model is not mixed into a search.
6. Embedding needs the API key and the network; search works without them once
   vectors are stored.

## Contract of slice 5.3

Decided 2026-10-17. The HTTP surface wraps the 5.1/5.2 search use case; it does
not add retrieval behaviour.

- **Route.** `GET /api/rag/search`, registered by `internal/rag/http` with the
  stdlib method+path pattern the study context already uses.
- **Query parameters.** `q` is required and trimmed; an empty or whitespace-only
  `q` is `400 "q is required"`. `limit` is optional: absent or empty uses
  `app.DefaultSearchLimit` (10); a non-number, a value below 1, or a value above
  `MaxSearchLimit` (50) is `400` with a message naming the bound.
- **Punctuation is not an error.** A `q` that holds no letter or digit (for
  example `...`) returns `200` with an empty array, exactly as the CLI exits 0
  with `0 results`.
- **Response.** `200` with a **bare JSON array** of result objects, built with
  `make([]SearchResultResponse, 0, n)` so an empty result encodes as `[]` and
  never as `null`. Fields: `chunk_id`, `document_id`, `document_title`,
  `page_number`, `snippet`, `stage`, `bm25`, `similarity`.
- **Score direction.** `bm25` is the **magnitude** of the lexical score (bigger
  is better) and `similarity` is the cosine (bigger is better); a stage that did
  not rank the chunk leaves its value `0`. This matches what the command prints,
  so the two surfaces do not disagree about which direction is better.
- **Layering.** The handler declares its own `SearchUseCase` interface in
  `contracts.go`, like `study/http`, so it is mockable and does not import the
  concrete use case. `NewRouter(search SearchUseCase) http.Handler` registers the
  one route and applies no CORS; the composition root wraps it, as it already
  does for the study router.
- **Composition root.** `cmd/server` builds a lexical `app.SearchUseCase` over
  `ragsqlite.NewChunkRepository`, upgrades it to `NewHybridSearchUseCase` when
  `cfg.OpenRouterAPIKey` is set, mounts the RAG router under `/api/rag/` on a
  root mux, and keeps `httpx.CORS(cfg.CORSAllowedOrigins)` as the outermost
  wrapper. The key stays optional: the server boots and answers lexical-only
  without it.
- **Tests** are `internal/rag/http/handler_test.go`, package `http_test`, with a
  hand-written mock and `httptest`, covering success, empty, missing `q`, an
  invalid `limit`, a punctuation-only `q`, and a use-case error. No UI and no
  plugin code is touched.

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

**L4 (2026-10-17, embedding model measured)** — eight candidate models were
measured against the real corpus before choosing one, because a name is not
evidence. The corpus was the 135 chunks of the two ingested papers (English) and
the test was 16 Spanish queries whose answer lives in one specific English
chunk; a model was scored by where it ranked that chunk. No model was judged by
its slug. Measured (`@1`/`@5`/MRR over 16 queries):

| model | dim | @1 | @5 | MRR | US$/M tok |
| --- | --- | --- | --- | --- | --- |
| `google/gemini-embedding-2` | 3072 | 0.50 | 0.88 | **0.68** | 0.20 |
| `baai/bge-m3` | 1024 | 0.44 | 0.75 | 0.57 | 0.01 |
| `perplexity/pplx-embed-v1-0.6b` | 1024 | 0.38 | 0.88 | 0.57 | 0.004 |
| `nvidia/nemotron-3-embed-1b:free` | 2048 | 0.31 | 0.88 | 0.53 | 0 |
| `voyageai/voyage-4` | 1024 | 0.25 | 0.88 | 0.47 | 0.06 |
| `voyageai/voyage-4-lite` | 1024 | 0.25 | 0.75 | 0.46 | 0.02 |
| `qwen/qwen3-embedding-4b` | 2560 | 0.38 | 0.50 | 0.46 | 0.02 |
| `qwen/qwen3-embedding-8b` | 4096 | 0.25 | 0.62 | 0.39 | 0.01 |

The lexical stage over the same 16 queries and the same corpus scored
precision@1 = 0.31 and @5 = 0.50 (page-level, and it only reached that through
shared technical tokens such as `transformer` and `0.1`), so the semantic stage
is what actually crosses the language boundary. `google/gemini-embedding-2` won
on every aggregate and costs about US$ 0,063 to embed a 359-page book, which is
why it is the chosen default; `baai/bge-m3` is the cheap alternative recorded
here if cost ever matters. The earlier probe with 6 synthetic topics was
discarded: every multilingual model scored 1.00, so it could not tell them
apart. The measurement scripts were throwaway, like the L2 probe.

**L5 (2026-10-17, slice 5.2 verified and committed)** — the slice was implemented
by a bounded writer and then verified by a separate read-only agent against the
Contract of slice 5.2; the writer's report was treated as a claim, not evidence.
`gofmt -l`, `go build ./...`, `go vet ./...` clean; `go test ./... -count=1` ok in
every package; `func Test` went from 370 at `09d3d78` to 435 in the slice (+65).

Live end-to-end on the `/tmp/step5-copia` copy with the real key: `-embed` wrote
**135 vectors at 3072 dimensions** with `google/gemini-embedding-2` in ~10 s
(`schema_version` 1|2|100|101|102, `length(vector) = 12288 = 3072*4`, 0 orphan
rows). With the key, `-search` returned `MATCH = lexical|semantic|hybrid` rows and
the scaled dot-product chunk came back with cosine 0.598. From a directory with
no `.env` and the key unset, `-search` stayed lexical-only and exited 0, and
`-embed` exited 1 naming `STUDY_OPENROUTER_API_KEY`. The independent verification
returned PASS on all six 5.2 acceptance criteria and on the 5.1 regression,
including a reversed-index fake that proved all 135 chunk/vector pairings survive
the batch, and a model-mismatch probe that proved another model's vectors are not
compared against the configured model's query.

Two advisories from that verification were fixed before the commit:
`TestFuseRankings_UsesTheFixedRRFConstant` did not actually pin k=60 or the rank
base (a mutation to k=59 passed it), so
`TestFuseRankings_PinsTheRankBaseAndKExactly` was added and observed to fail under
both mutations; and a semantic-only row printed `BM25 -0.000`, now `math.Abs`, so
an unranked stage reads `0.000`.

One residual finding is left for the user, not fixed: the fusion can rank a noisy
lexical hit above the correct semantic one. On the real corpus,
`-search "cómo se calcula la atención de producto punto escalado"` ranks a page-8
BLEU table first (lexical, matched the stopword `de` against `EN-[DE]`) and the
scaled dot-product chunk second (semantic, cosine 0.598). Both score 1/61 under
RRF, so the tie-break decides. The cause is the 5.1 contract keeping every token
as an `OR` term, stopwords included; dropping stopwords, weighting the semantic
stage, or widening the candidate pool are changes the user decides, not this
slice.

**L6 (2026-10-17, slice 5.3 verified and committed)** — the HTTP surface was
implemented by a bounded writer and then exercised live by the parent through the
real server, not only through the writer's httptest cases. `gofmt -l`,
`go build ./...`, `go vet ./...` clean; `go test ./... -count=1` ok in every
package; `func Test` went from 435 at `4b7ba95` to 447 (+12, no existing test
changed).

Live runs (ports 8097-8099; 8080 belongs to another project and `-port 0` is
rejected by design). With the key set, `GET /api/rag/search?q=attention&limit=3`
returned `200` with a bare JSON array whose stages were `hybrid`, `lexical`, and
`semantic`, and whose `bm25` was the magnitude and `similarity` the cosine. A
missing `q` returned `400 {"error":"q is required"}`; `q=...` returned
`200 []`; `limit` of `0`, `51`, or `abc` returned
`400 {"error":"limit must be between 1 and 50"}`. A Spanish query returned the
scaled dot-product chunk with `stage: semantic` and `similarity: 0.598` (the
noisy lexical hit still ranked first, the residual finding recorded in L5). Run
from a directory with no `.env` and no key, the server still answered `200`
lexical-only. `GET /api/concepts` still returned `200` on the root mux, so
mounting the RAG router under `/api/rag/` did not regress the study routes. The
composition root (`package main`) has no committed unit test; the live run above
is its evidence.

Step 5 is complete: the lexical index (5.1), the semantic stage and its fusion
(5.2), and the HTTP surface (5.3) are implemented, tested, and committed. The
residual ranking finding in L5 is the only open quality item, and it is a user
decision, not a defect.
