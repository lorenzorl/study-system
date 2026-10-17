package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
)

// instant is a fixed instant. Tests that pinned the clock to "now" would be
// permitted to pass on a machine whose zone or precision handling is wrong.
var instant = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

// ragRepos is the set of adapters under test, over one database.
type ragRepos struct {
	documents domain.DocumentRepository
	jobs      domain.JobRepository
	pages     domain.PageOutcomeRepository
	chunks    domain.ChunkRepository
}

// openRepos opens an in-memory database with the ingestion schema applied.
//
// In-memory SQLite is private to the connection that created it, which is why
// platform/db pins the pool to a single connection for memory DSNs: every
// repository here must see the same schema.
func openRepos(t *testing.T) (*sql.DB, ragRepos) {
	t.Helper()

	ctx := t.Context()
	sqlDB, err := db.Open(ctx, ":memory:")
	require.NoError(t, err, "failed to open in-memory test database")
	t.Cleanup(func() { sqlDB.Close() })

	require.NoError(t, db.Migrate(ctx, sqlDB, ragsqlite.Migrations()), "failed to apply ingestion migrations")

	return sqlDB, ragRepos{
		documents: ragsqlite.NewDocumentRepository(sqlDB),
		jobs:      ragsqlite.NewJobRepository(sqlDB),
		pages:     ragsqlite.NewPageOutcomeRepository(sqlDB),
		chunks:    ragsqlite.NewChunkRepository(sqlDB),
	}
}

func seedDocument(t *testing.T, ctx context.Context, repo domain.DocumentRepository, seed string) *domain.Document {
	t.Helper()

	document := &domain.Document{
		ID:            uuid.NewString(),
		Title:         "seed-" + seed,
		ArtifactID:    uuid.NewString(),
		SourceURI:     "/books/" + seed + ".pdf",
		ContentSHA256: testHash(seed),
		CreatedAt:     instant,
	}
	require.NoError(t, repo.Create(ctx, document))

	return document
}

func seedJob(
	t *testing.T,
	ctx context.Context,
	repos ragRepos,
	documentID string,
	totalPages int,
	startedAt time.Time,
) *domain.IngestionJob {
	t.Helper()

	job, err := domain.NewJob(uuid.NewString(), documentID, totalPages, startedAt)
	require.NoError(t, err)
	require.NoError(t, repos.jobs.Create(ctx, job))

	return job
}

// outcome builds a page outcome carrying text, so a round trip can be checked on
// content rather than on counts alone.
func outcome(jobID string, page int, status domain.PageStatus, text string, at time.Time) domain.PageOutcome {
	method := domain.MethodExtract
	if status == domain.PageTranscribed {
		method = domain.MethodVision
	}

	return domain.PageOutcome{
		JobID:      jobID,
		PageNumber: page,
		Status:     status,
		Method:     method,
		Text:       text,
		UpdatedAt:  at,
	}
}

func TestDocumentRepository_CreateAndFind(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	document := seedDocument(t, ctx, repos.documents, "ddd")

	byID, err := repos.documents.FindByID(ctx, document.ID)
	require.NoError(t, err)
	assert.Equal(t, document.ID, byID.ID)
	assert.Equal(t, document.Title, byID.Title)
	assert.Equal(t, document.ArtifactID, byID.ArtifactID)
	assert.Equal(t, document.SourceURI, byID.SourceURI)
	assert.Equal(t, document.ContentSHA256, byID.ContentSHA256)
	assert.True(t, document.CreatedAt.Equal(byID.CreatedAt), "created_at must survive the round trip")

	byHash, err := repos.documents.FindByContentHash(ctx, document.ContentSHA256)
	require.NoError(t, err)
	assert.Equal(t, document.ID, byHash.ID, "the content hash is the lookup a resume performs")
}

func TestDocumentRepository_UnknownLookupsAreNotFound(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	_, err := repos.documents.FindByID(ctx, uuid.NewString())
	assert.ErrorIs(t, err, apperror.ErrNotFound)

	_, err = repos.documents.FindByContentHash(ctx, testHash("never-ingested"))
	assert.ErrorIs(t, err, apperror.ErrNotFound, "unknown bytes are a miss, not a new document")
}

