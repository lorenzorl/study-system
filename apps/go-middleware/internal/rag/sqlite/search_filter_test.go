package sqlite_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
)

func TestChunkRepository_SearchInDocumentFiltersToThatDocument(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	target := indexedDocument(t, ctx, repos, "target", map[int][]string{1: {"aggregate root"}})
	indexedDocument(t, ctx, repos, "other", map[int][]string{1: {"aggregate boundary"}})

	chunks := ragsqlite.NewChunkRepository(sqlDB)

	whole, err := chunks.Search(ctx, "aggregate", 10)
	require.NoError(t, err)
	require.Len(t, whole, 2)

	scoped, err := chunks.SearchInDocument(ctx, "aggregate", target.ID, 10)
	require.NoError(t, err)
	require.Len(t, scoped, 1)
	assert.Equal(t, target.ID, scoped[0].DocumentID)
	assert.Equal(t, "target", scoped[0].DocumentTitle)

	none, err := chunks.SearchInDocument(ctx, "aggregate", uuid.NewString(), 10)
	require.NoError(t, err)
	assert.NotNil(t, none)
	assert.Empty(t, none, "a document that matches nothing is an empty slice, not an error")
}

func TestChunkRepository_SearchInDocumentLimitBoundsTheFilteredSet(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	target := indexedDocument(t, ctx, repos, "target", map[int][]string{1: {"aggregate"}})
	indexedDocument(t, ctx, repos, "noise", map[int][]string{
		1: {"aggregate aggregate aggregate aggregate"},
		2: {"aggregate aggregate aggregate aggregate"},
		3: {"aggregate aggregate aggregate aggregate"},
	})

	chunks := ragsqlite.NewChunkRepository(sqlDB)
	scoped, err := chunks.SearchInDocument(ctx, "aggregate", target.ID, 1)
	require.NoError(t, err)
	require.Len(t, scoped, 1, "the LIMIT must apply after the document filter")
	assert.Equal(t, target.ID, scoped[0].DocumentID)
}

// PRESERVE: an empty document_id is the whole-corpus query, not an error and not
// an empty result.
func TestChunkRepository_SearchInDocumentEmptyDocumentIDIsTheWholeCorpus(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	indexedDocument(t, ctx, repos, "a", map[int][]string{1: {"aggregate root"}})

	chunks := ragsqlite.NewChunkRepository(sqlDB)
	scoped, err := chunks.SearchInDocument(ctx, "aggregate", "", 10)
	require.NoError(t, err)
	assert.Len(t, scoped, 1)
}
