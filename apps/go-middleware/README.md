# go-middleware

HTTP backend for the study system. Go 1.25, standard library only, no framework,
no ORM, and no cgo (SQLite runs through `modernc.org/sqlite`, a pure-Go driver).

It serves the Obsidian plugin in `apps/obsidian-plugin`.

## Layout

Packages are organized **by bounded context first, layer second**. A context owns
its domain, its use cases, its adapters, and its HTTP surface.

```text
cmd/
├── ingest/main.go          acquires and persists a document (pipeline slice 1)
└── server/main.go          composition root: mounts both contexts

internal/
├── composition/            the only package that knows every context: database
│                           openers, the migration list, model clients, the
│                           wired contexts. Builds; never mounts.
├── platform/               cross-cutting, no business logic
│   ├── apperror/             sentinel errors shared by every context
│   ├── config/               env + .env resolution, defaults, validation
│   ├── db/                   SQLite connection setup, PRAGMAs, migration runner
│   ├── httpx/                JSON/error writers, CORS middleware
│   ├── llm/                  OpenRouter chat-completions client
│   └── storage/              artifact store: atomic writes, size ceiling
├── study/                  bounded context: topics, concepts, flashcards, reviews
│   ├── domain/               entities, repository ports, FSRS, no external imports
│   ├── app/                  one file per use case
│   ├── sqlite/               repository adapters + the study schema migrations
│   └── http/                 contracts, DTOs, one handler file per resource, router
└── rag/                    bounded context: documents, jobs, page outcomes, chunk index, embeddings
    ├── domain/               entities, ports, resumability, page heuristic, chunker, cosine, RRF
    ├── app/                  the conversion run, the index rebuild, embedding, and hybrid search
    ├── infra/pdf/            poppler adapters: page count, text, figures, renders
    ├── http/                 contracts, DTO, the search handler, and the RAG router
    └── sqlite/               repository adapters + the ingestion, chunk-index, and vector migrations
```

### The dependency rule

`domain` imports nothing but the standard library. `app` imports `domain` and
`platform`. `sqlite` and `http` are the outer edges. Nothing points inward.

Handlers declare the use case contracts they need (see `study/http/contracts.go`)
instead of importing concrete use case types, so transport tests use mocks and
the application layer can change shape without breaking them.

### Composition builds, the entrypoint mounts

`internal/composition` is the one package that knows every context. It turns a
`config.Config` and a `*sql.DB` into wired contexts (`NewStudy`, `NewRAG`, the
model `Clients`, the database openers, and the single migration list), and it is
where a rule lives once instead of once per binary: the two migration blocks in
one run, read-only versus read-write versus planning opens, and hybrid versus
lexical search.

The package registers no route, parses no flag, and starts no server. Each
entrypoint keeps its own transport, its own policy, and its own shutdown, which
is why there is no `Router` here: the RAG router needs the upload handler, and
that needs the store and the "is a key configured" flag — entrypoint policy, not
wiring. The dependency direction is outward only: this package imports every
context, and no context imports it.

It also exists so a third entrypoint is possible without a third copy of the
wiring. `cmd/server` and `cmd/ingest` both build through it today.

### Why by context, not by layer

A second context (RAG: documents, chunks, embeddings, and ingestion jobs) was
planned and now exists. Adding it meant adding a sibling of `study/`, exactly as
intended: no shared folder was edited for it, and no study file imports anything
from it. A flat `domain/`, `application/`, `infrastructure/` layout would have
mixed two unrelated domains into one set of folders and forced every later
change to reason about both.

## Running

One binary, one subcommand per job. `study-system help` lists them; every
subcommand answers `-h` with its own flags.

```bash
cd apps/go-middleware
go build -o bin/study-system ./cmd/cli

bin/study-system serve                              # the HTTP API on :8080; -port overrides
go run ./cmd/cli serve                              # same thing, without building

go run ./cmd/cli convert -pdf book.pdf -out ~/vault/book.md
                                                    # convert a book to Markdown, then index it
go run ./cmd/cli convert -pdf book.pdf -out ~/vault/book.md -dry-run
                                                    # what it would cost, without doing it
go run ./cmd/cli list                               # list stored artifacts
go run ./cmd/cli reindex                            # rebuild the chunk index from recorded pages
go run ./cmd/cli embed                              # rebuild the embeddings; needs the key and the network
go run ./cmd/cli search "attention" -limit 5         # search the index; no key, PDF, or storage

go test ./...
go vet ./...
```

