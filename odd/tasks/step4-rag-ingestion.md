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
- [x] T7 — `platform/llm`: OpenRouter client behind a port, key from config
- [x] T8 — `rag/infra/pdf`: `pdftoppm` behind a port
- [x] T9 — `rag/app`: `IngestDocumentUseCase`, resumable per page
- [x] T10 — `cmd/ingest`: run the pipeline, write the `.md` to `--out`
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

## Slice 4 — model client and pipeline (in progress)

Decisions taken here. An implementer must not re-decide them.

| Question | Choice | Why |
| --- | --- | --- |
| Where the key is required | `config.Load` reads `STUDY_OPENROUTER_API_KEY` as **optional**; `cmd/ingest` requires it and fails before page 1, naming the variable | The server never ingests. Requiring it in `Load` would stop the server booting for a key it does not use. "A missing key fails loudly at startup, not at page 200" is about the ingestion run. |
| Base URL | `STUDY_OPENROUTER_BASE_URL`, default `https://openrouter.ai/api/v1` | Read and used. It is the only way to point a run at a proxy, a gateway, or a test double, and the end-to-end verification needs it. |
| Model | `STUDY_LLM_MODEL`, default `qwen/qwen3.5-flash-02-23` | Verified against the live models API: present, `input_modalities` includes `image`. |
| Jobs per document | **One job per document, continued across runs.** A new job is created only when the document has none | The per-page outcomes are the single source of truth for that document's conversion and `job_pages` is keyed `(job_id, page_number)`. A job per run would re-extract every page of a book to resume one. |
| When the `.md` is written | **Only once every page has a terminal outcome** | A partial file whose header claims the full range is exactly the retired script's bug: `DDD-full.md` carries two headers. An interrupted run writes nothing, so the previous complete file stays valid. |
| Page failure | Recorded as a failed outcome with its reason, and the run continues | Acceptance criterion 4. A failed page is owed again by the next run, which is the domain rule. |
| Image inventory failure | **Aborts the run** | `pdfimages` failing means we do not know which pages carry figures. Treating it as zero images is the silent degradation the header-driven parse exists to prevent. |
| Transient failures | The model client retries 429 and 5xx up to 3 attempts with exponential backoff, never other 4xx | A rate limit in the middle of a 359-page run must not cost a page a manual re-run. A 400/401/402 is not transient and retrying it only wastes the run. |
| Output write | Temp file in the target directory, then rename | The vault file must never be observed half-written. |
| Text structure | **No passes, no cleanup, no heading inference in step 4.** Text is stored verbatim | It is what makes the output match `pdftotext` (criterion 7) and lets any later transformation run over stored text without re-extracting. |

Verified OpenRouter facts (measured; do not re-derive):

- `POST https://openrouter.ai/api/v1/chat/completions` with `Authorization: Bearer <key>` and `Content-Type: application/json`.
- Body: `{"model": "...", "messages": [{"role": "user", "content": [{"type": "text", "text": "..."}, {"type": "image_url", "image_url": {"url": "data:image/png;base64,<...>"}}]}]}`. `messages` is the only required field; `model` is always sent.
- Response text: `choices[0].message.content`. An empty `choices` array is an error, never an empty page.
- Errors: `{"error": {"message": "...", "code": 401}}`. Measured live (401 without a key). The returned error must carry the status and that message.
- Optional app-identity headers exist (`HTTP-Referer`, `X-OpenRouter-Title`); not sent in v1.

- [x] S4.1 `platform/config`: the three variables, defaults, and URL validation that names the variable
- [x] S4.2 `platform/llm`: OpenRouter client behind a one-method port, bounded retry, error mapping
- [x] S4.3 `rag/domain`: the transcriber port and `RenderMarkdown`
- [x] S4.4 `rag/infra/pdf`: a `Source` factory opening the three page ports over one file, with a nil runner so the fail-fast check stays
- [x] S4.5 `rag/app`: `IngestDocumentUseCase` — hash, find-or-create the document, find-or-continue the job, process the owed pages, settle, materialize
- [x] S4.6 `cmd/ingest`: `-out`, one progress line per page, summary, exit codes
- [x] S4.7 README + backlog, then commit the slice

### Contracts

`rag/domain` gains, alongside the existing ports:

```go
type PageTranscriber interface {
	Transcribe(ctx context.Context, prompt string, imagePNG []byte) (string, error)
}
```

`platform/llm.Client` satisfies it implicitly: the port is declared by the context that needs it, and the platform package knows nothing about `rag`. One method, no shared types, no import in either direction.

`rag/domain.RenderMarkdown(header, outcomes) string` is the only place the output document's shape is decided:

```md
# Processed Book for RAG

- source: <title>
- pages: 1-<total> (<recorded> recorded, <failed> failed)
- generated: <RFC3339 UTC>

<!-- page 1 -->
<verbatim text>

<!-- page 2 (failed) -->
```

Exactly one `# ` header; exactly one `<!-- page N -->` marker per page from 1 to the job's total, ascending; a failed page is marked `(failed)` and carries no text; page text is emitted verbatim (trailing whitespace trimmed) so it still matches `pdftotext`.

The prompt lives in `rag/app`, because what to ask the model is domain knowledge, while how to reach it is transport:

```text
You are converting ONE page of a book into Markdown for a retrieval index.

Transcribe the page's text exactly as printed: same wording, same order. Do not
summarise, do not paraphrase, do not explain, do not add commentary.
Do not reproduce running headers, footers, or page numbers.
For every diagram, figure, chart, or image, add a short factual description as a
blockquote line starting with "> Figure:".
Reproduce tables as Markdown tables.
Never invent content you cannot read. If a region is illegible, write [illegible]
instead of guessing.
This is page %d of %d. Output only the Markdown for the page, with no preamble.
```

