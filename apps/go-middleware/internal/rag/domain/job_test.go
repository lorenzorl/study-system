package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

var now = time.Date(2026, 10, 17, 15, 0, 0, 0, time.UTC)

// mustNewJob builds a job for tests whose inputs are known to be valid.
//
// It fails the test rather than discarding the error, so an unexpected validation
// failure reports what went wrong instead of panicking on a nil job.
func mustNewJob(t *testing.T, id, documentID string, totalPages int) *domain.IngestionJob {
	t.Helper()

	job, err := domain.NewJob(id, documentID, totalPages, now)
	require.NoError(t, err, "NewJob(%q, %q, %d)", id, documentID, totalPages)

	return job
}

func outcome(page int, status domain.PageStatus, at time.Time) domain.PageOutcome {
	return domain.PageOutcome{PageNumber: page, Status: status, UpdatedAt: at}
}

// --- NewJob ---

func TestNewJob_RejectsMissingIdentifiers(t *testing.T) {
	cases := []struct{ id, documentID string }{
		{"", "doc"},
		{"job", ""},
		{"", ""},
	}

	for _, tc := range cases {
		_, err := domain.NewJob(tc.id, tc.documentID, 10, now)

		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrInvalidInput)
	}
}

func TestNewJob_RejectsNonPositivePageCount(t *testing.T) {
	for _, pages := range []int{0, -1} {
		_, err := domain.NewJob("job", "doc", pages, now)

		require.Error(t, err, "page count %d must be rejected", pages)
		assert.ErrorIs(t, err, apperror.ErrInvalidInput)
	}
}

func TestNewJob_StartsPendingInUTC(t *testing.T) {
	local := time.FixedZone("TEST", -3*60*60)

	job, err := domain.NewJob("job-1", "doc-1", 359, now.In(local))
	require.NoError(t, err)

	assert.Equal(t, domain.JobPending, job.Status)
	assert.Equal(t, 359, job.TotalPages)
	assert.Nil(t, job.FinishedAt)
	assert.Equal(t, time.UTC, job.StartedAt.Location(), "instants must be UTC")
	assert.True(t, job.StartedAt.Equal(now))
}

// --- lifecycle ---

func TestMarkRunning_FromPending(t *testing.T) {
	job := mustNewJob(t, "job", "doc", 10)

	require.NoError(t, job.MarkRunning(now.Add(time.Minute)))

	assert.Equal(t, domain.JobRunning, job.Status)
	assert.Equal(t, now.Add(time.Minute), job.UpdatedAt)
}

// A resumed run starts from a job that is already running, so this must not fail.
func TestMarkRunning_IsIdempotentWhileRunning(t *testing.T) {
	job := mustNewJob(t, "job", "doc", 10)
	require.NoError(t, job.MarkRunning(now))

	require.NoError(t, job.MarkRunning(now.Add(time.Minute)),
		"resuming a running job must not be an error")
	assert.Equal(t, domain.JobRunning, job.Status)
}

func TestMarkRunning_RejectsAFinishedJob(t *testing.T) {
	for _, status := range []domain.JobStatus{domain.JobCompleted, domain.JobFailed} {
		job := mustNewJob(t, "job", "doc", 1)
		job.Status = status

		err := job.MarkRunning(now)

		require.Error(t, err, "a %s job must not restart", status)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	}
}

func TestSettle_RefusesWhilePagesArePending(t *testing.T) {
	job := mustNewJob(t, "job", "doc", 10)
	require.NoError(t, job.MarkRunning(now))

	err := job.Settle(domain.Progress{Total: 10, Extracted: 4, Pending: 6}, now)

	require.Error(t, err)
	assert.ErrorIs(t, err, apperror.ErrConflict)
	assert.Equal(t, domain.JobRunning, job.Status, "the job must stay open")
}

// Partial output is usually still worth keeping, so some success means completed.
func TestSettle_CompletesWhenSomePagesSucceeded(t *testing.T) {
	job := mustNewJob(t, "job", "doc", 10)
	require.NoError(t, job.MarkRunning(now))

	progress := domain.Progress{Total: 10, Extracted: 7, Failed: 3}
	require.NoError(t, job.Settle(progress, now.Add(time.Hour)))

	assert.Equal(t, domain.JobCompleted, job.Status)
	require.NotNil(t, job.FinishedAt)
	assert.Equal(t, now.Add(time.Hour), *job.FinishedAt)
}

func TestSettle_FailsWhenNothingSucceeded(t *testing.T) {
	job := mustNewJob(t, "job", "doc", 10)
	require.NoError(t, job.MarkRunning(now))

	require.NoError(t, job.Settle(domain.Progress{Total: 10, Failed: 10}, now))

	assert.Equal(t, domain.JobFailed, job.Status)
}

func TestSettle_IsIdempotent(t *testing.T) {
	job := mustNewJob(t, "job", "doc", 1)
	require.NoError(t, job.MarkRunning(now))

	progress := domain.Progress{Total: 1, Extracted: 1}
	require.NoError(t, job.Settle(progress, now))
	finished := *job.FinishedAt

	require.NoError(t, job.Settle(progress, now.Add(time.Hour)))

	assert.Equal(t, domain.JobCompleted, job.Status)
	assert.Equal(t, finished, *job.FinishedAt, "a second settle must not move the finish time")
}

