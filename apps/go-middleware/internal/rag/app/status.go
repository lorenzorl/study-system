package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// DocumentState is the derived processing state of a document. It is never
// stored: it is a pure function of the job, the counts, and the processor's last
// failure, so it cannot drift out of agreement with them.
type DocumentState string

const (
	// StateQueued means no job exists: the document was registered but no page
	// has been attempted.
	StateQueued DocumentState = "queued"

	// StateConverting means a pending or running job is producing page text.
	StateConverting DocumentState = "converting"

	// StateIndexing means ingestion completed but not every chunk has a vector.
	StateIndexing DocumentState = "indexing"

	// StateCompleted means ingestion completed and every chunk has a vector.
	StateCompleted DocumentState = "completed"

	// StateFailed means the job failed, or a recorded in-process failure won.
	StateFailed DocumentState = "failed"
)

// FailureReporter reports the last in-process failure for a document, if any. It
// is the background processor's read side.
type FailureReporter interface {
	LastFailure(documentID string) (string, bool)
}

// DocumentStatus is one document's processing status, built from the
// repositories and the processor's last failure.
type DocumentStatus struct {
	DocumentID string
	Title      string
	State      DocumentState
	Pages      domain.Progress
	Chunks     int
	Embedded   int
	Error      string
	StartedAt  *time.Time
	UpdatedAt  *time.Time
	FinishedAt *time.Time
}

// DocumentStatusUseCase builds the status object the polling endpoint returns.
//
// Every count is an aggregate read: the status answers "how many", and loading
// the page text or the chunk text to answer it would make one poll read a whole
// book.
type DocumentStatusUseCase struct {
	documents  domain.DocumentRepository
	jobs       domain.JobRepository
	pages      domain.PageOutcomeRepository
	chunks     domain.ChunkRepository
	embeddings domain.EmbeddingRepository
	failures   FailureReporter
	model      string
}

// NewDocumentStatusUseCase wires the status read. failures may be nil, which
// means no in-process failure is known.
func NewDocumentStatusUseCase(
	documents domain.DocumentRepository,
	jobs domain.JobRepository,
	pages domain.PageOutcomeRepository,
	chunks domain.ChunkRepository,
	embeddings domain.EmbeddingRepository,
	failures FailureReporter,
	model string,
) *DocumentStatusUseCase {
	return &DocumentStatusUseCase{
		documents:  documents,
		jobs:       jobs,
		pages:      pages,
		chunks:     chunks,
		embeddings: embeddings,
		failures:   failures,
		model:      model,
	}
}

// Execute builds one document's status. An unknown document is
// apperror.ErrNotFound, which the transport maps to 404.
func (uc *DocumentStatusUseCase) Execute(ctx context.Context, documentID string) (DocumentStatus, error) {
	document, err := uc.documents.FindByID(ctx, documentID)
	if err != nil {
		return DocumentStatus{}, err
	}

	status := DocumentStatus{
		DocumentID: document.ID,
		Title:      document.Title,
	}

	if status.Chunks, err = uc.chunks.CountByDocument(ctx, documentID); err != nil {
		return DocumentStatus{}, fmt.Errorf("count the chunks of document %s: %w", documentID, err)
	}
	if status.Embedded, err = uc.embeddings.CountByDocument(ctx, documentID, uc.model); err != nil {
		return DocumentStatus{}, fmt.Errorf("count the embeddings of document %s: %w", documentID, err)
	}

	job, err := uc.jobs.FindLatestByDocumentID(ctx, documentID)
	switch {
	case errors.Is(err, apperror.ErrNotFound):
		// No job: nothing has been attempted, which the derived state reports as
		// queued.
	case err != nil:
		return DocumentStatus{}, fmt.Errorf("look up the latest job of document %s: %w", documentID, err)
	default:
		counts, countErr := uc.pages.CountByJob(ctx, job.ID)
		if countErr != nil {
			return DocumentStatus{}, fmt.Errorf("count the pages of job %s: %w", job.ID, countErr)
		}
		status.Pages = domain.Progress{
			Total:       job.TotalPages,
			Extracted:   counts.Extracted,
			Transcribed: counts.Transcribed,
			Failed:      counts.Failed,
			Pending:     job.TotalPages - counts.Extracted - counts.Transcribed - counts.Failed,
		}

		started, updated := job.StartedAt, job.UpdatedAt
		status.StartedAt = &started
		status.UpdatedAt = &updated
		status.FinishedAt = job.FinishedAt
	}

	failed := false
	if uc.failures != nil {
		if message, ok := uc.failures.LastFailure(documentID); ok {
			status.Error = message
			failed = true
		}
	}

	status.State = deriveDocumentState(status, job, failed)
	return status, nil
}

// deriveDocumentState applies the specified derivation: a recorded in-process
// failure wins; then no job means queued; a running or pending job means
// converting; a failed job means failed; a completed job means indexing while it
// has no chunk yet or not every chunk has a vector, then completed.
//
// The no-chunk case is not the same as "nothing to index": ingestion settles the
// job before indexing writes the first chunk, so a completed job with zero chunks
// is a pipeline that has not reached the index stage yet, not one that is done.
func deriveDocumentState(status DocumentStatus, job *domain.IngestionJob, failed bool) DocumentState {
	if failed {
		return StateFailed
	}
	if job == nil {
		return StateQueued
	}

	switch job.Status {
	case domain.JobPending, domain.JobRunning:
		return StateConverting
	case domain.JobFailed:
		return StateFailed
	case domain.JobCompleted:
		if status.Chunks == 0 || status.Embedded < status.Chunks {
			return StateIndexing
		}
		return StateCompleted
	default:
		return StateQueued
	}
}