`cmd/server` is still there for anyone who runs it directly; it shares the serve
loop with `serve` and has the same flags.

In a shell, `search` takes the query as its positional arguments, so a phrase
needs no quoting to survive the flag parser: `search shortest path -limit 5` and
`search -limit 5 shortest path` mean the same thing.

The SQLite file lives at `data/study.db` (relative to this directory) and is
created automatically. The schema is applied on open.

Configuration comes from the environment. A `.env` file in this directory is
read too, if present; a real environment variable always beats the file, and the
file always beats the built-in default. See the Configuration section below for
the variables, and create `.env` with the ones you need to override.

## API

| Method | Route | Purpose |
| --- | --- | --- |
| `POST` | `/api/topics` | Create a topic manually |
| `POST` | `/api/concepts` | Create a concept under a topic |
| `GET` | `/api/concepts` | Topic-to-concept tree |
| `POST` | `/api/sync/concept` | Upsert topic + concept from an Obsidian note path |
| `POST` | `/api/sync/flashcards` | Batch-upsert flashcards by `obsidian_id` |
| `POST` | `/api/sync/resource` | Upsert an external study resource by `source_uri` |
| `GET` | `/api/study/due` | Flashcards whose next review is due |
| `POST` | `/api/study/review` | Record a review, recompute the card state |
| `GET` | `/api/rag/search` | Ranked chunks for a query (`q`, `limit`, and an optional `document_id` that scopes it to one document); no key required |
| `GET` | `/api/rag/documents` | The corpus: `id`, `title`, `created_at` and the page and chunk counts of each document |
| `POST` | `/api/rag/documents` | Upload a PDF and queue background processing; `multipart/form-data` with one `file` part |
| `GET` | `/api/rag/documents/{id}/status` | Processing status of an uploaded document |

## Configuration

Every knob is read in exactly one place: `internal/platform/config`. If a
variable is not listed here, nothing reads it.

| Variable | Default | Purpose |
| --- | --- | --- |
| `STUDY_PORT` | `8080` | HTTP listen port, 1-65535. The `-port` flag overrides it and applies the same validation. |
| `STUDY_DB_DSN` | `file:data/study.db` | SQLite data source name. |
| `STUDY_STORAGE_ROOT` | `data/artifacts` | Directory holding stored artifacts. |
| `STUDY_MAX_UPLOAD_BYTES` | `104857600` (100 MiB) | Largest single artifact accepted. The largest real book on hand is 14 MiB. |
| `STUDY_CORS_ORIGINS` | Obsidian + localhost origins | Comma-separated allowlist of origins that may read API responses. |
| `STUDY_OPENROUTER_API_KEY` | unset | OpenRouter key used by ingestion. Read as optional so the server keeps booting without it; `cmd/ingest` requires it and aborts before page 1 when it is missing. |
| `STUDY_LLM_MODEL` | `qwen/qwen3.5-flash-02-23` | Model slug used for the pages that need the model. |
| `STUDY_EMBEDDING_MODEL` | `google/gemini-embedding-2` | Multilingual embedding model for the semantic stage; chosen from measured evidence (see Semantic search). |
| `STUDY_OPENROUTER_BASE_URL` | `https://openrouter.ai/api/v1` | Base URL of the chat-completions API. Override it for a proxy, a gateway, or a test double; a malformed value aborts the boot. |

Invalid values fail at startup with a message naming the variable. A port
outside 1-65535, a non-numeric port, or an origin list made only of separators
all abort the boot rather than falling back silently.

### CORS

`STUDY_CORS_ORIGINS` defaults to the origins Obsidian and the local dev harness
use. **That default has not been verified against a real device.** If a
legitimate client is rejected, add its origin to the variable instead of
widening the default.

Setting it to `*` restores a permissive wildcard. A wildcard is incompatible
with credentialed requests, but it still lets any website the user visits read
this API's responses, and this API has no authentication. Treat it as a
deliberate, temporary measure.

`httpx.CORS` is a pure function of the origin list it receives: it reads no
environment and decides no policy. The composition root in `main.go` is what
connects config to the middleware.

## Artifact storage

