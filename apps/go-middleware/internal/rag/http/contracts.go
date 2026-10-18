package http

import (
	"context"
	"io"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// The HTTP layer declares the use case contracts it needs, rather than
// importing concrete use case types. That keeps handlers mockable and lets the
// application layer change shape without breaking transport tests. This mirrors
// study/http/contracts.go.

// SearchUseCase answers a query over the chunk index.
//
// Both the lexical and the hybrid use cases satisfy it: the difference is which
// stages they run, not the contract the transport sees.
type SearchUseCase interface {
	// Search returns the chunks matching query over the whole corpus, best first.
	// A query of only punctuation and a query that matches nothing both return an
	// empty slice and no error, so they are a 200 with an empty array and never a
	// 400.
	Search(ctx context.Context, query string, limit int) ([]domain.SearchResult, error)

	// SearchInDocument returns the chunks of one document matching query, best
	// first. An empty documentID is the whole corpus, exactly as Search. A
	// document that matches nothing is a 200 with an empty array, not a 404 and
	// not a 400: it is the same empty answer a query with no match returns.
	SearchInDocument(ctx context.Context, query, documentID string, limit int) ([]domain.SearchResult, error)
}

// MaxSearchLimit bounds the `limit` query parameter.
//
// A request is a caller this process does not control, so the limit is bounded
// here rather than passed through: without a bound, one client could ask for the
// whole index and make every search answer with the corpus. Absent or empty uses
// app.DefaultSearchLimit.
const MaxSearchLimit = 50

// UploadDocumentStore persists an uploaded payload.
//
// It is deliberately narrow: the handler streams bytes and learns the artifact,
// and the store keeps every other decision — where the file lands, how large it
// may be — to itself.
type UploadDocumentStore interface {
	Put(ctx context.Context, originalName string, content io.Reader) (storage.Artifact, error)
}

// UploadDocumentRepository resolves the document for uploaded bytes. The method
// set is the one app.FindOrCreateDocument needs, so the shared registration rule
// is what actually creates the row.
type UploadDocumentRepository interface {
	FindByContentHash(ctx context.Context, contentSHA256 string) (*domain.Document, error)
	Create(ctx context.Context, document *domain.Document) error
}

// DocumentProcessor queues a registered document for background processing.
//
// Submit must not block: the upload request returns as soon as the document is
// queued, which is the asynchronous model (S2).
type DocumentProcessor interface {
	Submit(documentID string)
}

// DocumentStatusUseCase builds a document's processing status.
type DocumentStatusUseCase interface {
	Execute(ctx context.Context, documentID string) (app.DocumentStatus, error)
}

// DocumentListUseCase lists the corpus with the counts a caller needs to choose a
// document to attach.
type DocumentListUseCase interface {
	// Execute returns every document, newest first, with its page and chunk
	// counts. An empty corpus is an empty slice, never nil, so the transport
	// encodes it as [].
	Execute(ctx context.Context) ([]app.DocumentSummary, error)
}
