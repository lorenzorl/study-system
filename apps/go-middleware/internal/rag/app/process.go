package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// markdownFileName is where a processed document's Markdown lands inside its own
// artifact directory, next to the stored PDF (S4). It is a constant rather than
// configuration: the directory belongs to one document, so the name cannot
// collide with another document's, and no new knob is needed.
const markdownFileName = "book.md"

// ProcessReport summarises one document's full pipeline run, stage by stage, so
// a caller can see what each stage produced rather than only the final counts.
type ProcessReport struct {
	// DocumentID is the document that was processed.
	DocumentID string

	// Ingest is the conversion stage's report. It is nil when the stage did not
	// reach a plan.
	Ingest *Report

	// Chunks is how many chunks the index stage wrote.
	Chunks int

	// Embeddings is how many vectors the embedding stage wrote.
	Embeddings int
}

// ProcessDocumentUseCase runs the whole pipeline for one already-registered
// document: ingestion, then the lexical index, then the embeddings.
//
// The document is registered by the caller (the upload endpoint) before this
// runs, so the use case starts from the stored payload and never stores it
// again. Running the three stages in one use case is what makes an uploaded book
// hybrid-searchable when processing completes (S3), and it is the unit the
// background processor runs.
type ProcessDocumentUseCase struct {
	documents domain.DocumentRepository
	store     *storage.Store
	ingest    *IngestDocumentUseCase
	index     *IndexDocumentUseCase
	embed     *EmbedDocumentUseCase
}

// NewProcessDocumentUseCase wires the full pipeline for one document.
func NewProcessDocumentUseCase(
	documents domain.DocumentRepository,
	store *storage.Store,
	ingest *IngestDocumentUseCase,
	index *IndexDocumentUseCase,
	embed *EmbedDocumentUseCase,
) *ProcessDocumentUseCase {
	return &ProcessDocumentUseCase{
		documents: documents,
		store:     store,
		ingest:    ingest,
		index:     index,
		embed:     embed,
	}
}

// Execute runs the three stages in order. The first failure stops the pipeline
// and is returned, so a partially processed document is reported as failed
// rather than as done.
func (uc *ProcessDocumentUseCase) Execute(ctx context.Context, documentID string) (ProcessReport, error) {
	report := ProcessReport{DocumentID: documentID}

	document, err := uc.documents.FindByID(ctx, documentID)
	if err != nil {
		return report, err
	}

	payloadPath, err := uc.store.Path(document.ArtifactID)
	if err != nil {
		return report, fmt.Errorf("resolve the stored payload of document %s: %w", documentID, err)
	}
	dir, err := uc.store.Directory(document.ArtifactID)
	if err != nil {
		return report, fmt.Errorf("resolve the artifact directory of document %s: %w", documentID, err)
	}

	// The ingestion run finds this document by its content hash and resumes it,
	// so the bytes the upload stored are what it renders from.
	ingested, err := uc.ingest.Execute(ctx, IngestRequest{
		PDFPath:   payloadPath,
		Title:     document.Title,
		SourceURI: document.SourceURI,
		OutPath:   filepath.Join(dir, markdownFileName),
	}, Options{}, Hooks{})
	if err != nil {
		return report, fmt.Errorf("ingest document %s: %w", documentID, err)
	}
	report.Ingest = ingested

	indexed, err := uc.index.Execute(ctx, documentID)
	if err != nil {
		return report, fmt.Errorf("index document %s: %w", documentID, err)
	}
	report.Chunks = indexed.Chunks

	embedded, err := uc.embed.Execute(ctx, documentID)
	if err != nil {
		return report, fmt.Errorf("embed document %s: %w", documentID, err)
	}
	report.Embeddings = embedded.Embeddings

	return report, nil
}