`platform/storage` persists binary artifacts: the document a pipeline acquires,
and later the page renders and Markdown it produces. It exists so no pipeline
even has to know a path, which is the fault the standalone `optimizebooks`
script had — its input, page range, and output were all literals in the source.

Layout on disk:

```text
<root>/<artifact-id>/payload     the bytes
<root>/<artifact-id>/meta.json   original name, size, timestamp
<root>/<artifact-id>/work/       per-artifact scratch space
<root>/incoming/                 staging for in-flight writes
```

### Invariants

- **The identifier is generated, never supplied, and validated on every read.**
  It is the only value that reaches a path, and it must match a strict UUID
  pattern. Path traversal is removed by construction rather than filtered.
- **The payload filename is fixed.** Neither a caller-supplied name nor the
  metadata file can influence where content is read from. The original name is
  metadata only, reduced to its base name, so tampering with `meta.json` cannot
  redirect a read.
- **Writes are staged and renamed.** An interrupted or oversized transfer leaves
  nothing behind, so a half-written artifact can never later look valid.
- **Content is streamed in bounded chunks with context checks.** A large PDF does
  not become a large allocation. The ceiling is enforced by reading one byte past
  it, not by buffering the whole thing.

`Artifact.OriginalName` carries the format hint. The store is format-agnostic on
purpose: a tool that needs an extension should ask for the original name rather
than infer one from a filename.

## Page extraction

`rag/infra/pdf` drives the poppler command-line tools: `pdfinfo` for the page
count, `pdftotext` for the text layer, `pdfimages -list` for the image
inventory, and `pdftoppm` for page renders. All four sit behind the ports in
`rag/domain` (`TextExtractor`, `FigureIndex`, `PageRenderer`), so the pipeline is
testable with no binary installed and with no network access.

Install them with `brew install poppler`. A constructor that needs a missing
tool fails on the spot, naming the binary, instead of failing hundreds of pages
into a run.

The text path is the default and the vision model is rationed
(`domain.NeedsModel`): a page goes to the model only when its extracted text is
shorter than `domain.MinExtractableText` or when the page carries an image.
Extraction is exact and free; a model that is unsure invents confidently, and a
confident invention matches queries and answers them wrongly. Rationing is what
limits how much of the corpus can be confidently wrong.

Three properties of these adapters are deliberate:

- **`pdfimages -list` is parsed header-driven**, using the column positions the
  header declares rather than fixed offsets. A row that cannot be parsed is an
  error, never a silent skip: counting zero images looks exactly like a book with
  no diagrams, and the vision path would stop running with no symptom at all.
- **Renders are transient.** `RenderPage` renders into a fresh temporary
  directory under the artifact workspace, returns the PNG bytes, and removes that
  directory on every path. A 359-page book leaves no orphan images behind, which
  matters because artifact retention (`P1`) does not exist yet. A render is
  reproducible from the stored PDF, so there is nothing worth persisting.
- **Render resolution is a constant**, 150 dpi, not a config knob: the vision
  path needs one legible resolution, and nothing measures an alternative. PNG at
  that resolution is the current choice; JPEG is the lever if request payloads
  ever become the limiting factor.

The only PDF in the repository is the test fixture at
`internal/rag/infra/pdf/testdata/three-pages.pdf`: three pages, being text-only,
text plus a 2x2 image, and image-only. `testdata/make_fixture.go` regenerates it.

## Ingestion

`go run ./cmd/ingest -pdf book.pdf --out <path>.md` converts a PDF to Markdown, one
page at a time. The output path is supplied by the caller: the backend never
guesses where a vault is.

The pipeline acquires the bytes, identifies the document by their content hash,
continues that document's job, processes only the pages that still owe an outcome,
and materializes the Markdown from everything recorded. Nothing is written until
every page reached a terminal outcome, so an interrupted run leaves the previous
complete file in place instead of a partial one. That is the retired
`optimizebooks` script's defect stated as a property: its output carried two
headers because it wrote a range before processing it.

**One job per document, continued rather than restarted.** The per-page outcomes
are the single source of truth for that document's conversion, and the schema
keeps exactly one row per page, so a re-run after a kill redoes only the pages
that are still owed. A page that failed is owed again; a page that succeeded is
never re-extracted, and its stored text is what appears in the output.

