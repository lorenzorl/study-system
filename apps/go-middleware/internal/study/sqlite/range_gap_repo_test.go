package sqlite_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// S2 and S7: a gap is addressable on its own, and changing its state keeps the
// point, the page and the quote — the evidence is not a state.
func TestRangeGapRepository_UpdateStateKeepsTheEvidence(t *testing.T) {
	ctx := t.Context()
	sqlDB := openMigratedDB(t)
	explanation := newExplanation("an explanation of the aggregate boundary")
	require.NoError(t, sqlite.NewRangeExplanationRepository(sqlDB).Create(ctx, explanation))

	repo := sqlite.NewRangeGapRepository(sqlDB)
	target := explanation.Gaps[1]

	updated, err := repo.UpdateState(ctx, target.ID, domain.RangeGapStateDiscarded)
	require.NoError(t, err)

	assert.Equal(t, target.ID, updated.ID)
	assert.Equal(t, domain.RangeGapStateDiscarded, updated.State)
	assert.Equal(t, target.Point, updated.Point)
	assert.Equal(t, target.Page, updated.Page)
	assert.Equal(t, target.Quote, updated.Quote)

	// Only the gap that was addressed moved.
	gaps, err := repo.ListByExplanationID(ctx, explanation.ID)
	require.NoError(t, err)
	require.Len(t, gaps, 2)
	assert.Equal(t, domain.RangeGapStateOpen, gaps[0].State)
	assert.Equal(t, domain.RangeGapStateDiscarded, gaps[1].State)
}

// S7: undoing a discard returns the gap to open, so discarding is never a one-way
// deletion.
func TestRangeGapRepository_UpdateStateCanReturnAGapToOpen(t *testing.T) {
	ctx := t.Context()
	sqlDB := openMigratedDB(t)
	explanation := newExplanation("an explanation of the aggregate boundary")
	require.NoError(t, sqlite.NewRangeExplanationRepository(sqlDB).Create(ctx, explanation))

	repo := sqlite.NewRangeGapRepository(sqlDB)
	gapID := explanation.Gaps[0].ID

	_, err := repo.UpdateState(ctx, gapID, domain.RangeGapStateDiscarded)
	require.NoError(t, err)

	updated, err := repo.UpdateState(ctx, gapID, domain.RangeGapStateOpen)
	require.NoError(t, err)
	assert.Equal(t, domain.RangeGapStateOpen, updated.State)
}

func TestRangeGapRepository_UpdateStateOfAnUnknownGapIsNotFound(t *testing.T) {
	sqlDB := openMigratedDB(t)
	repo := sqlite.NewRangeGapRepository(sqlDB)

	_, err := repo.UpdateState(t.Context(), "no-such-gap", domain.RangeGapStateDiscarded)

	assert.ErrorIs(t, err, apperror.ErrNotFound)
}
