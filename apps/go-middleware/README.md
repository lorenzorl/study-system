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
└── server/main.go          composition root: wires everything by hand

internal/
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
└── rag/                    bounded context: documents, ingestion jobs, page outcomes
    ├── domain/               entities, ports, resumability invariants, page heuristic
    ├── app/                  the conversion run: acquire, resume, materialize
    ├── infra/pdf/            poppler adapters: page count, text, figures, renders
    └── sqlite/               repository adapters + the ingestion schema migrations
```

### The dependency rule

`domain` imports nothing but the standard library. `app` imports `domain` and
`platform`. `sqlite` and `http` are the outer edges. Nothing points inward.

Handlers declare the use case contracts they need (see `study/http/contracts.go`)
instead of importing concrete use case types, so transport tests use mocks and
the application layer can change shape without breaking them.

### Why by context, not by layer

A second context (RAG: documents, chunks, embeddings, and ingestion jobs) was
planned and now exists. Adding it meant adding a sibling of `study/`, exactly as
intended: no shared folder was edited for it, and no study file imports anything
from it. A flat `domain/`, `application/`, `infrastructure/` layout would have
mixed two unrelated domains into one set of folders and forced every later
change to reason about both.

## Running

```bash
cd apps/go-middleware
go run ./cmd/server                            # listens on :8080, override with -port
go run ./cmd/ingest -pdf book.pdf --out ~/vault/book.md   # convert a book to Markdown
go run ./cmd/ingest -pdf book.pdf -dry-run                # what it would cost, without doing it
go run ./cmd/ingest -list                        # list stored artifacts
go test ./...
go vet ./...
```

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
