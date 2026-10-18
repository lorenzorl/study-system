package app

import (
	"context"
	"fmt"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// DocumentCatalog is the slice of the document port the corpus list needs: every
// document, including one nobody has converted yet, in a deterministic order.
type DocumentCatalog interface {
	ListAll(ctx context.Context) ([]*domain.Document, error)
}

// PageCounter counts a document's recorded pages without reading their text.
type PageCounter interface {
	CountByDocument(ctx context.Context, documentID string) (int, error)
}

// DocumentSummary is one entry of the corpus list: what a caller needs to choose
// a document to attach and to see whether it finished processing.
//
// The server's internals — the artifact id, the source URI, the content hash —
// are deliberately absent: a caller does not choose by them, and they must not
// leave the server.
type DocumentSummary struct {
	ID        string
	Title     string
	CreatedAt time.Time
	Pages     int
	Chunks    int
}

// ListDocumentsUseCase builds the corpus list.
type ListDocumentsUseCase struct {
	documents DocumentCatalog
	pages     PageCounter
	chunks    domain.ChunkRepository
}

// NewListDocumentsUseCase wires the corpus list.
func NewListDocumentsUseCase(
	documents DocumentCatalog,
	pages PageCounter,
	chunks domain.ChunkRepository,
) *ListDocumentsUseCase {
	return &ListDocumentsUseCase{
		documents: documents,
		pages:     pages,
		chunks:    chunks,
	}
}

// Execute returns every document, newest first, with its page and chunk counts.
//
// The counts are aggregate reads, not lists: answering "how many" by loading the
// pages or the chunks would read a whole book per document. An empty corpus is an
// empty slice and never nil, so the transport encodes it as [].
func (uc *ListDocumentsUseCase) Execute(ctx context.Context) ([]DocumentSummary, error) {
	documents, err := uc.documents.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}

	summaries := make([]DocumentSummary, 0, len(documents))
	for _, document := range documents {
		pages, err := uc.pages.CountByDocument(ctx, document.ID)
		if err != nil {
			return nil, fmt.Errorf("count the pages of document %s: %w", document.ID, err)
		}

		chunks, err := uc.chunks.CountByDocument(ctx, document.ID)
		if err != nil {
			return nil, fmt.Errorf("count the chunks of document %s: %w", document.ID, err)
		}

		summaries = append(summaries, DocumentSummary{
			ID:        document.ID,
			Title:     document.Title,
			CreatedAt: document.CreatedAt,
			Pages:     pages,
			Chunks:    chunks,
		})
	}

	return summaries, nil
}
