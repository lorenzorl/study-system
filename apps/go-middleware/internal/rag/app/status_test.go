package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
)

// staticFailures is a hand-written FailureReporter with one fixed answer.
type staticFailures map[string]string

func (f staticFailures) LastFailure(documentID string) (string, bool) {
	message, ok := f[documentID]
	return message, ok
}

// noTextPages wraps the real page repository and fails the test if the status
// reads page text: the status must use the aggregate count.
type noTextPages struct {
	domain.PageOutcomeRepository
	listCalls int
}

func (p *noTextPages) ListOutcomes(context.Context, string) ([]domain.PageOutcome, error) {
	p.listCalls++
	return nil, errors.New("the status must not load page text")
}

func newStatusUseCase(h *indexHarness, failures app.FailureReporter) *app.DocumentStatusUseCase {
	return app.NewDocumentStatusUseCase(
		h.docs, h.jobs, h.pages, h.chunks,
		ragsqlite.NewEmbeddingRepository(h.sqlDB),
		failures,
		"test/model",
	)
}

// settleAsCompleted marks the document's latest job completed from its recorded
// outcomes.
func settleAsCompleted(t *testing.T, h *indexHarness, documentID string) *domain.IngestionJob {
	t.Helper()
	ctx := t.Context()

	job, err := h.jobs.FindLatestByDocumentID(ctx, documentID)
	require.NoError(t, err)
	outcomes, err := h.pages.ListOutcomes(ctx, job.ID)
	require.NoError(t, err)
	require.NoError(t, job.Settle(domain.ProgressOf(job.TotalPages, outcomes), indexInstant))
	require.NoError(t, h.jobs.UpdateStatus(ctx, job))

	return job
}