Which pages reach the model is decided by the same rule as the extractor path:
text shorter than `domain.MinExtractableText`, or a page carrying an image. A
typical 15-page paper sends 2 to 5 pages to the model; a born-digital text-only
document sends none. The model is asked to transcribe faithfully, describe
figures and tables, and mark what it cannot read rather than invent it.

Also deliberate:

- **A run says what it will cost before it does anything.** The plan reports how
  many pages are already recorded, how many come from the text layer, how many
  will reach the model, and a rough duration. `-dry-run` prints it and exits
  without storing the bytes, creating the document or writing the file; it needs
  no API key, because it never calls the model. `-max-model-pages N` refuses a
  run whose plan exceeds N, and the refusal happens before anything is stored or
  created, so a run that is refused leaves nothing behind.
- **The output file is never someone else's.** A target that exists without the
  generated header is refused, with `-force` as the way to replace it deliberately;
  a target this pipeline generated is rewritten as normal. The file is written
  0644, because a vault is shared and a sync process running as another user has
  to be able to read it.
- **The output is committed atomically**: a temporary file in the target
directory is renamed into place, so a vault file is never observed half-written.
- **A failed page does not fail the run.** It is recorded with its reason, the
  rest of the book continues, and the run reports how many succeeded. The command
  exits non-zero only when the run aborts or when nothing succeeded.
- **429 and 5xx are retried three times with backoff**, because a rate limit in
  the middle of a 359-page book must not cost a page a manual re-run. Other 4xx
  responses are not retried: they are request or account defects.
- **`-list` needs no key, no database, and no PDF.** It exists to inspect the
  store and returns before anything else is opened.

## Lexical search

`go run ./cmd/ingest -reindex` rebuilds the chunk index from the page text
already recorded in `job_pages`, and `go run ./cmd/ingest -search "query" -limit N`
answers a lexical query over it. Both read the database and nothing else: no API
key, no PDF, no network, no storage root. `-search` opens the database read-only
and fails when it is absent rather than answering an empty result set.

The index is derived data with a rebuildable life cycle. `-reindex` walks every
document that has a recorded page, deletes that document's chunks, and writes
them again inside one transaction, so it is idempotent and repairs staleness; a
run that settles also rebuilds the document it just converted.

- **Chunking** is `rag/domain.ChunkPage`, a pure function of the page text and
  the page number: ≈300 tokens (1200 characters) per chunk with a 15% overlap,
  cut on blank lines, and never across a page boundary, so every chunk cites
  exactly one page (S4).
- **The schema** is migration 101: `chunks` is the source of truth with the
  citation, and `chunks_fts` is an FTS5 table holding a second copy of the text,
  tokenized with `unicode61 remove_diacritics 2`. The copy buys a delete path
  with no triggers and no `'delete'`-row ceremony; the two tables are always
  written together in one transaction.
- **The query** is the user's text, not a query language: each whitespace token
  becomes one quoted literal term (embedded quotes doubled), the terms are joined
  with `OR`, and `bm25` ranks them. A token with no letter or digit is dropped, so
  a punctuation-only query is an empty result and never an error. FTS5's syntax
  (`NEAR`, prefixes, field filters) is deliberately not exposed.
- **The ranking** is `bm25`, which is smaller-is-better (negative scores). The
  command prints the magnitude under `RELEVANCE`, so a bigger number is a better
  match, and orders ascending.
- **Accents fold in both directions**, which is what lets a Spanish query find
  Spanish text however the accents were typed (S1/S5). The lexical stage is real
  but partial: it cannot cross the language boundary on its own, which is what the
  semantic stage below is for.

## Semantic search and hybrid retrieval

`-embed` rebuilds the vectors and `-search` uses them. This is the stage that
crosses the language boundary between a Spanish query and an English book (S1).

- **The model** is `google/gemini-embedding-2` (3072 dimensions), read from
  `STUDY_EMBEDDING_MODEL`. It was chosen from a measured ranking of eight
  multilingual candidates over 16 Spanish queries against the real English corpus,
  not from a slug; `baai/bge-m3` is the cheaper alternative recorded in the log.
- **`-embed` is separate from `-reindex`.** Indexing is local and free; embedding
  is the only step that needs the API key and the network, so it is never reached
  by accident from a conversion or a reindex. It rebuilds every document that has
  chunks, one document at a time and inside one transaction per document, so it is
  idempotent: a re-run replaces rather than appends.