The last paragraph is the whole point of the hybrid design: a confident invention matches queries and answers them wrongly, so the model is told to mark what it cannot read rather than fill it in.

`IngestDocumentUseCase`:

```go
type IngestRequest struct {
	PDFPath   string // the local file to convert
	Title     string
	SourceURI string // informational, recorded on first sight of the bytes
	OutPath   string // where the Markdown goes; the vault, never guessed by the backend
}

type PageEvent struct {
	Page, Total int
	Status      domain.PageStatus
	Method      domain.PageMethod
	Chars       int
	Elapsed     time.Duration
	Err         string
}

type Report struct {
	DocumentID, JobID, OutPath         string
	TotalPages, Extracted, Transcribed int
	Failed, Skipped                    int
	Settled                            bool
	Duration                           time.Duration
}
```

Flow, in order: hash the file (sha256, streamed) → `FindByContentHash` → on a miss, `store.Put` and `Create` the document, on a hit reuse the existing document and **do not store the bytes again** → `store.Path`/`Workspace` for the payload and the render scratch → `source.Open(payloadPath, workDir)` → `FindLatestByDocumentID`, continuing an existing job and creating one only when there is none (`MarkRunning` only from `pending`: a settled job is continued without it, because the domain refuses to move a finished job back to running) → `ListOutcomes` and `PendingPages` → fetch `ImageCounts` once, before the loop → for each owed page: extract, decide with `NeedsModel`, render and transcribe when it says so, `SaveOutcome`, report through the callback → re-read outcomes, `ProgressOf`, `Settle` when settled → materialize and write the file only when settled.

The use case takes `*storage.Store` directly rather than a new port: `app` is allowed to import `platform`, the store is the real thing in tests too (rooted at a temp dir), and inventing an `ArtifactLocator` interface to hide two method calls would add a seam nothing needs.

`cmd/ingest` requires `-out` for a conversion run, prints one line per page (`page 90/359  vision  3.2s`), then a summary with the counts, and exits non-zero when the run aborts or when nothing succeeded. The key is validated before the database is opened or the first page is read.

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

## Log

**L1 (2026-10-17, request summary; the verbatim text lives in the Engram mirror
under `odd/step4-rag-ingestion/tasks`)** — hybrid conversion (`pdftotext` for the
text, the model only on difficult or illustrated pages), model
`qwen/qwen3.5-flash-02-23` through OpenRouter with the key in
`STUDY_OPENROUTER_API_KEY`, and the generated `.md` written into the Obsidian
vault through `--out`, never guessed by the backend. End goal: ingest and search.

**L2 (2026-10-17, corrections found while verifying slice 4)** — two real defects,
each fixed with a failing test observed first:

- `failedOutcome` left `UpdatedAt` at the zero value, so a failed page was
  persisted with the year-0001 instant in the audit column.
- A freshly created job was persisted as `pending` and stayed pending while its
  pages were being processed. It is now created already `running`, because the
  domain defines `pending` as "no page has been attempted".

Two findings of mine were withdrawn after measuring, and both are worth keeping:
a page-count check that compared a per-document count against a global one, and a
claimed vector-figure defect that rendering the page disproved (backlog A2).

**L3 (2026-10-17, end-to-end evidence, with no real key needed)** — the command was
run against three real downloaded PDFs using a local double of the
chat-completions endpoint:

- A text-only paper of 19 pages made 0 model calls, and the emitted page text is
  byte-identical to `pdftotext` (acceptance criterion 7).
- The same input twice created no second document and no second job, reported 19
  pages already recorded, and produced an identical body — only the `generated:`
  line differs.
- A SIGKILL four seconds into the book with figures left **no** output file, with
  `job_pages` holding only the finished pages and the job in `running`; the re-run
  continued it, recorded the rest, and wrote one header with one marker per page.
- A page whose provider failed three times was recorded failed with the reason,
  the run continued and settled anyway, and the next run retried only that page,
  whose success replaced the failure.

Still unproven after L3: the real 359-page book (T11).

**L4 (2026-10-17, the real API run and the hardening it motivated)** — the two
runs in L3 were repeated against the real OpenRouter API (no double, real key):
15 pages (13 extracted, 2 to the model) in 1m51s and 18 pages (13 + 5) in 5m42s,
both settled with zero failures. Measured: **US$ 0,0093 for the seven model
calls** (US$ 0,0013 per page), and **31s to 1m41s per call**. The money is not
the constraint; the time is, which is why the plan now reports an estimate.
Quality spot-checked: 93% to 99% of each model page's vocabulary matches
`pdftotext`, the missing words are the template boilerplate and running headers
the prompt tells the model to drop, the added words are figure descriptions and
text read from inside figures (which `pdftotext` cannot see at all), and the
diagram description of *Attention Is All You Need*'s Figure 1 was verified
against the rendered page and is accurate.

Three decisions taken by the user, then implemented and verified at the command
line against the real PDFs:

| Question | Choice |
| --- | --- |
| Editing a generated `.md` by hand | Never assumed: a target carrying the generated header is rewritten as normal, one without it is refused, and `-force` replaces it deliberately |
| Warning before paying for the model | A plan printed before anything is written, `-dry-run` that stops there, and `-max-model-pages N` that refuses on the plan |
| Permissions of the output | 0644, because a vault may be synced by another user |

The refusal had to be moved in front of acquisition: it originally happened after
storing the artifact and creating the document and job, so "refuse before
spending" still left a stored PDF and an empty job behind. Planning now reads the
local file and the database only.

Still unproven: the real 359-page book, killed and resumed (T11), and whether its
155 diagrams are raster images the figure signal can see (backlog A2).
