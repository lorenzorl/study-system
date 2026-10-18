package sqlite_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// chunksRows counts the source-of-truth chunk rows directly, so an assertion
// about "the same rows" is about stored rows and not about what a query chose to
// return.
func chunksRows(t *testing.T, sqlDB *sql.DB) int {
	t.Helper()

	var count int
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM chunks`).Scan(&count))
	return count
}

// ftsRowsMatching counts the FTS rows still holding a term. Reading the index
// table itself is the only way to see a copy left behind by a delete: the join
// in Search would hide it.
func ftsRowsMatching(t *testing.T, sqlDB *sql.DB, term string) int {
	t.Helper()

	var count int
	require.NoError(t, sqlDB.QueryRow(
		`SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH ?`, `"`+term+`"`,
	).Scan(&count))
	return count
}

// indexedDocument seeds a document with a job and replaces its chunks with the
// given per-page texts, so a search test works against rows written the way the
// adapter writes them.
func indexedDocument(
	t *testing.T,
	ctx context.Context,
	repos ragRepos,
	title string,
	pages map[int][]string,
) *domain.Document {
	t.Helper()

	document := &domain.Document{
		ID:            uuid.NewString(),
		Title:         title,
		ArtifactID:    uuid.NewString(),
		SourceURI:     "/books/" + title,
		ContentSHA256: testHash(title),
		CreatedAt:     instant,
	}
	require.NoError(t, repos.documents.Create(ctx, document))

	totalPages := 0
	for page := range pages {
		if page > totalPages {
			totalPages = page
		}
	}
	job := seedJob(t, ctx, repos, document.ID, totalPages, instant)

	chunks := make([]domain.Chunk, 0, len(pages))
	for page := 1; page <= totalPages; page++ {
		// The page outcome is recorded as well, because a chunk's text always
		// comes from a recorded page and a reindex reads it back from there.
		require.NoError(t, repos.pages.SaveOutcome(ctx,
			outcome(job.ID, page, domain.PageExtracted, strings.Join(pages[page], "\n\n"), instant)))

		for ordinal, text := range pages[page] {
			chunks = append(chunks, domain.Chunk{PageNumber: page, Ordinal: ordinal, Text: text})
		}
	}
	require.NoError(t, repos.chunks.ReplaceDocumentChunks(ctx, document.ID, job.ID, chunks, instant))

	return document
}

func TestChunkRepository_ReplaceLeavesTheSameRowsWhenRunTwice(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	document := indexedDocument(t, ctx, repos, "un libro", map[int][]string{
		1: {"el agregado es la raíz"},
		2: {"el repositorio guarda objetos"},
	})

	before := chunksRows(t, sqlDB)

	// The same document is written again, which is what -reindex and a settled
	// re-run both do.
	require.NoError(t, replaceWithCurrentChunks(t, ctx, repos, document.ID))

	assert.Equal(t, before, chunksRows(t, sqlDB),
		"a replace must leave the same number of chunk rows, not append a second set")

	results, err := repos.chunks.Search(ctx, "agregado", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1, "the second replace must not duplicate the result either")
}

func TestChunkRepository_ReplaceReflectsChangedText(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	document := indexedDocument(t, ctx, repos, "un libro", map[int][]string{
		1: {"el agregado es la raíz"},
	})

	job, err := repos.jobs.FindLatestByDocumentID(ctx, document.ID)
	require.NoError(t, err)
	require.NoError(t, repos.chunks.ReplaceDocumentChunks(ctx, document.ID, job.ID, []domain.Chunk{
		{PageNumber: 1, Ordinal: 0, Text: "el repositorio guarda objetos"},
	}, instant))

	removed, err := repos.chunks.Search(ctx, "agregado", 10)
	require.NoError(t, err)
	assert.Empty(t, removed, "the replaced text must stop matching")

	found, err := repos.chunks.Search(ctx, "repositorio", 10)
	require.NoError(t, err)
	assert.Len(t, found, 1)

	// The FTS table is a copy, so the proof that it was cleaned too is that the
	// term only the removed chunk held matches nothing while its row count is
	// back to one.
	assert.Equal(t, 1, chunksRows(t, sqlDB))
	assert.Equal(t, 0, ftsRowsMatching(t, sqlDB, "agregado"))
}

func TestChunkRepository_ReplaceWithNothingRemovesTheIndexRowsToo(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	document := indexedDocument(t, ctx, repos, "un libro", map[int][]string{
		1: {"un término exclusivo del libro"},
	})

	job, err := repos.jobs.FindLatestByDocumentID(ctx, document.ID)
	require.NoError(t, err)
	require.NoError(t, repos.chunks.ReplaceDocumentChunks(ctx, document.ID, job.ID, nil, instant))

	assert.Equal(t, 0, chunksRows(t, sqlDB), "the chunk rows must be gone")
	assert.Equal(t, 0, ftsRowsMatching(t, sqlDB, "exclusivo"),
		"a copy left behind in the FTS table would still hold the removed chunk's term")

	results, err := repos.chunks.Search(ctx, "exclusivo", 10)
	require.NoError(t, err)
	assert.Empty(t, results, "a term that only existed in the removed rows must not match")
}

func TestChunkRepository_SearchRanksTheBestMatchFirst(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	indexedDocument(t, ctx, repos, "both", map[int][]string{1: {"aggregate root"}})
	indexedDocument(t, ctx, repos, "one", map[int][]string{1: {"aggregate only"}})
	indexedDocument(t, ctx, repos, "other", map[int][]string{1: {"unrelated prose"}})

	results, err := repos.chunks.Search(ctx, "aggregate root", 10)
	require.NoError(t, err)
	require.Len(t, results, 2, "a chunk that matches only one term is still returned")

	assert.Equal(t, "both", results[0].DocumentTitle,
		"the chunk matching both terms must rank first")
	for i := 0; i+1 < len(results); i++ {
		assert.LessOrEqual(t, results[i].Score, results[i+1].Score,
			"bm25 is smaller-is-better, so the order must be ascending")
	}
	assert.Negative(t, results[0].Score, "bm25 returns negative scores")
}

func TestChunkRepository_SearchRespectsTheLimit(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	indexedDocument(t, ctx, repos, "a", map[int][]string{1: {"aggregate root repositories"}})
	indexedDocument(t, ctx, repos, "b", map[int][]string{1: {"aggregate boundaries"}})
	indexedDocument(t, ctx, repos, "c", map[int][]string{1: {"aggregate invariants"}})

	all, err := repos.chunks.Search(ctx, "aggregate", 10)
	require.NoError(t, err)
	require.Len(t, all, 3)

	limited, err := repos.chunks.Search(ctx, "aggregate", 2)
	require.NoError(t, err)
	assert.Len(t, limited, 2)
	assert.Equal(t, all[:2], limited, "the limit must keep the best-ranked results")
}

func TestChunkRepository_SearchReturnsTheCitation(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	indexedDocument(t, ctx, repos, "Domain-Driven Design", map[int][]string{
		3: {"a chapter about nothing in particular"},
		4: {"the aggregate is the consistency boundary"},
	})

	results, err := repos.chunks.Search(ctx, "aggregate", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)

	result := results[0]
	assert.Equal(t, "Domain-Driven Design", result.DocumentTitle,
		"a result must name the book a reader should open")
	assert.Equal(t, 4, result.PageNumber, "a result must name the exact page")
	assert.NotEmpty(t, result.ChunkID)
	assert.Contains(t, result.Snippet, "[aggregate]",
		"the matched term must be visible inside the snippet")
	assert.NotContains(t, result.Snippet, "\n", "a snippet is one line")
}

// A snippet is a citation aid: 12 tokens with the matched terms marked. A caller
// that feeds the retrieved text to a model needs the chunk itself, so a result
// carries both and they are not the same string.
func TestChunkRepository_SearchReturnsTheFullChunkText(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	const chunk = "the aggregate is the consistency boundary of the model, and everything inside it must stay consistent in one transaction"

	indexedDocument(t, ctx, repos, "Domain-Driven Design", map[int][]string{4: {chunk}})

	results, err := repos.chunks.Search(ctx, "aggregate", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, chunk, results[0].Text, "a result must carry the whole chunk, verbatim")
	assert.NotEqual(t, results[0].Text, results[0].Snippet,
		"the snippet is a bounded window and the text is not")
}

func TestChunkRepository_SearchFoldsAccentsInBothDirections(t *testing.T) {
	t.Run("an unaccented Spanish query finds accented text", func(t *testing.T) {
		_, repos := openRepos(t)
		ctx := t.Context()

		indexedDocument(t, ctx, repos, "diseño", map[int][]string{
			2: {"el obstáculo del diseño no detiene el árbol de decisión"},
		})

		results, err := repos.chunks.Search(ctx, "obstaculo diseno arbol", 10)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 2, results[0].PageNumber)
	})

	t.Run("an accented Spanish query finds unaccented text", func(t *testing.T) {
		_, repos := openRepos(t)
		ctx := t.Context()

		indexedDocument(t, ctx, repos, "sin acentos", map[int][]string{
			5: {"el obstaculo y el diseno"},
		})

		results, err := repos.chunks.Search(ctx, "obstáculo diseño", 10)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, 5, results[0].PageNumber)
	})
}

func TestChunkRepository_PunctuationOnlyQueryIsAnEmptyResultAndNotAnError(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	indexedDocument(t, ctx, repos, "a", map[int][]string{1: {"aggregate root"}})

	for name, query := range map[string]string{
		"punctuation": "...",
		"symbols":     "??? !!! —",
		"quotes only": `"''"`,
		"empty":       "",
		"spaces":      "   ",
	} {
		t.Run(name, func(t *testing.T) {
			results, err := repos.chunks.Search(ctx, query, 10)
			require.NoError(t, err, "a query with no term must not be an error")
			assert.NotNil(t, results, "no matches must be an empty slice, not nil")
			assert.Empty(t, results)
		})
	}
}

func TestChunkRepository_AQueryContainingADoubleQuoteDoesNotError(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	indexedDocument(t, ctx, repos, "a", map[int][]string{1: {"hot jupiter in a thick disk"}})

	for name, query := range map[string]string{
		"embedded quote": `thick "disk`,
		"quoted phrase":  `"hot jupiter"`,
		"unbalanced":     `"disk`,
		"trailing quote": `disk"`,
		"doubled quotes": `""`,
		"star and caret": `disk* ^thick`,
		"operator words": `disk AND NOT thick`,
	} {
		t.Run(name, func(t *testing.T) {
			results, err := repos.chunks.Search(ctx, query, 10)
			require.NoError(t, err, "the user's text must never reach MATCH raw")
			assert.NotNil(t, results)
		})
	}

	// A quoted phrase is a literal phrase, not a syntax error, and still matches.
	results, err := repos.chunks.Search(ctx, `"hot jupiter"`, 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestChunkRepository_SearchWithNoMatchesIsAnEmptySlice(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	indexedDocument(t, ctx, repos, "a", map[int][]string{1: {"aggregate root"}})

	results, err := repos.chunks.Search(ctx, "término inexistente", 10)
	require.NoError(t, err)
	assert.NotNil(t, results, "a caller ranges over the result, so it must not be nil")
	assert.Empty(t, results)
}

// replaceWithCurrentChunks re-reads the document's page text and indexes it
// again, which is the path a reindex takes.
func replaceWithCurrentChunks(
	t *testing.T,
	ctx context.Context,
	repos ragRepos,
	documentID string,
) error {
	t.Helper()

	job, err := repos.jobs.FindLatestByDocumentID(ctx, documentID)
	require.NoError(t, err)

	outcomes, err := repos.pages.ListOutcomes(ctx, job.ID)
	require.NoError(t, err)

	chunks := make([]domain.Chunk, 0, len(outcomes))
	for _, outcome := range outcomes {
		chunks = append(chunks, domain.ChunkPage(outcome.Text, outcome.PageNumber, domain.ChunkOptions{})...)
	}

	return repos.chunks.ReplaceDocumentChunks(ctx, documentID, job.ID, chunks, instant)
}