func TestIsResumable(t *testing.T) {
	cases := map[domain.JobStatus]bool{
		domain.JobPending:   true,
		domain.JobRunning:   true,
		domain.JobCompleted: false,
		domain.JobFailed:    false,
	}

	for status, want := range cases {
		t.Run(string(status), func(t *testing.T) {
			job := &domain.IngestionJob{Status: status}
			assert.Equal(t, want, job.IsResumable())
		})
	}
}

// --- PendingPages: the resumability core ---

func TestPendingPages_FreshJobWorksEveryPage(t *testing.T) {
	pending := domain.PendingPages(3, nil)

	assert.Equal(t, []int{1, 2, 3}, pending)
}

func TestPendingPages_SkipsRecordedPages(t *testing.T) {
	outcomes := []domain.PageOutcome{
		outcome(1, domain.PageExtracted, now),
		outcome(2, domain.PageTranscribed, now),
		outcome(4, domain.PageExtracted, now),
	}

	pending := domain.PendingPages(5, outcomes)

	assert.Equal(t, []int{3, 5}, pending,
		"both extracted and transcribed pages are finished")
}

// A retry is the point of resuming, so a failed page comes back.
func TestPendingPages_RetriesFailedPages(t *testing.T) {
	outcomes := []domain.PageOutcome{
		outcome(1, domain.PageExtracted, now),
		outcome(2, domain.PageFailed, now),
		outcome(3, domain.PageFailed, now),
	}

	pending := domain.PendingPages(4, outcomes)

	assert.Equal(t, []int{2, 3, 4}, pending)
}

// This is the bug the retired script had: a run that died must not silently skip
// pages, and a stale row must not make the job look finished.
func TestPendingPages_IgnoresOutcomesOutsideTheDocument(t *testing.T) {
	outcomes := []domain.PageOutcome{
		outcome(0, domain.PageExtracted, now),
		outcome(-5, domain.PageExtracted, now),
		outcome(99, domain.PageExtracted, now),
	}

	pending := domain.PendingPages(2, outcomes)

	assert.Equal(t, []int{1, 2}, pending, "out-of-range rows must not mark pages done")
}

func TestPendingPages_ReturnsAscendingOrder(t *testing.T) {
	pending := domain.PendingPages(5, []domain.PageOutcome{
		outcome(2, domain.PageExtracted, now),
		outcome(5, domain.PageExtracted, now),
	})

	assert.Equal(t, []int{1, 3, 4}, pending)
}

// --- ProgressOf ---

func TestProgressOf_DerivesEveryCount(t *testing.T) {
	outcomes := []domain.PageOutcome{
		outcome(1, domain.PageExtracted, now),
		outcome(2, domain.PageExtracted, now),
		outcome(3, domain.PageTranscribed, now),
		outcome(4, domain.PageFailed, now),
	}

	progress := domain.ProgressOf(6, outcomes)

	assert.Equal(t, 6, progress.Total)
	assert.Equal(t, 2, progress.Extracted)
	assert.Equal(t, 1, progress.Transcribed)
	assert.Equal(t, 1, progress.Failed)
	assert.Equal(t, 2, progress.Pending)
	assert.Equal(t, 3, progress.Succeeded())
	assert.False(t, progress.Settled())
}

func TestProgressOf_EmptyIsAllPending(t *testing.T) {
	progress := domain.ProgressOf(4, nil)

	assert.Equal(t, 4, progress.Pending)
	assert.True(t, progress.Settled() == false)
	assert.Equal(t, 0, progress.Succeeded())
}

// A page recorded twice must count once, or a resumed run reports progress it
// does not have.
func TestProgressOf_DeduplicatesByPageKeepingTheLatest(t *testing.T) {
	outcomes := []domain.PageOutcome{
		outcome(1, domain.PageFailed, now),
		outcome(1, domain.PageExtracted, now.Add(time.Minute)),
	}

	progress := domain.ProgressOf(2, outcomes)

	assert.Equal(t, 1, progress.Extracted, "the later outcome wins")
	assert.Equal(t, 0, progress.Failed)
	assert.Equal(t, 1, progress.Pending)
}

func TestProgressOf_SettledWhenEveryPageHasAnOutcome(t *testing.T) {
	outcomes := []domain.PageOutcome{
		outcome(1, domain.PageExtracted, now),
		outcome(2, domain.PageFailed, now),
	}

	progress := domain.ProgressOf(2, outcomes)

	assert.True(t, progress.Settled())
	assert.Equal(t, 1, progress.Succeeded(), "a settled job can still be a partial success")
}

func TestProgressOf_IgnoresOutcomesOutsideTheDocument(t *testing.T) {
	outcomes := []domain.PageOutcome{
		outcome(1, domain.PageExtracted, now),
		outcome(500, domain.PageExtracted, now),
	}

	progress := domain.ProgressOf(2, outcomes)

	assert.Equal(t, 1, progress.Extracted, "an out-of-range row is not part of this document")
	assert.Equal(t, 1, progress.Pending)
}

// --- SortOutcomes ---

func TestSortOutcomes_OrdersByPageNumber(t *testing.T) {
	outcomes := []domain.PageOutcome{
		outcome(9, domain.PageExtracted, now),
		outcome(2, domain.PageExtracted, now),
		outcome(5, domain.PageExtracted, now),
	}

	domain.SortOutcomes(outcomes)

	assert.Equal(t, []int{2, 5, 9}, []int{
		outcomes[0].PageNumber, outcomes[1].PageNumber, outcomes[2].PageNumber,
	})
}
