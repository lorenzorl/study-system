# Step 4 — Ingestion context: job model and hybrid conversion

**Status:** in progress
**Created:** 2026-10-17
**Depends on:** steps 1-3 (`config`, `db` migrations, `storage`)

## Decisions taken by the user (2026-10-17)

| Decision | Choice | Consequence |
| --- | --- | --- |
| End goal | **Ingest + search** | Step 4 and step 5. No plugin UI, so the deleted Dify view stays deleted. |
| Conversion | **Hybrid**: `pdftotext` for text, the LLM only on pages that need it | Pays for ~15-20% of pages instead of 100%, and the text stays exact instead of being re-guessed from a rendered image. |
| Model | **`qwen/qwen3.5-flash-02-23` via OpenRouter** | Parity with the retired script. Key read from `STUDY_OPENROUTER_API_KEY`; never a literal. |
| Generated `.md` | **In the Obsidian vault**, path passed with `--out` | The dashboard can read it and the plugin already parses `question::answer` from it. The backend never guesses the vault path. |

## Why hybrid, restated

Measured on the real book before deciding:

- `DDD.pdf` is **born-digital** (Producer `PStill`, 2003, from Word): its text layer
  is exact. `pdftotext` extracts 142,925 words instantly, for free.
- The LLM pipeline renders each page to a lossy JPEG and asks a model to
  re-transcribe text that was already perfect. That is a lossy round trip, and it
  is also 359 multimodal calls.
- What the LLM adds that no extractor can: **descriptions of diagrams** (155 of
  them in `DDD-full.md`, verified accurate against page 180) and reconstructed
  tables.

So: extract everywhere, render and transcribe only where extraction is weak or a
figure is present.

**Failure modes are not symmetric, and that is why the LLM is rationed.** An
extractor that fails produces visibly garbled text that matches no query. A model
that is unsure **invents confidently**, and a confident invention matches queries
and answers them wrongly. Rationing the model limits how much of the corpus can be
confidently wrong.

## The bug this step must not repeat

The retired script had no resumability. Its run died at page 179, the header still
claimed "Pages 1 to 359" because it wrote the range before processing, and the user
patched `paginaInicio = 180` by hand and concatenated two files. `DDD-full.md`
therefore contains **two `# Processed Book for RAG` headers**, one in the middle of
the book, which is a poisoned chunk in a RAG index.

A job with persisted per-page state makes a re-run continue instead of restart.

## Tasks

- [ ] T1 — Task file (this document)
- [x] T2 — `rag/domain`: `Document`, `IngestionJob`, `PageOutcome`, statuses, ports
- [x] T3 — Migration: `documents` and `ingestion_jobs`, versioned
- [x] T4 — `rag/sqlite`: repositories for documents and jobs
- [x] T5 — `rag/infra/pdf`: `pdfinfo` page count and `pdftotext` behind a port
- [x] T6 — "Needs the model" heuristic (sparse text, or a figure on the page)
- [ ] T7 — `platform/llm`: OpenRouter client behind a port, key from config
- [x] T8 — `rag/infra/pdf`: `pdftoppm` behind a port
- [ ] T9 — `rag/app`: `IngestDocumentUseCase`, resumable per page
- [ ] T10 — `cmd/ingest`: run the pipeline, write the `.md` to `--out`
- [ ] T11 — Verify against a real book, twice, to prove resumability
- [ ] T12 — README + backlog

## Slice 2 — migration + repositories (in progress)

Note: the session numbering and the slice numbering above differ. Slice 1 delivered
the job model (`rag/domain/job.go`); slice 2 is the migration and the repositories.

Decisions locked by the user (2026-10-17):

| Question | Choice | Why |
| --- | --- | --- |
| Migration numbering | **rag owns the 100-199 block** | `schema_version` is one global table keyed only by `version`, and the runner treats every version <= `max(version)` as applied. Disjoint blocks make a cross-context collision impossible and stop a single-context `Migrate` call from silently skipping the other context's migrations. |
| Document identity | **content sha256** (`documents.content_sha256 UNIQUE`) | If the PDF changed, it is a different document, so a resume can never mix pages from two versions of the file — the failure class that left two headers in `DDD-full.md`. The path is informational only, not a key. |
| Page text | **`job_pages.text`** | The `.md` is materialized by reading recorded pages in order, so a resume re-extracts nothing and can never emit a page twice. |

