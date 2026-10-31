package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// errNotUsed marks the repository methods the page read never calls, so a use
// case that reaches for one fails loudly instead of silently reading something
// else.
var errNotUsed = errors.New("not used by the page read")

// fakePageDocuments answers FindByID from a fixed set.
type fakePageDocuments map[string]*domain.Document

func (d fakePageDocuments) Create(context.Context, *domain.Document) error { return errNotUsed }

func (d fakePageDocuments) FindByID(_ context.Context, id string) (*domain.Document, error) {
	document, ok := d[id]
	if !ok {
		return nil, apperror.ErrNotFound
	}
	return document, nil
}

func (d fakePageDocuments) FindByContentHash(context.Context, string) (*domain.Document, error) {
	return nil, errNotUsed
}

func (d fakePageDocuments) ListIndexable(context.Context) ([]*domain.Document, error) {
	return nil, errNotUsed
}

// fakePageJobs answers FindLatestByDocumentID from a fixed set.
type fakePageJobs map[string]*domain.IngestionJob

func (j fakePageJobs) Create(context.Context, *domain.IngestionJob) error { return errNotUsed }

func (j fakePageJobs) FindByID(context.Context, string) (*domain.IngestionJob, error) {
	return nil, errNotUsed
}

func (j fakePageJobs) FindLatestByDocumentID(
	_ context.Context, documentID string,
) (*domain.IngestionJob, error) {
	job, ok := j[documentID]
	if !ok {
		return nil, apperror.ErrNotFound
	}
	return job, nil
}

func (j fakePageJobs) UpdateStatus(context.Context, *domain.IngestionJob) error { return errNotUsed }

// fakePageRange answers ListPageTextsInRange and records what it was asked, so a
// refused range can prove the query was never issued.
type fakePageRange struct {
	outcomes []domain.PageOutcome
	err      error
	calls    int
	gotJobID string
	gotFrom  int
	gotTo    int
}

func (p *fakePageRange) ListPageTextsInRange(
	_ context.Context, jobID string, from, to int,
) ([]domain.PageOutcome, error) {
	p.calls++
	p.gotJobID, p.gotFrom, p.gotTo = jobID, from, to
	if p.err != nil {
		return nil, p.err
	}
	return p.outcomes, nil
}

// pageRead is one wired use case over the three fakes.
type pageRead struct {
	useCase   *app.DocumentPagesUseCase
	rangeFake *fakePageRange
}

// newPageRead wires the use case for one document with one job of totalPages.
func newPageRead(
	documentID, title string, totalPages int, outcomes []domain.PageOutcome,
) pageRead {
	job := &domain.IngestionJob{ID: "job-1", DocumentID: documentID, TotalPages: totalPages}
	rangeFake := &fakePageRange{outcomes: outcomes}

	return pageRead{
		useCase: app.NewDocumentPagesUseCase(
			fakePageDocuments{documentID: {ID: documentID, Title: title}},
			fakePageJobs{documentID: job},
			rangeFake,
		),
		rangeFake: rangeFake,
	}
}

func TestDocumentPagesUseCase_ReturnsTheRequestedRange(t *testing.T) {
	read := newPageRead("doc-1", "DDD.pdf", 359, []domain.PageOutcome{
		{JobID: "job-1", PageNumber: 12, Status: domain.PageExtracted, Method: domain.MethodExtract, Text: "el agregado"},
		{JobID: "job-1", PageNumber: 14, Status: domain.PageFailed, Method: domain.MethodVision, Error: "no text"},
	})

	pages, err := read.useCase.Execute(t.Context(), "doc-1", 12, 14)
	require.NoError(t, err)

	assert.Equal(t, "doc-1", pages.DocumentID)
	assert.Equal(t, "DDD.pdf", pages.Title)
	assert.Equal(t, 359, pages.TotalPages)
	require.Len(t, pages.Pages, 2)
	assert.Equal(t, app.PageText{
		PageNumber: 12,
		Status:     domain.PageExtracted,
		Method:     domain.MethodExtract,
		Text:       "el agregado",
	}, pages.Pages[0])
	assert.Equal(t, app.PageText{
		PageNumber: 14,
		Status:     domain.PageFailed,
		Method:     domain.MethodVision,
		Error:      "no text",
	}, pages.Pages[1])

	assert.Equal(t, "job-1", read.rangeFake.gotJobID, "the latest job's pages are the ones read")
	assert.Equal(t, 12, read.rangeFake.gotFrom)
	assert.Equal(t, 14, read.rangeFake.gotTo)
}

func TestDocumentPagesUseCase_RefusesARangeOutsideTheContract(t *testing.T) {
	tests := []struct {
		name    string
		from    int
		to      int
		message string
	}{
		{name: "from below the first page", from: 0, to: 2, message: "from must be at least 1"},
		{name: "to before from", from: 5, to: 4, message: "to must be at least from (5)"},
		{
			name: "over the page cap", from: 1, to: 51,
			message: "at most 50 pages can be requested per call",
		},
		{
			name: "past the document's last page", from: 340, to: 360,
			message: "to must be at most 359, the document's last page",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			read := newPageRead("doc-1", "DDD.pdf", 359, nil)

			_, err := read.useCase.Execute(t.Context(), "doc-1", test.from, test.to)

			require.ErrorIs(t, err, apperror.ErrInvalidInput)

			var refusal *app.PageRangeError
			require.ErrorAs(t, err, &refusal)
			assert.Equal(t, test.message, refusal.Error())
			assert.Zero(t, read.rangeFake.calls, "a refused range is never queried")
		})
	}
}

func TestDocumentPagesUseCase_UnknownDocumentIsNotFound(t *testing.T) {
	read := newPageRead("doc-1", "DDD.pdf", 10, nil)

	_, err := read.useCase.Execute(t.Context(), "doc-2", 1, 2)

	assert.ErrorIs(t, err, apperror.ErrNotFound)
	assert.Zero(t, read.rangeFake.calls)
}

// A document whose ingestion never ran has no recorded text and no page count,
// so the range cannot be answered: it is the same not-found as an unknown
// document, because to a caller the two are equally not there.
func TestDocumentPagesUseCase_DocumentWithNoJobIsNotFound(t *testing.T) {
	read := newPageRead("doc-1", "DDD.pdf", 10, nil)
	read.useCase = app.NewDocumentPagesUseCase(
		fakePageDocuments{"doc-1": {ID: "doc-1", Title: "DDD.pdf"}},
		fakePageJobs{},
		read.rangeFake,
	)

	_, err := read.useCase.Execute(t.Context(), "doc-1", 1, 2)

	assert.ErrorIs(t, err, apperror.ErrNotFound)
	assert.Zero(t, read.rangeFake.calls)
}

// A range the job recorded nothing in is an empty answer, not a miss: the pages
// exist, they simply have no outcome yet.
func TestDocumentPagesUseCase_RangeWithNoRecordedPageIsEmpty(t *testing.T) {
	read := newPageRead("doc-1", "DDD.pdf", 359, nil)

	pages, err := read.useCase.Execute(t.Context(), "doc-1", 1, 3)
	require.NoError(t, err)

	assert.NotNil(t, pages.Pages, "an empty range encodes as [], never as null")
	assert.Empty(t, pages.Pages)
	assert.Equal(t, 359, pages.TotalPages)
}
