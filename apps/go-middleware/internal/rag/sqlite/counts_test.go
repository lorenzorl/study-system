package sqlite_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
)

// T1 — the per-document and per-job aggregate counts the status endpoint reads.
// They answer "how many" without loading any text, which is the whole point:
// the status must not read a 359-page book to count its pages or its chunks.

func TestChunkRepository_CountByDocument(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	document := indexedDocument(t, ctx, repos, "un libro", map[int][]string{
		1: {"primera", "segunda"},
		2: {"tercera"},
	})

	count, err := repos.chunks.CountByDocument(ctx, document.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	unknown, err := repos.chunks.CountByDocument(ctx, uuid.NewString())
	require.NoError(t, err)
	assert.Equal(t, 0, unknown, "a document with no chunk counts zero, not an error")
}

func TestEmbeddingRepository_CountByDocument(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()
	embeddings := ragsqlite.NewEmbeddingRepository(sqlDB)

	document := indexedDocument(t, ctx, repos, "un libro", map[int][]string{
		1: {"primera", "segunda"},
		2: {"tercera"},
	})

	require.NoError(t, embeddings.ReplaceDocumentEmbeddings(ctx, document.ID, "model-a",
		embeddingsFor(t, ctx, repos, document.ID, []float32{1, 0, 0}), instant))

	count, err := embeddings.CountByDocument(ctx, document.ID, "model-a")
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	other, err := embeddings.CountByDocument(ctx, document.ID, "model-b")
	require.NoError(t, err)
	assert.Equal(t, 0, other, "a vector from another model is not part of this model's set")

	unknown, err := embeddings.CountByDocument(ctx, uuid.NewString(), "model-a")
	require.NoError(t, err)
	assert.Equal(t, 0, unknown)
}

func TestPageOutcomeRepository_CountByJob(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	document := seedDocument(t, ctx, repos.documents, "ddd")
	job := seedJob(t, ctx, repos, document.ID, 5, instant)

	require.NoError(t, repos.pages.SaveOutcome(ctx, outcome(job.ID, 1, domain.PageExtracted, "uno", instant)))
	require.NoError(t, repos.pages.SaveOutcome(ctx, outcome(job.ID, 2, domain.PageExtracted, "dos", instant)))
	require.NoError(t, repos.pages.SaveOutcome(ctx, outcome(job.ID, 3, domain.PageTranscribed, "tres", instant)))
	require.NoError(t, repos.pages.SaveOutcome(ctx, outcome(job.ID, 4, domain.PageFailed, "", instant)))

	counts, err := repos.pages.CountByJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, counts.Extracted)
	assert.Equal(t, 1, counts.Transcribed)
	assert.Equal(t, 1, counts.Failed)

	// A retried page replaces its row, so it is counted once, exactly as
	// ProgressOf deduplicates it.
	require.NoError(t, repos.pages.SaveOutcome(ctx, outcome(job.ID, 4, domain.PageExtracted, "cuatro", instant)))
	counts, err = repos.pages.CountByJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, counts.Extracted)
	assert.Equal(t, 1, counts.Transcribed)
	assert.Equal(t, 0, counts.Failed)

	empty, err := repos.pages.CountByJob(ctx, uuid.NewString())
	require.NoError(t, err)
	assert.Equal(t, domain.PageCounts{}, empty)
}
