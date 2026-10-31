package sqlite_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
)

// pageNumbersOf is the recorded page numbers in the order the query returned
// them, which is what proves the range was ordered by the index rather than
// sorted afterwards.
func pageNumbersOf(outcomes []domain.PageOutcome) []int {
	numbers := make([]int, 0, len(outcomes))
	for _, outcome := range outcomes {
		numbers = append(numbers, outcome.PageNumber)
	}
	return numbers
}

// T1 — the range read behind GET /api/rag/documents/{id}/pages. It returns the
// recorded pages inside the range and nothing else: loading a 359-page book to
// answer a twelve-page request is exactly what this query exists to avoid.
func TestPageOutcomeRepository_ListPageTextsInRange(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()
	pageTexts := ragsqlite.NewPageOutcomeRepository(sqlDB)

	document := seedDocument(t, ctx, repos.documents, "ddd")
	job := seedJob(t, ctx, repos, document.ID, 10, instant)

	// Recorded out of order, and with a gap: page 3 produced no outcome at all,
	// which is not the same as a page that failed.
	for _, page := range []int{5, 1, 2, 4} {
		require.NoError(t, repos.pages.SaveOutcome(ctx,
			outcome(job.ID, page, domain.PageExtracted, fmt.Sprintf("texto %d", page), instant)))
	}

	inRange, err := pageTexts.ListPageTextsInRange(ctx, job.ID, 2, 4)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 4}, pageNumbersOf(inRange), "pages 2 and 4 only, by page number ascending")
	require.Len(t, inRange, 2)
	assert.Equal(t, "texto 2", inRange[0].Text)
	assert.Equal(t, domain.PageExtracted, inRange[0].Status)

	whole, err := pageTexts.ListPageTextsInRange(ctx, job.ID, 1, 5)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 4, 5}, pageNumbersOf(whole))

	// The bounds are inclusive on both ends, so a single-page range is the page
	// itself and one page outside it is empty.
	single, err := pageTexts.ListPageTextsInRange(ctx, job.ID, 5, 5)
	require.NoError(t, err)
	assert.Equal(t, []int{5}, pageNumbersOf(single))

	none, err := pageTexts.ListPageTextsInRange(ctx, job.ID, 6, 10)
	require.NoError(t, err)
	assert.NotNil(t, none, "a range with no recorded page is an empty slice, never nil")
	assert.Empty(t, none)
}

// Another job's pages are not this job's, even when the page numbers coincide:
// the range is scoped to the job the document's latest run produced.
func TestPageOutcomeRepository_ListPageTextsInRangeIsScopedToOneJob(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()
	pageTexts := ragsqlite.NewPageOutcomeRepository(sqlDB)

	first := seedDocument(t, ctx, repos.documents, "ddd")
	firstJob := seedJob(t, ctx, repos, first.ID, 3, instant)
	require.NoError(t, repos.pages.SaveOutcome(ctx,
		outcome(firstJob.ID, 1, domain.PageExtracted, "primera", instant)))

	second := seedDocument(t, ctx, repos.documents, "otro")
	secondJob := seedJob(t, ctx, repos, second.ID, 3, instant)
	require.NoError(t, repos.pages.SaveOutcome(ctx,
		outcome(secondJob.ID, 1, domain.PageExtracted, "otra", instant)))

	mine, err := pageTexts.ListPageTextsInRange(ctx, secondJob.ID, 1, 3)
	require.NoError(t, err)
	require.Len(t, mine, 1)
	assert.Equal(t, "otra", mine[0].Text)

	unknown, err := pageTexts.ListPageTextsInRange(ctx, uuid.NewString(), 1, 3)
	require.NoError(t, err)
	assert.Empty(t, unknown)
}