func TestDocumentStatusUseCase_UnknownDocumentIsNotFound(t *testing.T) {
	h := newIndexHarness(t)

	_, err := newStatusUseCase(h, nil).Execute(t.Context(), uuid.NewString())
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

func TestDocumentStatusUseCase_DocumentWithNoJobIsQueued(t *testing.T) {
	h := newIndexHarness(t)
	document := h.seedDocumentWithoutPages(t, "never run")

	status, err := newStatusUseCase(h, nil).Execute(t.Context(), document.ID)
	require.NoError(t, err)

	assert.Equal(t, app.StateQueued, status.State)
	assert.Equal(t, document.Title, status.Title)
	assert.Equal(t, 0, status.Chunks)
	assert.Equal(t, 0, status.Embedded)
	assert.Nil(t, status.StartedAt)
	assert.Nil(t, status.UpdatedAt)
	assert.Nil(t, status.FinishedAt)
	assert.Empty(t, status.Error)
}

func TestDocumentStatusUseCase_RunningJobIsConvertingWithPageCounts(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	document := h.seedRecordedPages(t, "un libro", map[int]string{1: "uno", 2: "dos", 3: "tres"})

	job, err := h.jobs.FindLatestByDocumentID(ctx, document.ID)
	require.NoError(t, err)
	require.NoError(t, job.MarkRunning(indexInstant))
	require.NoError(t, h.jobs.UpdateStatus(ctx, job))

	// One page failed, so the counts must come from the recorded outcomes.
	require.NoError(t, h.pages.SaveOutcome(ctx, domain.PageOutcome{
		JobID: job.ID, PageNumber: 3, Status: domain.PageFailed,
		Method: domain.MethodVision, UpdatedAt: indexInstant,
	}))

	status, err := newStatusUseCase(h, nil).Execute(ctx, document.ID)
	require.NoError(t, err)

	assert.Equal(t, app.StateConverting, status.State)
	assert.Equal(t, 3, status.Pages.Total)
	assert.Equal(t, 2, status.Pages.Extracted)
	assert.Equal(t, 0, status.Pages.Transcribed)
	assert.Equal(t, 1, status.Pages.Failed)
	assert.Equal(t, 0, status.Pages.Pending)
	require.NotNil(t, status.StartedAt)
	require.NotNil(t, status.UpdatedAt)
	assert.Nil(t, status.FinishedAt, "a running job has not finished")
}

func TestDocumentStatusUseCase_CompletedJobIsIndexingUntilEmbedded(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	document := h.seedRecordedPages(t, "un libro", map[int]string{1: "el agregado", 2: "el repositorio"})
	settleAsCompleted(t, h, document.ID)

	status, err := newStatusUseCase(h, nil).Execute(ctx, document.ID)
	require.NoError(t, err)
	// A completed job whose chunks have not been written yet is still indexing:
	// ingestion settles before the index stage runs, so "no chunk" is not "done".
	assert.Equal(t, app.StateIndexing, status.State)
	assert.Equal(t, 0, status.Chunks)

	_, err = h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	status, err = newStatusUseCase(h, nil).Execute(ctx, document.ID)
	require.NoError(t, err)
	assert.Equal(t, app.StateIndexing, status.State, "chunks exist but none is embedded yet")
	assert.Greater(t, status.Chunks, 0)
	assert.Equal(t, 0, status.Embedded)

	_, err = embedderFor(h, spanishToEnglishEmbedder(), "test/model").Execute(ctx, document.ID)
	require.NoError(t, err)

	status, err = newStatusUseCase(h, nil).Execute(ctx, document.ID)
	require.NoError(t, err)
	assert.Equal(t, app.StateCompleted, status.State)
	assert.Equal(t, status.Chunks, status.Embedded)
	require.NotNil(t, status.FinishedAt)
}

func TestDocumentStatusUseCase_FailedJobIsFailed(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	document := h.seedRecordedPages(t, "un libro", map[int]string{1: "uno"})

	job, err := h.jobs.FindLatestByDocumentID(ctx, document.ID)
	require.NoError(t, err)
	require.NoError(t, h.pages.SaveOutcome(ctx, domain.PageOutcome{
		JobID: job.ID, PageNumber: 1, Status: domain.PageFailed,
		Method: domain.MethodExtract, UpdatedAt: indexInstant,
	}))
	settleAsCompleted(t, h, document.ID)

	status, err := newStatusUseCase(h, nil).Execute(ctx, document.ID)
	require.NoError(t, err)
	assert.Equal(t, app.StateFailed, status.State)
	assert.Equal(t, 1, status.Pages.Failed)
}

func TestDocumentStatusUseCase_InProcessFailureWins(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	document := h.seedRecordedPages(t, "un libro", map[int]string{1: "uno"})
	settleAsCompleted(t, h, document.ID)
	_, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)
	_, err = embedderFor(h, spanishToEnglishEmbedder(), "test/model").Execute(ctx, document.ID)
	require.NoError(t, err)

	status, err := newStatusUseCase(h, staticFailures{document.ID: "the model is down"}).Execute(ctx, document.ID)
	require.NoError(t, err)

	assert.Equal(t, app.StateFailed, status.State, "a recorded in-process failure wins over a completed job")
	assert.Equal(t, "the model is down", status.Error)
}

func TestDocumentStatusUseCase_DoesNotLoadPageText(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	document := h.seedRecordedPages(t, "un libro", map[int]string{1: "uno", 2: "dos"})

	pages := &noTextPages{PageOutcomeRepository: h.pages}
	useCase := app.NewDocumentStatusUseCase(
		h.docs, h.jobs, pages, h.chunks,
		ragsqlite.NewEmbeddingRepository(h.sqlDB), nil, "test/model")

	status, err := useCase.Execute(ctx, document.ID)
	require.NoError(t, err)

	assert.Equal(t, 0, pages.listCalls, "the status reads aggregate counts, never page text")
	assert.Equal(t, 2, status.Pages.Total)
	assert.Equal(t, 2, status.Pages.Extracted)
}
