package sqlite_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// S1 and S3: one range is one row, and re-marking it is a state change rather
// than a second row. created_at belongs to the first marking; updated_at to the
// last one.
func TestReadingProgressRepository_UpsertKeepsCreatedAtAndBumpsUpdatedAt(t *testing.T) {
	sqlDB := openMigratedDB(t)
	repo := sqlite.NewReadingProgressRepository(sqlDB)
	ctx := t.Context()

	created := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	first, err := repo.Upsert(ctx, &domain.ReadingProgress{
		ID: "rp1", DocumentID: "doc-1", FromPage: 24, ToPage: 36,
		State: domain.ReadingStateRead, CreatedAt: created, UpdatedAt: created,
	})
	require.NoError(t, err)
	assert.Equal(t, created, first.CreatedAt)
	assert.Equal(t, domain.ReadingStateRead, first.State)

	later := created.Add(2 * time.Hour)
	second, err := repo.Upsert(ctx, &domain.ReadingProgress{
		ID: "rp2", DocumentID: "doc-1", FromPage: 24, ToPage: 36,
		State: domain.ReadingStateUnread, CreatedAt: later, UpdatedAt: later,
	})
	require.NoError(t, err)

	assert.Equal(t, created, second.CreatedAt, "the first marking of a range owns created_at")
	assert.Equal(t, later, second.UpdatedAt)
	assert.Equal(t, domain.ReadingStateUnread, second.State)

	ranges, err := repo.ListByDocument(ctx, "doc-1")
	require.NoError(t, err)
	require.Len(t, ranges, 1, "one range is one row, however many times it is marked")
	assert.Equal(t, "rp1", ranges[0].ID, "the row keeps the identifier it was created with")
	assert.Equal(t, domain.ReadingStateUnread, ranges[0].State)
}

// S1: unread is a stored state, not the absence of a row. A range explicitly
// marked unread keeps its row so the reader can tell "not read" from "never
// looked at".
func TestReadingProgressRepository_StoresUnreadAsAStateOfItsOwn(t *testing.T) {
	sqlDB := openMigratedDB(t)
	repo := sqlite.NewReadingProgressRepository(sqlDB)
	ctx := t.Context()

	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	_, err := repo.Upsert(ctx, &domain.ReadingProgress{
		ID: "rp1", DocumentID: "doc-1", FromPage: 24, ToPage: 36,
		State: domain.ReadingStateUnread, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)

	ranges, err := repo.ListByDocument(ctx, "doc-1")
	require.NoError(t, err)
	require.Len(t, ranges, 1)
	assert.Equal(t, domain.ReadingStateUnread, ranges[0].State)
}

// S5: the list is ordered by page, so a client renders ranges in the order the
// book prints them rather than in the order they were marked.
func TestReadingProgressRepository_ListsADocumentsRangesInPageOrder(t *testing.T) {
	sqlDB := openMigratedDB(t)
	repo := sqlite.NewReadingProgressRepository(sqlDB)
	ctx := t.Context()

	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	for i, page := range []int{100, 24, 61} {
		_, err := repo.Upsert(ctx, &domain.ReadingProgress{
			ID: "rp" + string(rune('a'+i)), DocumentID: "doc-1",
			FromPage: page, ToPage: page + 5,
			State: domain.ReadingStateRead, CreatedAt: now, UpdatedAt: now,
		})
		require.NoError(t, err)
	}

	_, err := repo.Upsert(ctx, &domain.ReadingProgress{
		ID: "rp-other", DocumentID: "doc-2", FromPage: 1, ToPage: 2,
		State: domain.ReadingStateRead, CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)

	ranges, err := repo.ListByDocument(ctx, "doc-1")
	require.NoError(t, err)

	pages := make([]int, 0, len(ranges))
	for _, reading := range ranges {
		pages = append(pages, reading.FromPage)
	}
	assert.Equal(t, []int{24, 61, 100}, pages, "page order, and only this document")
}

// S5: a document with nothing marked is an empty slice, not nil, so the
// transport can encode [].
func TestReadingProgressRepository_ADocumentWithNothingMarkedIsEmpty(t *testing.T) {
	sqlDB := openMigratedDB(t)
	repo := sqlite.NewReadingProgressRepository(sqlDB)

	ranges, err := repo.ListByDocument(t.Context(), "doc-nobody-touched")
	require.NoError(t, err)
	assert.NotNil(t, ranges)
	assert.Empty(t, ranges)
}
