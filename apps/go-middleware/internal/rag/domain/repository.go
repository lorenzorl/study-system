package domain

import "context"

// DocumentRepository manages persistence of Document entities.
type DocumentRepository interface {
	// Create inserts a document. A content hash that is already present is a
	// conflict: the caller should have found it with FindByContentHash and
	// resumed, and silently creating a second document for the same bytes would
	// split one book's history across two rows.
	Create(ctx context.Context, document *Document) error

	// FindByID returns a document by its primary key, or apperror.ErrNotFound.
	FindByID(ctx context.Context, id string) (*Document, error)

	// FindByContentHash returns the document for an already-acquired content
	// hash, or apperror.ErrNotFound when those bytes have never been ingested.
	// This is the lookup that decides resume or start fresh.
	FindByContentHash(ctx context.Context, contentSHA256 string) (*Document, error)
}

// JobRepository manages persistence of IngestionJob entities.
type JobRepository interface {
	// Create inserts a job. An unknown document is apperror.ErrNotFound.
	Create(ctx context.Context, job *IngestionJob) error

	// FindByID returns a job by its primary key, or apperror.ErrNotFound.
	FindByID(ctx context.Context, id string) (*IngestionJob, error)

	// FindLatestByDocumentID returns the most recently created job for a
	// document, which is the one a re-run resumes. It returns
	// apperror.ErrNotFound when the document has never been run.
	FindLatestByDocumentID(ctx context.Context, documentID string) (*IngestionJob, error)

	// UpdateStatus persists the lifecycle fields after a domain transition
	// (MarkRunning, Settle). Progress is deliberately not persisted: it is
	// derived from page outcomes, so there is no counter to drift.
	UpdateStatus(ctx context.Context, job *IngestionJob) error
}

// PageOutcomeRepository manages the recorded per-page outcomes of a job.
type PageOutcomeRepository interface {
	// SaveOutcome records a page's outcome, replacing any earlier outcome for the
	// same page.
	//
	// The replace is the point, not an implementation detail: retrying a failed
	// page is what resuming means, so the page's row is updated rather than
	// appended. The primary key (job_id, page_number) is what keeps exactly one
	// outcome per page, which is what makes the derived progress honest.
	//
	// An unknown job is apperror.ErrNotFound.
	SaveOutcome(ctx context.Context, outcome PageOutcome) error

	// ListOutcomes returns every recorded outcome of a job, ordered by page
	// number ascending. Reading them all is the point: the output document is
	// materialized from recorded text, so a resumed run needs the pages it did
	// not touch just as much as the ones it did.
	ListOutcomes(ctx context.Context, jobID string) ([]PageOutcome, error)
}