- **The schema is migration 102**: `chunk_embeddings` stores one vector per chunk
  with the `model` and `dimensions` that produced it. A vector from another model
  is filtered out of a search rather than compared against the current query
  vector, and a blob whose length is not `dimensions * 4` is an error. The foreign
  key cascades from `chunks`, so a reindex that replaces a document's chunks
  cannot leave a vector pointing at a chunk that no longer exists.
- **The encoding** is `float32`, little-endian, `dimensions * 4` bytes.
- **The fusion** is Reciprocal Rank Fusion with k = 60. Ranks are fused, not scores,
  because bm25 is smaller-is-better and negative while cosine is larger-is-better
  in [-1, 1]; a score-normalising fusion would have to invent a common scale. A
  result carries the stage that found it: `lexical`, `semantic`, or `hybrid` when
  both agreed.
- **Search degrades gracefully.** The lexical stage always runs and reads only the
  database. The semantic stage runs only when vectors are stored for the configured
  model and the query holds a letter or a digit; with no stored vectors, or with no
  API key, `-search` is a lexical search and makes no network call. A
  punctuation-only query returns no results without embedding anything.
- **`-search` prints** `MATCH` (the stage), `BM25` (the lexical score's magnitude,
  bigger is better) and `COSINE` (the semantic similarity, bigger is better). The
  score a stage did not produce reads 0.

## Retrieval API

`GET /api/rag/search` is the HTTP surface of the same search the CLI runs, and it
adds no retrieval behaviour: it wraps `app.SearchUseCase`, so a search answers the
same ranking the command prints. The intended consumer is the Obsidian plugin;
there is no UI. The route is registered by `rag/http`, and the composition root
mounts it under `/api/rag/` with `httpx.CORS` as the outermost wrapper.

| Parameter | Required | Rule |
| --- | --- | --- |
| `q` | yes | Trimmed. An empty or whitespace-only value is `400 "q is required"`. |
| `limit` | no | Absent or empty uses `app.DefaultSearchLimit` (10). A non-number, a value below 1, or a value above `MaxSearchLimit` (50) is `400` naming the bound. |

- **Response.** `200` with a **bare JSON array** of result objects, never a
  wrapper and never `null`: an empty result encodes as `[]`, which is why the
  handler builds it with `make([]SearchResultResponse, 0, n)`. Fields:
  `chunk_id`, `document_id`, `document_title`, `page_number`, `snippet`,
  `text`, `stage`, `bm25`, `similarity`.
- **`snippet` and `text` are not the same string, and the difference is the point.**
  `snippet` is the citation aid: FTS5 builds it from twelve tokens around the
  match with the matched terms marked, and a semantic hit gets a 240-rune excerpt.
  `text` is the chunk verbatim. A caller that only shows a result wants the
  snippet; a caller that asks a model to answer from the result needs `text`,
  because twelve tokens of context is a fragment and a model answering from a
  fragment invents the rest.
- **Score direction.** `bm25` is the lexical score's **magnitude** (bigger is
  better) and `similarity` is the cosine (bigger is better). A stage that did not
  rank a chunk leaves its value `0`. The command prints the same directions, so
  the two surfaces do not disagree about which way is better.
- **Punctuation is not an error.** A `q` with no letter or digit (for example
  `...`) returns `200` with `[]`, exactly as the CLI exits `0` with
  `0 results`.
- **No key required.** The lexical stage reads only the database. With
  `STUDY_OPENROUTER_API_KEY` set the server upgrades to the hybrid use case, and
  it still degrades to lexical-only when the configured model has no stored
  vectors.

## Document upload and processing status

`POST /api/rag/documents` accepts a PDF and queues it;
`GET /api/rag/documents/{documentID}/status` reports how far processing has got.
The pipeline runs in the background in the same process (S2): the upload returns
immediately and the client polls the status. Processing is ingestion, the lexical
index, and the embeddings, so an uploaded book is hybrid-searchable when it
completes (S3). The intended consumer is the Obsidian plugin; there is no UI.

### Upload

`POST /api/rag/documents`

- **Body.** `multipart/form-data` with one `file` part. The part is streamed into
  the artifact store; only the five `%PDF-` header bytes are buffered, and the
  SHA-256 is computed on the same stream. The document is found by content hash
  and created only when absent, so re-uploading the same bytes resumes the same
  document.
