// Package domain holds the ingestion context's entities, its invariants, and the
// ports its adapters implement.
//
// The defining concern of this context is resumability. The retired
// optimizebooks script had none: its run died at page 179, it had already written
// a header claiming "Pages 1 to 359", and the book was finished by hand-editing a
// page number and concatenating two files. That left two headers in the middle of
// the output, which is a poisoned chunk in a retrieval index.
package domain

import (
	"fmt"
	"sort"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
)

// JobStatus is the lifecycle state of an ingestion run.
type JobStatus string

const (
	// JobPending means the job exists but no page has been attempted.
	JobPending JobStatus = "pending"

	// JobRunning means at least one page has been attempted and the run has not
	// been declared finished.
	JobRunning JobStatus = "running"

	// JobCompleted means every page reached a terminal state, at least one of them
	// successfully.
	JobCompleted JobStatus = "completed"

	// JobFailed means every attempted page failed. A partially successful run is
	// completed, not failed: the caller decides whether partial is good enough.
	JobFailed JobStatus = "failed"
)

// PageStatus is the state of a single page within a job.
type PageStatus string

const (
	// PageExtracted means the text came from the PDF's own text layer.
	PageExtracted PageStatus = "extracted"

	// PageTranscribed means the text came from the model, because extraction was
	// weak or the page carried a figure.
	PageTranscribed PageStatus = "transcribed"

	// PageFailed means the page produced nothing usable. A failed page is retried
	// by the next run rather than skipped.
	PageFailed PageStatus = "failed"
)

// PageMethod records which path produced a page's text, so a run can be audited
// and so the cost of a re-run is predictable.
type PageMethod string

const (
	MethodExtract PageMethod = "extract"
	MethodVision  PageMethod = "vision"
)

// IngestionJob is one run of the pipeline over one document.
//
// It deliberately does NOT cache per-page counts. Progress is derived from the
// recorded page outcomes, so there is no denormalised counter to drift out of
// agreement with the pages it summarises. That drift is exactly what makes a
// naive resume skip pages or redo them.
type IngestionJob struct {
	ID         string
	DocumentID string
	Status     JobStatus
	TotalPages int

	StartedAt  time.Time
	UpdatedAt  time.Time
	FinishedAt *time.Time
}

// PageOutcome is the recorded result for one page. It is the unit of
// resumability: a page with an outcome is not attempted again.
type PageOutcome struct {
	JobID      string
	PageNumber int
	Status     PageStatus
	Method     PageMethod
	Error      string
	UpdatedAt  time.Time
}

// Progress summarises a job's pages. Every field is derived from recorded
// outcomes.
type Progress struct {
	Total       int
	Extracted   int
	Transcribed int
	Failed      int
	Pending     int
}

// Settled reports whether every page reached a terminal state.
func (p Progress) Settled() bool { return p.Pending == 0 }

// Succeeded is the number of pages that produced usable text.
func (p Progress) Succeeded() int { return p.Extracted + p.Transcribed }

// NewJob starts a job for a document.
func NewJob(id, documentID string, totalPages int, now time.Time) (*IngestionJob, error) {
	if id == "" || documentID == "" {
		return nil, fmt.Errorf("job id and document id are required: %w", apperror.ErrInvalidInput)
	}
	if totalPages < 1 {
		return nil, fmt.Errorf("total pages must be positive, got %d: %w", totalPages, apperror.ErrInvalidInput)
	}

	instant := now.UTC()
	return &IngestionJob{
		ID:         id,
		DocumentID: documentID,
		Status:     JobPending,
		TotalPages: totalPages,
		StartedAt:  instant,
		UpdatedAt:  instant,
	}, nil
}

// MarkRunning moves a fresh job into the running state. Calling it again is a
// no-op, because a resumed run starts from a job that is already running.
func (j *IngestionJob) MarkRunning(now time.Time) error {
	switch j.Status {
	case JobRunning:
		return nil
	case JobPending:
	default:
		return fmt.Errorf("cannot start a job in state %q: %w", j.Status, apperror.ErrConflict)
	}

	j.Status = JobRunning
	j.UpdatedAt = now.UTC()
	return nil
}

// Settle closes the job once every page has a terminal outcome.
//
// A run where some pages failed but others succeeded is Completed: partial output
// is usually still worth keeping, and the caller can inspect Progress. Failed is
// reserved for a run where nothing usable came out at all.
func (j *IngestionJob) Settle(progress Progress, now time.Time) error {
	if !progress.Settled() {
		return fmt.Errorf("cannot settle with %d pages pending: %w", progress.Pending, apperror.ErrConflict)
	}
	if j.Status == JobCompleted || j.Status == JobFailed {
		return nil
	}

	instant := now.UTC()
	if progress.Succeeded() > 0 {
		j.Status = JobCompleted
	} else {
		j.Status = JobFailed
	}
	j.UpdatedAt = instant
	j.FinishedAt = &instant
	return nil
}

// IsResumable reports whether the run left work behind.
func (j *IngestionJob) IsResumable() bool {
	return j.Status == JobRunning || j.Status == JobPending
}

// PendingPages returns the 1-based page numbers that still need work, ascending.
//
// A page that failed is pending again: a retry is the point of resuming. A page
// with any other outcome is finished and must NOT be redone, because redoing it
// would duplicate its text in the output.
//
// Recorded outcomes for page numbers outside the document are ignored rather than
// trusted, so a stale row cannot make the job look finished when it is not.
func PendingPages(totalPages int, outcomes []PageOutcome) []int {
	done := make(map[int]bool, len(outcomes))
	for _, outcome := range outcomes {
		if outcome.PageNumber < 1 || outcome.PageNumber > totalPages {
			continue
		}
		if outcome.Status == PageFailed {
			continue
		}
		done[outcome.PageNumber] = true
	}

	pending := make([]int, 0, totalPages-len(done))
	for page := 1; page <= totalPages; page++ {
		if !done[page] {
			pending = append(pending, page)
		}
	}
	return pending
}

// ProgressOf derives progress from recorded outcomes.
//
// Outcomes are deduplicated by page number, keeping the last one, so a page
// recorded twice cannot inflate a count. This is the invariant that keeps a
// resumed run honest.
func ProgressOf(totalPages int, outcomes []PageOutcome) Progress {
	latest := make(map[int]PageOutcome, len(outcomes))
	for _, outcome := range outcomes {
		if outcome.PageNumber < 1 || outcome.PageNumber > totalPages {
			continue
		}
		existing, seen := latest[outcome.PageNumber]
		if !seen || !outcome.UpdatedAt.Before(existing.UpdatedAt) {
			latest[outcome.PageNumber] = outcome
		}
	}

	progress := Progress{Total: totalPages}
	for _, outcome := range latest {
		switch outcome.Status {
		case PageExtracted:
			progress.Extracted++
		case PageTranscribed:
			progress.Transcribed++
		case PageFailed:
			progress.Failed++
		}
	}

	progress.Pending = totalPages - len(latest)
	return progress
}

// SortOutcomes orders outcomes by page number, for stable output.
func SortOutcomes(outcomes []PageOutcome) {
	sort.Slice(outcomes, func(i, j int) bool {
		return outcomes[i].PageNumber < outcomes[j].PageNumber
	})
}
