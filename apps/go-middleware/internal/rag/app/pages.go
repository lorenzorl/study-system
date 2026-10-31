package app

import (
	"context"
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// PageRangeCap is the largest number of pages one request may ask for.
//
// The endpoint is the door to a 359-page book of roughly 917 KB of text, so the
// cap is part of the contract rather than an implementation detail: without it a
// single request could pull a whole book's text over a phone connection.
const PageRangeCap = 50

// PageRangeError is a refused page range, carrying the sentence the caller
// reads.
//
// The bound it names is data the caller's handler does not have — the document's
// last page lives in its latest job — so the refusal is worded where that bound
// is known. It unwraps to apperror.ErrInvalidInput, so a caller that only knows
// the sentinels still reports the same 400.
type PageRangeError struct {
	message string
}

// Error is the caller-facing refusal.
func (e *PageRangeError) Error() string { return e.message }

// Unwrap makes every refused range an invalid input to any caller that classifies
// by sentinel.
func (e *PageRangeError) Unwrap() error { return apperror.ErrInvalidInput }

// ValidatePageRange enforces the range contract: pages are 1-based, the range runs
// forward, one call asks for at most PageRangeCap pages, and it may not run past
// the document's last page.
//
// totalPages is a parameter rather than a lookup so the transport can refuse an
// oversized request before anything is read, and the use case can refuse the range
// a document cannot satisfy. A totalPages of 0 means the count is not known yet,
// and then the last-page rule is not applied.
func ValidatePageRange(from, to, totalPages int) error {
	switch {
	case from < 1:
		return &PageRangeError{message: "from must be at least 1"}
	case to < from:
		return &PageRangeError{message: fmt.Sprintf("to must be at least from (%d)", from)}
	case to-from+1 > PageRangeCap:
		return &PageRangeError{message: fmt.Sprintf(
			"at most %d pages can be requested per call", PageRangeCap)}
	case totalPages > 0 && to > totalPages:
		return &PageRangeError{message: fmt.Sprintf(
			"to must be at most %d, the document's last page", totalPages)}
	}
	return nil
}

// PageTextRepository is the slice of the page store this read needs: the recorded
// outcomes inside one range of one job.
//
// The range belongs to the port rather than being applied to a full listing in Go,
// for the same reason the document filter does in ChunkSearchRepository: only the
// store can apply it before the rows are read, and a filter applied here would
// load a whole book to answer a twelve-page request.
type PageTextRepository interface {
	// ListPageTextsInRange returns the recorded outcomes of jobID whose page
	// number is between from and to, page number ascending, as an empty slice when
	// the job recorded none in that range. A page with no recorded outcome is
	// absent: a gap is not a failure.
	ListPageTextsInRange(ctx context.Context, jobID string, from, to int) ([]domain.PageOutcome, error)
}

// PageText is one page of the answer: its citation and what the run left on it.
type PageText struct {
	// PageNumber is the 1-based page the text belongs to.
	PageNumber int

	// Status is the outcome the run recorded: extracted, transcribed, or failed.
	Status domain.PageStatus

	// Method is how the text was obtained.
	Method domain.PageMethod

	// Text is the page's text, verbatim, and empty for a failed page.
	Text string

	// Error is why the page produced no usable text, and empty otherwise.
	Error string
}

// DocumentPages is one document's recorded page text over a range: everything a
// caller needs to propose a card and cite it.
type DocumentPages struct {
	DocumentID string
	Title      string

	// TotalPages is the document's page count as its latest job recorded it, which
	// is what bounds a range and what a citation is checked against.
	TotalPages int

	// Pages is the recorded pages of the range, ascending by page number. A page
	// the run never recorded is absent, so this may be shorter than the range.
	Pages []PageText
}

// DocumentPagesUseCase answers the text of a document's page range.
//
// It reads through the latest job because page text lives on the job, not on the
// document: a re-run replaces the outcomes it touches, and the latest run is the
// one whose text is current.
type DocumentPagesUseCase struct {
	documents domain.DocumentRepository
	jobs      domain.JobRepository
	pages     PageTextRepository
}

// NewDocumentPagesUseCase wires the page-range read.
func NewDocumentPagesUseCase(
	documents domain.DocumentRepository,
	jobs domain.JobRepository,
	pages PageTextRepository,
) *DocumentPagesUseCase {
	return &DocumentPagesUseCase{documents: documents, jobs: jobs, pages: pages}
}

// Execute returns the recorded text of the requested range.
//
// An unknown document, and a document whose ingestion never ran, are both
// apperror.ErrNotFound: with no job there is no recorded text and no page count, so
// the range cannot be answered and "not found" is the honest answer rather than an
// empty book.
func (uc *DocumentPagesUseCase) Execute(
	ctx context.Context,
	documentID string,
	from, to int,
) (DocumentPages, error) {
	document, err := uc.documents.FindByID(ctx, documentID)
	if err != nil {
		return DocumentPages{}, err
	}

	job, err := uc.jobs.FindLatestByDocumentID(ctx, documentID)
	if err != nil {
		return DocumentPages{}, err
	}

	// The range is validated before the query, so a refused request never reads a
	// page. The count comes from the job because that is where "how long is this
	// document" is recorded.
	if err := ValidatePageRange(from, to, job.TotalPages); err != nil {
		return DocumentPages{}, err
	}

	outcomes, err := uc.pages.ListPageTextsInRange(ctx, job.ID, from, to)
	if err != nil {
		return DocumentPages{}, fmt.Errorf(
			"read pages %d to %d of job %s: %w", from, to, job.ID, err)
	}

	// make with len guarantees an empty range encodes as [] and never as null: a
	// caller ranges over this list, and "the pages are not recorded yet" must not
	// be a case it has to special-case.
	pages := make([]PageText, 0, len(outcomes))
	for _, outcome := range outcomes {
		pages = append(pages, PageText{
			PageNumber: outcome.PageNumber,
			Status:     outcome.Status,
			Method:     outcome.Method,
			Text:       outcome.Text,
			Error:      outcome.Error,
		})
	}

	return DocumentPages{
		DocumentID: document.ID,
		Title:      document.Title,
		TotalPages: job.TotalPages,
		Pages:      pages,
	}, nil
}