func TestDocumentRepository_DuplicateContentHashIsAConflict(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	document := seedDocument(t, ctx, repos.documents, "ddd")

	duplicate := &domain.Document{
		ID:            uuid.NewString(),
		Title:         "the same book, ingested again",
		ArtifactID:    uuid.NewString(),
		SourceURI:     "/elsewhere/DDD.pdf",
		ContentSHA256: document.ContentSHA256,
		CreatedAt:     instant,
	}
	err := repos.documents.Create(ctx, duplicate)
	assert.ErrorIs(t, err, apperror.ErrConflict,
		"the same bytes must not become two documents: the caller should have resumed instead")

	var count int
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM documents`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestJobRepository_CreateAndFindByID(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	document := seedDocument(t, ctx, repos.documents, "ddd")
	job := seedJob(t, ctx, repos, document.ID, 359, instant)

	loaded, err := repos.jobs.FindByID(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, document.ID, loaded.DocumentID)
	assert.Equal(t, domain.JobPending, loaded.Status)
	assert.Equal(t, 359, loaded.TotalPages)
	assert.True(t, job.StartedAt.Equal(loaded.StartedAt))
	assert.True(t, job.UpdatedAt.Equal(loaded.UpdatedAt))
	assert.Nil(t, loaded.FinishedAt, "a job that has not finished has no finish instant")
	assert.True(t, loaded.IsResumable())
}

func TestJobRepository_UnknownJobIsNotFound(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	_, err := repos.jobs.FindByID(ctx, uuid.NewString())
	assert.ErrorIs(t, err, apperror.ErrNotFound)

	document := seedDocument(t, ctx, repos.documents, "ddd")
	_, err = repos.jobs.FindLatestByDocumentID(ctx, document.ID)
	assert.ErrorIs(t, err, apperror.ErrNotFound, "a document nobody has run has no job to resume")
}

func TestJobRepository_CreateForAnUnknownDocumentIsNotFound(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	job, err := domain.NewJob(uuid.NewString(), uuid.NewString(), 10, instant)
	require.NoError(t, err)

	err = repos.jobs.Create(ctx, job)
	assert.ErrorIs(t, err, apperror.ErrNotFound,
		"the caller named a document that does not exist, so the foreign key is reported as a miss")
}

func TestJobRepository_UpdateStatusPersistsTheLifecycle(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	document := seedDocument(t, ctx, repos.documents, "ddd")
	job := seedJob(t, ctx, repos, document.ID, 3, instant)

	runningAt := instant.Add(time.Minute)
	require.NoError(t, job.MarkRunning(runningAt))
	require.NoError(t, repos.jobs.UpdateStatus(ctx, job))

	loaded, err := repos.jobs.FindByID(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobRunning, loaded.Status)
	assert.True(t, runningAt.Equal(loaded.UpdatedAt))
	assert.Nil(t, loaded.FinishedAt, "a running job has not finished")

	outcomes := []domain.PageOutcome{
		outcome(job.ID, 1, domain.PageExtracted, "one", instant),
		outcome(job.ID, 2, domain.PageTranscribed, "two", instant),
		outcome(job.ID, 3, domain.PageExtracted, "three", instant),
	}
	settledAt := instant.Add(2 * time.Minute)
	require.NoError(t, job.Settle(domain.ProgressOf(3, outcomes), settledAt))
	require.NoError(t, repos.jobs.UpdateStatus(ctx, job))

	loaded, err = repos.jobs.FindByID(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobCompleted, loaded.Status)
	require.NotNil(t, loaded.FinishedAt)
	assert.True(t, settledAt.Equal(*loaded.FinishedAt))
	assert.True(t, settledAt.Equal(loaded.UpdatedAt))
	assert.False(t, loaded.IsResumable())
}

func TestJobRepository_UpdateStatusOnAnUnknownJobIsNotFound(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	job, err := domain.NewJob(uuid.NewString(), uuid.NewString(), 10, instant)
	require.NoError(t, err)

	assert.ErrorIs(t, repos.jobs.UpdateStatus(ctx, job), apperror.ErrNotFound)
}

// Stored instants carry whole seconds, so two jobs really can share a started_at.
// The resume lookup must still be unambiguous, and it is the tiebreaker that
// makes it so.
func TestJobRepository_FindLatestByDocumentIDPrefersTheMostRecentJob(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	document := seedDocument(t, ctx, repos.documents, "ddd")
	seedJob(t, ctx, repos, document.ID, 10, instant)
	second := seedJob(t, ctx, repos, document.ID, 10, instant)

	latest, err := repos.jobs.FindLatestByDocumentID(ctx, document.ID)
	require.NoError(t, err)
	assert.Equal(t, second.ID, latest.ID,
		"with equal started_at values the later insert is the newer job")

	third := seedJob(t, ctx, repos, document.ID, 10, instant.Add(time.Hour))

	latest, err = repos.jobs.FindLatestByDocumentID(ctx, document.ID)
	require.NoError(t, err)
	assert.Equal(t, third.ID, latest.ID)
}

func TestJobRepository_FindLatestIsScopedToOneDocument(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	first := seedDocument(t, ctx, repos.documents, "ddd")
	other := seedDocument(t, ctx, repos.documents, "clean-code")

	seedJob(t, ctx, repos, first.ID, 10, instant)
	otherJob := seedJob(t, ctx, repos, other.ID, 20, instant.Add(time.Hour))

	latest, err := repos.jobs.FindLatestByDocumentID(ctx, first.ID)
	require.NoError(t, err)
	assert.NotEqual(t, otherJob.ID, latest.ID, "another book's job must never be resumed")
	assert.Equal(t, first.ID, latest.DocumentID)
}

func TestPageOutcomeRepository_SaveOutcomeReplacesTheOutcomeForTheSamePage(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	document := seedDocument(t, ctx, repos.documents, "ddd")
	job := seedJob(t, ctx, repos, document.ID, 5, instant)

	failed := outcome(job.ID, 3, domain.PageFailed, "", instant)
	failed.Error = "pdftotext exited 1"
	require.NoError(t, repos.pages.SaveOutcome(ctx, failed))

	recorded, err := repos.pages.ListOutcomes(ctx, job.ID)
	require.NoError(t, err)
	require.Len(t, recorded, 1)
	assert.Equal(t, domain.PageFailed, recorded[0].Status)
	assert.Equal(t, "pdftotext exited 1", recorded[0].Error)

	// The retry, which is what resuming means.
	require.NoError(t, repos.pages.SaveOutcome(ctx,
		outcome(job.ID, 3, domain.PageExtracted, "aggregate root", instant.Add(time.Minute))))

	recorded, err = repos.pages.ListOutcomes(ctx, job.ID)
	require.NoError(t, err)
	require.Len(t, recorded, 1, "a retried page must replace its outcome, not add one")
	assert.Equal(t, domain.PageExtracted, recorded[0].Status)
	assert.Equal(t, "aggregate root", recorded[0].Text)
	assert.Empty(t, recorded[0].Error, "the previous failure reason must not survive a success")
	assert.True(t, recorded[0].UpdatedAt.After(instant))

	var count int
	require.NoError(t, sqlDB.QueryRow(
		`SELECT count(*) FROM job_pages WHERE job_id = ?`, job.ID).Scan(&count))
	assert.Equal(t, 1, count)
}

// A failed page is pending again, so the retry path is the resume path. It must
// not make the job look like it has more pages than it does.
func TestPageOutcomeRepository_ARetriedPageChangesTheDerivedProgress(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	document := seedDocument(t, ctx, repos.documents, "ddd")
	job := seedJob(t, ctx, repos, document.ID, 2, instant)

	failed := outcome(job.ID, 2, domain.PageFailed, "", instant)
	failed.Error = "figure only"
	require.NoError(t, repos.pages.SaveOutcome(ctx, outcome(job.ID, 1, domain.PageExtracted, "first", instant)))
	require.NoError(t, repos.pages.SaveOutcome(ctx, failed))

	recorded, err := repos.pages.ListOutcomes(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{2}, domain.PendingPages(2, recorded))
	assert.Equal(t, 1, domain.ProgressOf(2, recorded).Failed)

	require.NoError(t, repos.pages.SaveOutcome(ctx,
		outcome(job.ID, 2, domain.PageTranscribed, "a diagram of the aggregate", instant.Add(time.Minute))))

	recorded, err = repos.pages.ListOutcomes(ctx, job.ID)
	require.NoError(t, err)
	assert.Empty(t, domain.PendingPages(2, recorded))
	progress := domain.ProgressOf(2, recorded)
	assert.Equal(t, 0, progress.Failed)
	assert.Equal(t, 1, progress.Extracted)
	assert.Equal(t, 1, progress.Transcribed)
	assert.True(t, progress.Settled())
}

func TestPageOutcomeRepository_SaveOutcomeForAnUnknownJobIsNotFound(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	err := repos.pages.SaveOutcome(ctx, outcome(uuid.NewString(), 1, domain.PageExtracted, "text", instant))
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

func TestPageOutcomeRepository_ListOutcomesIsOrderedByPageNumber(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	document := seedDocument(t, ctx, repos.documents, "ddd")
	job := seedJob(t, ctx, repos, document.ID, 5, instant)

	for _, page := range []int{5, 3, 1} {
		require.NoError(t, repos.pages.SaveOutcome(ctx,
			outcome(job.ID, page, domain.PageExtracted, fmt.Sprintf("page %d", page), instant)))
	}

	recorded, err := repos.pages.ListOutcomes(ctx, job.ID)
	require.NoError(t, err)
	require.Len(t, recorded, 3)

	pages := make([]int, 0, len(recorded))
	for _, recordedOutcome := range recorded {
		pages = append(pages, recordedOutcome.PageNumber)
	}
	assert.Equal(t, []int{1, 3, 5}, pages,
		"the materializer reads pages in order, so the read must be ordered")
}

func TestPageOutcomeRepository_ListOutcomesOfAJobWithNoOutcomesIsEmpty(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	document := seedDocument(t, ctx, repos.documents, "ddd")
	job := seedJob(t, ctx, repos, document.ID, 5, instant)

	recorded, err := repos.pages.ListOutcomes(ctx, job.ID)
	require.NoError(t, err)
	assert.NotNil(t, recorded, "an empty result must be an empty slice, not nil")
	assert.Empty(t, recorded)
}

func TestPageOutcomeRepository_TextSurvivesTheRoundTrip(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	document := seedDocument(t, ctx, repos.documents, "ddd")
	job := seedJob(t, ctx, repos, document.ID, 1, instant)

	text := "don't \"quote\" me — ünïcode\nsecond line\n\ttabbed\n\n```go\nfunc f() {}\n```"

	require.NoError(t, repos.pages.SaveOutcome(ctx,
		outcome(job.ID, 1, domain.PageTranscribed, text, instant)))

	recorded, err := repos.pages.ListOutcomes(ctx, job.ID)
	require.NoError(t, err)
	require.Len(t, recorded, 1)
	assert.Equal(t, text, recorded[0].Text)
	assert.Equal(t, domain.MethodVision, recorded[0].Method)
	assert.Equal(t, domain.PageTranscribed, recorded[0].Status)
}

// The acceptance test for resumability at the persistence layer.
//
// A run dies at page 179 of 359 and the next process knows only the content hash
// of the book. Everything else has to come out of the database, and what it must
// produce is: the same job, the pages that are still owed, and exactly one
// recorded outcome per page when it finishes.
func TestIngestion_AnInterruptedRunResumesFromRecordedOutcomes(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	const totalPages = 359

	document := seedDocument(t, ctx, repos.documents, "ddd")
	job := seedJob(t, ctx, repos, document.ID, totalPages, instant)

	require.NoError(t, job.MarkRunning(instant))
	require.NoError(t, repos.jobs.UpdateStatus(ctx, job))

	// First run: pages 1-179, page 90 failed, then the process died.
	for page := 1; page <= 179; page++ {
		at := instant.Add(time.Duration(page) * time.Second)
		if page == 90 {
			failed := outcome(job.ID, page, domain.PageFailed, "", at)
			failed.Error = "the model returned nothing for a figure-only page"
			require.NoError(t, repos.pages.SaveOutcome(ctx, failed))
			continue
		}

		status := domain.PageExtracted
		if page%2 == 1 {
			status = domain.PageTranscribed
		}
		require.NoError(t, repos.pages.SaveOutcome(ctx,
			outcome(job.ID, page, status, fmt.Sprintf("text of page %d", page), at)))
	}

	// Second process. It knows nothing but the bytes it just acquired.
	resumedDocument, err := repos.documents.FindByContentHash(ctx, testHash("ddd"))
	require.NoError(t, err)
	resumedJob, err := repos.jobs.FindLatestByDocumentID(ctx, resumedDocument.ID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, resumedJob.ID, "the resumed process must continue the same job")
	assert.True(t, resumedJob.IsResumable())

	recorded, err := repos.pages.ListOutcomes(ctx, resumedJob.ID)
	require.NoError(t, err)

	pending := domain.PendingPages(resumedJob.TotalPages, recorded)
	progress := domain.ProgressOf(totalPages, recorded)
	// The two views differ by exactly the failed pages, and that difference is the
	// contract: Progress counts a failed page as settled (it has a terminal
	// outcome), while PendingPages counts it as owed (retrying it is what resuming
	// means).
	assert.Equal(t, progress.Pending+progress.Failed, len(pending))

	require.Len(t, pending, 181, "pages 180-359 plus the failed page 90, which is owed again")
	assert.Equal(t, 90, pending[0], "the failed page is owed again")
	assert.Equal(t, 180, pending[1], "completed pages are not redone")
	assert.Equal(t, totalPages, pending[len(pending)-1])
	assert.NotContains(t, pending, 89, "a transcribed page is finished")

	// Pages 1-179 with page 90 failed: 90 odd pages went to the model and 88 even
	// pages did not (page 90 is even and failed).
	assert.Equal(t,
		domain.Progress{Total: totalPages, Extracted: 88, Transcribed: 90, Failed: 1, Pending: 180},
		progress)

	// The second run finishes the book.
	for _, page := range pending {
		require.NoError(t, repos.pages.SaveOutcome(ctx,
			outcome(job.ID, page, domain.PageExtracted, fmt.Sprintf("text of page %d", page), instant.Add(time.Hour))))
	}

	recorded, err = repos.pages.ListOutcomes(ctx, resumedJob.ID)
	require.NoError(t, err)
	assert.Empty(t, domain.PendingPages(totalPages, recorded))

	progress = domain.ProgressOf(totalPages, recorded)
	assert.Equal(t, 0, progress.Pending)
	assert.Equal(t, 0, progress.Failed, "a retried page replaces its failure")
	assert.Equal(t, totalPages, progress.Succeeded())
	assert.Equal(t, 359, len(recorded))

	require.NoError(t, resumedJob.Settle(progress, instant.Add(2*time.Hour)))
	require.NoError(t, repos.jobs.UpdateStatus(ctx, resumedJob))

	settled, err := repos.jobs.FindByID(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.JobCompleted, settled.Status)
	require.NotNil(t, settled.FinishedAt)
	assert.False(t, settled.IsResumable())

	// Exactly one outcome per page. This is what keeps a resumed run from
	// duplicating a page's text in the output it materializes.
	var rows, distinctPages int
	require.NoError(t, sqlDB.QueryRow(
		`SELECT count(*), count(DISTINCT page_number) FROM job_pages WHERE job_id = ?`, job.ID,
	).Scan(&rows, &distinctPages))
	assert.Equal(t, totalPages, rows)
	assert.Equal(t, totalPages, distinctPages)

	texts := make(map[int]string, len(recorded))
	for _, recordedOutcome := range recorded {
		texts[recordedOutcome.PageNumber] = recordedOutcome.Text
	}
	assert.Len(t, texts, totalPages, "every page must carry text for the materializer")
	assert.Equal(t, "text of page 90", texts[90], "the retried page carries the new text")
	assert.Equal(t, "text of page 359", texts[359])
}