- [x] S2.1 `rag/domain`: `Document`, `PageOutcome.Text`, and the repository ports
- [x] S2.2 `rag/sqlite/migrations.go`: `documents`, `ingestion_jobs`, `job_pages` with primary key `(job_id, page_number)`
- [x] S2.3 `rag/sqlite`: document, job, and page-outcome repositories
- [x] S2.4 `cmd/server`: apply both contexts' migration lists
- [x] S2.5 Tests: adoption of an existing study database, one outcome per page, resume after interruption
- [x] S2.6 README + backlog, then commit the slice

## Slice 3 — extraction, heuristic, render (in progress)

The four poppler tools live in ONE package, `internal/rag/infra/pdf`, not in the
`infra/extract` and `infra/render` split this document originally suggested: they
are the same tool family, they share one injectable `Runner` and one error style,
and a two-package split would have duplicated both or needed a third package for
the runner. The ports they implement are still separate in `rag/domain`.

- [x] S3.1 `rag/domain`: `TextExtractor`, `FigureIndex`, `PageRenderer`, `NeedsModel`, `MinExtractableText`
- [x] S3.2 `rag/infra/pdf`: header-driven `pdfimages` parse, form-feed-stripping `pdftotext`, `pdfinfo` page count, transient `pdftoppm` renders
- [x] S3.3 A committed three-page fixture plus the generator that reproduces it (`testdata/make_fixture.go`)
- [x] S3.4 Tests: fake-runner parsers, process failures, fail-fast when poppler is absent, real-binary integration (skipped only when a tool is missing)
- [x] S3.5 README section, then commit the slice

Decisions taken while implementing, not asked about again:

| Question | Choice | Why |
| --- | --- | --- |
| Sparse-text threshold | **200 runes** (`MinExtractableText`) | The real test book yields roughly 2,400 characters per page, so 200 is an order of magnitude below a normal page. It catches headings and captions; it does not try to detect *garbled* text, which would need the model it is trying to ration. |
| Figure signal | **`pdfimages -list`, parsed header-driven** | It is the tool that knows which pages carry a raster image. A header-driven parse survives a column reorder; an unparseable row is an error, because a silent zero is indistinguishable from a book with no diagrams. |
| Render format and size | **PNG at 150 dpi, constant** | One legible resolution, no config knob. Renders are transient: written to a temp directory under the workspace and removed on every path, since retention (`P1`) does not exist. |
| Blank page | **Reaches the model** | Its text is under the threshold, so it is sent. Accepted waste (a handful of calls per book) instead of a third rule that special-cases a page carrying no signal. |

## Acceptance criteria

1. `gofmt`, `go vet`, all tests green; frontend untouched.
2. Ingestion is **resumable**: killing it mid-book and re-running does not redo
   completed pages and does not duplicate content in the output.
3. The output `.md` has **one** header, and its page markers reflect the range
   actually processed.
4. A page that fails does not abort the run: it is recorded as failed with its
   reason, and the job reports how many succeeded.
5. `pdftotext` and `pdftoppm` live behind ports, so the pipeline is testable
   without either binary and without network access.
6. No API key literal anywhere; a missing key fails loudly at startup rather than
   at page 200.
7. The generated `.md` on a digital book matches `pdftotext` for the text, proving
   the extractor path is not lossy.

## Out of scope

- Chunking, embeddings, search. That is step 5.
- Retention/GC (`P1` in the backlog).
- Any plugin UI.

## Session boundaries

This step is 3-4 sessions of work. Suggested slices, each independently
verifiable:

1. **Job model + migration + repositories** — no LLM, no external binary.
2. **Extraction + heuristic + renderer** — needs `pdftotext`/`pdftoppm`, no network.
3. **LLM client + orchestration + `cmd/ingest`** — first end-to-end run.
4. **Resumability verification on a real book + docs.**