- **`202 Accepted`** with `{"document_id": "<id>", "state": "queued"}`.
- **`400 Bad Request`** when the request is not multipart, has no `file` part,
  the part is empty, or the bytes do not start with `%PDF-`.
- **`413 Request Entity Too Large`** when the body exceeds
  `STUDY_MAX_UPLOAD_BYTES`; the store enforces the ceiling.
- **`503 Service Unavailable`** when `STUDY_OPENROUTER_API_KEY` is unset. The
  pipeline cannot transcribe a page without it, so the upload is refused up
  front rather than queued to fail later. The server still boots and serves
  search and status.

The Markdown lands at `data/artifacts/<artifact-id>/book.md`, next to the stored
PDF (S4). No new configuration is involved.

### Status

`GET /api/rag/documents/{documentID}/status`

- **`200 OK`** with the status object:

```json
{
  "document_id": "…",
  "title": "DDD.pdf",
  "state": "queued|converting|indexing|completed|failed",
  "pages": {"total": 359, "extracted": 333, "transcribed": 26, "failed": 0, "pending": 0},
  "chunks": 1045,
  "embedded": 1045,
  "error": "…",
  "started_at": "…",
  "updated_at": "…",
  "finished_at": "…"
}
```

- **`state` is derived, never stored.** A recorded in-process failure wins; then
  no job means `queued`; a running or pending job means `converting`; a failed
  job means `failed`; a completed job means `indexing` until `embedded >= chunks`,
  then `completed`.
- `error`, `started_at`, `updated_at` and `finished_at` are omitted when empty.
- Counts come from aggregate reads, so a poll never loads page or chunk text.
- **`404 Not Found`** when the document is unknown.

The processor's queue and its last error live in memory. A server restart loses
them; the document and its recorded pages stay in the database, so re-uploading
the same bytes resumes from where it stopped.

## Schema migrations

`platform/db` owns the mechanism; each context owns its schema and exports a
migration list. `sqlite.Migrations()` is the study context's and
`rag/sqlite.Migrations()` is the ingestion context's.

- **Each context owns a version block.** `schema_version` is one table keyed by
  the version alone, and the runner treats every version at or below
  `max(version)` as already applied. Under one shared ascending sequence, a call
  that received only one context's list would not fail — it would silently skip
  the other context's lower versions. Study therefore owns 1-99 and the
  ingestion context owns 100-199, which makes the mistake harmless and a
  cross-context collision impossible. `cmd/server` applies
  `slices.Concat(study.Migrations(), rag.Migrations())` in one run.

- `Migrate` applies pending migrations in ascending version order, each inside a
  transaction that also records it in `schema_version`. A migration either
  applies completely and is recorded, or leaves nothing behind.
- It is idempotent, so it runs on every boot.
- Version 1 of the study schema is a **baseline**: every statement uses
  `CREATE TABLE IF NOT EXISTS`. That is what lets a database created before this
  system existed be adopted without data loss.
- Forward-only. There is no down path; a single-user local database does not
  need one.

To add a table, append a migration to the owning context's list. Never edit an
already-applied migration: databases in the wild have run it.

### Connection PRAGMAs

`db.Open` is the only supported way to open the database, because it is what
applies `foreign_keys=1`, `busy_timeout=5000`, and `journal_mode=WAL` to every
connection through the DSN. `PRAGMA foreign_keys` is a per-connection setting, so
issuing it once against a pooled handle would leave most connections unprotected.

The driver import lives in `platform/db` and nowhere else, so there is exactly
one place that can get this wrong.

## Known debt

The authoritative list lives in
[`odd/tasks/backlog.md`](../../odd/tasks/backlog.md) so it is maintained in
one place and cannot rot into conflicting copies. As of step 3 it holds an
unverified CORS origin allowlist (`A1`) and the absence of any artifact
retention policy (`P1`).

A timezone defect (`B1`) was closed after step 3. Its entry in the backlog is
worth reading: the first diagnosis overstated it, and the correction is recorded
there rather than quietly deleted.

One property of the design belongs here rather than in a task list: **a database
adopted from before the migration system keeps whatever columns it had**, except
the one migration 2 drops explicitly. Any further drift has to be expressed as a
new migration.
