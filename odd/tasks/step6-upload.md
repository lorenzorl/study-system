# Step 6 — HTTP upload and processing status

**Status:** in progress
**Created:** 2026-10-17
**Depends on:** step 4 (documents, ingestion jobs, page outcomes), step 5
(chunks, embeddings, search), `platform/storage`, `platform/llm`,
`rag/infra/pdf`.

## Specs

**S1 — The request.** "crea un endpoint para subir el pdf, y otro para saber el
estado de procesamiento del libro."

**S2 — Processing model (user, 2026-10-17).** Asynchronous in the same process.
The upload request returns immediately with a `document_id`; an internal worker
runs the pipeline; the status endpoint is polled. The ingestion is resumable, so
a restart can be recovered by re-uploading the same bytes. Chosen over an
external worker (more machinery) and over a synchronous request (a request that
blocks for ~20 minutes is a request that fails).

**S3 — What "processing" includes (user, 2026-10-17).** Ingestion (PDF → page
text) plus the lexical index plus the embeddings, so an uploaded book is
hybrid-searchable when processing completes.

**S4 — Where the Markdown goes (user, 2026-10-17).**
`data/artifacts/<artifact-id>/book.md`, next to the stored PDF. No new
configuration.

**S5 — Constraints inherited from the system.** No new dependencies. The
ingestion command keeps working unchanged. The server still boots without an API
key (upload is the only part that needs one). Bodies are capped by
`STUDY_MAX_UPLOAD_BYTES`. Tests are hermetic: no network, no real key, no
poppler.

## Contract

### Endpoints

- **`POST /api/rag/documents`** — `multipart/form-data` with one `file` part.
  - `400` when the request is not multipart, has no `file` part, the part is
    empty, or the bytes do not start with `%PDF-`.
  - `413` when the body exceeds `STUDY_MAX_UPLOAD_BYTES` (the store enforces it).
  - `503` when the server has no `STUDY_OPENROUTER_API_KEY`, because it cannot
    transcribe a page and the pipeline would fail later.
  - `202` otherwise, with `{"document_id": "<id>", "state": "queued"}`.
  - The body is streamed into `storage.Store.Put` (never buffered whole in
    memory) and the SHA-256 is computed on the same stream. The document is
    found by content hash and created only when absent, so re-uploading the same
    bytes resumes the same document.
- **`GET /api/rag/documents/{documentID}/status`** — `200` with the status
  object below, `404` when the document is unknown.

### Status object

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

`state` is derived, never stored: a recorded in-process failure wins; then no job
means `queued`; a running or pending job means `converting`; a failed job means
`failed`; a completed job means `indexing` while it has no chunk yet or not every
chunk has a vector, then `completed`. The no-chunk case matters because ingestion
settles the job before the index stage writes the first chunk. `error`,
`started_at`, `updated_at` and `finished_at` are omitted when empty. Counts come
from aggregate reads, never from loading page text.

### New pieces

- `domain`: `ChunkRepository.CountByDocument(ctx, documentID) (int, error)` and
  `EmbeddingRepository.CountByDocument(ctx, documentID, model string) (int, error)`.
- `rag/app`: `ProcessDocumentUseCase` runs ingestion → index → embeddings in
  sequence for one document and reports what each stage produced.
  `BackgroundProcessor` runs it on a bounded pool of workers with a context
  derived from the server's, records the last failure per document in memory,
  and exposes `Wait()` for graceful shutdown. `DocumentStatusUseCase` builds the
  status object from the repositories plus the processor's last failure.
- `rag/http`: `POST /api/rag/documents` and
  `GET /api/rag/documents/{id}/status`, with the handlers declaring their own
  interfaces in `contracts.go`, like the rest of the context.
- `cmd/server`: builds the storage, the RAG repositories, the ingest/index/embed
  use cases and the background processor, mounts the routes, and drains the
  processor on shutdown.

### Known limitation

The in-flight/queued state of the processor and its last error live in memory.
A server restart loses them; the document and its recorded pages stay in the
database, so re-uploading the same bytes resumes from where it stopped.

## Tasks

- [x] T1 — `domain` + `sqlite`: the two per-document count reads, with tests (S5)
- [x] T2 — `rag/app`: `ProcessDocumentUseCase`, `BackgroundProcessor`,
      `DocumentStatusUseCase`, with tests (S2, S3)
- [x] T3 — `rag/http`: the upload handler and its tests (S1, S4, S5)
- [x] T4 — `rag/http`: the status handler and its tests (S1)
- [x] T5 — `cmd/server`: wiring, shutdown drain, README, backlog (S5)
- [x] T6 — live end-to-end verification and commit (S1-S5)

## Log

**L1 (2026-10-17, request)** — the user asked for two endpoints: "crea un
endpoint para subir el pdf, y otro para saber el estado de procesamiento del
libro." After the exploration of the ingestion and storage code, the user chose
the three design decisions quoted in S2, S3 and S4.

**L2 (2026-10-17, verified live and committed)** — implemented by a bounded writer
(one run timed out after writing the layer, a second finished the wiring) and then
verified live by the parent. `gofmt -l`, `go build ./...`, `go vet ./...` clean;
`go test ./... -count=1` ok in every package; `func Test` went from 447 at
`1202a45` to 476 (+29).

Live run against a copy of the real database and a temporary storage root:
`POST /api/rag/documents` with the three-page fixture returned `202`
`{"document_id":"3bb6aa16-…","state":"queued"}`. Polling
`GET /api/rag/documents/3bb6aa16-…/status` showed `queued` → `converting`
(`pending` 3→0) → `indexing` (`chunks` 3, `embedded` 0) → `completed`
(`embedded` 3) in about 40 s. The final object carried 3 pages, 3 transcribed,
0 failed, 3 chunks and 3 embeddings with timestamps, and `book.md` was written to
`data/artifacts/<artifact-id>/book.md`. Searching `Figure` through the command
returned `three-pages.pdf` page 3 with cosine 0.725, so the uploaded book was
hybrid-searchable when processing completed.

One defect was found and fixed during verification: a completed job with zero
chunks read `completed`, because the contract said "until `embedded >= chunks`".
That window is real — ingestion settles the job before the index stage writes the
first chunk — so the derivation now reads `indexing` while `chunks == 0` or
`embedded < chunks`, and the test was updated. With no API key the server still
boots and answers search and status; only the upload answers `503`.
