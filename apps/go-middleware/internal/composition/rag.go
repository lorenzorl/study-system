package composition

import (
	"database/sql"
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	ragapp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/infra/pdf"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
)

// RAG is the RAG context's repositories, wired to one database.
//
// The repositories are unexported and the use cases are built on demand, because
// a RAG use case needs something the repositories cannot supply: a store, a
// transcriber, an embedder, or the processor. Each factory below takes exactly
// the outside dependencies its use case needs, so a command that only searches
// never has to build a store, and a dry run never has to build a client.
//
// The fields hold the concrete adapters, not the domain ports. The application
// layer declares narrower ports of its own — ChunkSearchRepository adds
// SearchInDocument, DocumentCatalog adds ListAll, PageCounter adds CountByDocument
// — and several of those live on a different adapter than their domain port. The
// concrete type satisfies every one of them, and this package is the place that is
// allowed to know which adapter it holds.
type RAG struct {
	documents  *ragsqlite.DocumentRepository
	jobs       *ragsqlite.JobRepository
	pages      *ragsqlite.PageOutcomeRepository
	chunks     *ragsqlite.ChunkRepository
	embeddings *ragsqlite.EmbeddingRepository
}

// NewRAG wires the RAG context's repositories over an open database.
func NewRAG(sqlDB *sql.DB) *RAG {
	return &RAG{
		documents:  ragsqlite.NewDocumentRepository(sqlDB),
		jobs:       ragsqlite.NewJobRepository(sqlDB),
		pages:      ragsqlite.NewPageOutcomeRepository(sqlDB),
		chunks:     ragsqlite.NewChunkRepository(sqlDB),
		embeddings: ragsqlite.NewEmbeddingRepository(sqlDB),
	}
}

// Search builds the search use case: hybrid when an embedding client exists,
// lexical-only when it does not.
//
// This is the one place that rule lives. It was previously repeated at two call
// sites, where the lexical branch could drift from the hybrid one without either
// failing.
//
// The embedding model is required even on the lexical path, where it is not read.
// It is the client that chooses the branch, not the caller: passing the model
// unconditionally is what keeps this decision out of the two entrypoints, and it
// is why the parameter is named for the role it plays rather than left as a bare
// "model".
func (r *RAG) Search(clients Clients, embeddingModel string) *ragapp.SearchUseCase {
	if clients.Embedding == nil {
		return ragapp.NewSearchUseCase(r.chunks)
	}
	return ragapp.NewHybridSearchUseCase(r.chunks, r.embeddings, clients.Embedding, embeddingModel)
}

// Ingest builds the conversion run.
//
// A nil store and no model client are the dry-run shape: the use case plans
// against the database and the PDF but persists nothing and calls nobody. That is
// why the transcriber is narrowed from the client field here rather than being
// passed as an interface: a nil *llm.Client stored in a domain.PageTranscriber
// would pass a nil check inside the use case.
func (r *RAG) Ingest(clients Clients, store *storage.Store) *ragapp.IngestDocumentUseCase {
	var transcriber domain.PageTranscriber
	if clients.Model != nil {
		transcriber = clients.Model
	}

	return ragapp.NewIngestDocumentUseCase(
		r.documents,
		r.jobs,
		r.pages,
		// A nil runner selects the real poppler binaries, so a missing tool fails
		// at Open rather than at the page that first needs it.
		pdf.NewSource(nil),
		transcriber,
		store,
	)
}

// Index builds the lexical index rebuild. It needs the database and nothing
// else: no API key, no PDF, no store.
func (r *RAG) Index() *ragapp.IndexDocumentUseCase {
	return ragapp.NewIndexDocumentUseCase(r.documents, r.jobs, r.pages, r.chunks)
}

// Embed builds the embedding rebuild.
//
// It refuses rather than narrowing. There is no lexical fallback for writing a
// vector, so a use case built without an embedder would not fail here — it would
// fail deep inside a run, as a nil dereference, after the caller believed it had
// a working stage. Ingest and Status narrow because they have a meaningful
// no-client behaviour; this one does not, so the absence is an error the caller
// has to handle.
func (r *RAG) Embed(clients Clients, embeddingModel string) (*ragapp.EmbedDocumentUseCase, error) {
	if clients.Embedding == nil {
		return nil, fmt.Errorf(
			"%s is not set: embedding a document calls the model and cannot run without it: %w",
			config.EnvOpenRouterAPIKey, apperror.ErrInvalidInput)
	}

	return ragapp.NewEmbedDocumentUseCase(
		r.documents, r.chunks, r.embeddings, clients.Embedding, embeddingModel), nil
}

// Process builds the full pipeline for one already-registered document:
// ingestion, then the index, then the embeddings.
//
// It requires a model client for the same reason Embed does, and reports its
// absence here rather than letting a nil dereference surface mid-run. The rest of
// the wiring is the three stages this package already builds.
func (r *RAG) Process(
	clients Clients, embeddingModel string, store *storage.Store,
) (*ragapp.ProcessDocumentUseCase, error) {
	embed, err := r.Embed(clients, embeddingModel)
	if err != nil {
		return nil, err
	}

	return ragapp.NewProcessDocumentUseCase(
		r.documents,
		store,
		r.Ingest(clients, store),
		r.Index(),
		embed,
	), nil
}

// Status builds the processing-status read.
//
// The processor is taken as its concrete type on purpose. The status use case
// declares a FailureReporter interface and checks it for nil, but a nil
// *BackgroundProcessor stored in that interface is not == nil, so the check would
// pass and the first status read would panic. Narrowing here, while the value is
// still concrete, is what keeps "no processor" meaning "no known failure" — the
// case of a process that never ran the pipeline.
func (r *RAG) Status(processor *ragapp.BackgroundProcessor, embeddingModel string) *ragapp.DocumentStatusUseCase {
	var failures ragapp.FailureReporter
	if processor != nil {
		failures = processor
	}

	return ragapp.NewDocumentStatusUseCase(
		r.documents, r.jobs, r.pages, r.chunks, r.embeddings, failures, embeddingModel)
}

// ListDocuments builds the corpus list: every document with its page and chunk
// counts.
func (r *RAG) ListDocuments() *ragapp.ListDocumentsUseCase {
	return ragapp.NewListDocumentsUseCase(r.documents, r.pages, r.chunks)
}

// DocumentRepo returns the document repository, which the upload endpoint needs
// directly.
//
// It is the one repository exposed, and it is exposed because the registration of
// uploaded bytes is a write the transport performs itself: the handler stores the
// payload, then calls app.FindOrCreateDocument over this repository, so the
// "already ingested these bytes" rule has one owner. Returning the adapter rather
// than a port keeps the composition package honest about what it holds; the
// handler still declares the narrow interface it accepts.
func (r *RAG) DocumentRepo() *ragsqlite.DocumentRepository {
	return r.documents
}
