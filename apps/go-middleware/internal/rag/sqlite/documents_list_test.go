package sqlite_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
)

// T7: the corpus list reads every document, including one nobody has converted
// yet, because a caller choosing an attachment must be able to see it.
func TestDocumentRepository_ListAllReturnsEveryDocument(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	withPages := seedDocument(t, ctx, repos.documents, "with-pages")
	seedJob(t, ctx, repos, withPages.ID, 3, instant)
	seedDocument(t, ctx, repos.documents, "without-pages")

	documents := ragsqlite.NewDocumentRepository(sqlDB)
	all, err := documents.ListAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2, "a document nobody has converted is still part of the corpus")
}

func TestDocumentRepository_ListAllOnAnEmptyCorpusIsAnEmptySlice(t *testing.T) {
	sqlDB, _ := openRepos(t)

	all, err := ragsqlite.NewDocumentRepository(sqlDB).ListAll(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, all, "an empty corpus is a slice, never nil")
	assert.Empty(t, all)
}

func TestPageOutcomeRepository_CountByDocument(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	document := seedDocument(t, ctx, repos.documents, "ddd")
	job := seedJob(t, ctx, repos, document.ID, 5, instant)
	require.NoError(t, repos.pages.SaveOutcome(ctx, outcome(job.ID, 1, domain.PageExtracted, "uno", instant)))
	require.NoError(t, repos.pages.SaveOutcome(ctx, outcome(job.ID, 2, domain.PageExtracted, "dos", instant)))
	require.NoError(t, repos.pages.SaveOutcome(ctx, outcome(job.ID, 3, domain.PageFailed, "", instant)))

	pages := ragsqlite.NewPageOutcomeRepository(sqlDB)
	count, err := pages.CountByDocument(ctx, document.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	unknown, err := pages.CountByDocument(ctx, "no-such-document")
	require.NoError(t, err)
	assert.Equal(t, 0, unknown, "an unknown document counts zero, not an error")
}
