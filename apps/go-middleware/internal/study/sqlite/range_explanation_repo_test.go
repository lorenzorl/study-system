package sqlite_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// newExplanation is one explanation of a range, with two gaps, ready to store.
func newExplanation(text string) *domain.RangeExplanation {
	return &domain.RangeExplanation{
		ID:         uuid.New().String(),
		DocumentID: "c6d1e9a3-0000-4000-8000-000000000001",
		FromPage:   88,
		ToPage:     114,
		Text:       text,
		CreatedAt:  domain.Now(),
		Gaps: []domain.RangeGap{
			{
				ID:    uuid.New().String(),
				Point: "the boundary is defined by the invariants that must hold in one transaction",
				Page:  89,
				Quote: "Cluster the ENTITIES and VALUE OBJECTS into AGGREGATES",
				State: domain.RangeGapStateOpen,
			},
			{
				ID:    uuid.New().String(),
				Point: "a repository is per aggregate root",
				Page:  108,
				Quote: "only AGGREGATE roots may be obtained from the database directly",
				State: domain.RangeGapStateOpen,
			},
		},
	}
}

func TestRangeExplanationRepository_CreateStoresTheExplanationAndItsGaps(t *testing.T) {
	ctx := t.Context()
	sqlDB := openMigratedDB(t)
	t.Cleanup(func() { sqlDB.Close() })

	repo := sqlite.NewRangeExplanationRepository(sqlDB)
	gapRepo := sqlite.NewRangeGapRepository(sqlDB)

	// The text is stored verbatim, newlines included: it is the reader's own
	// words, not a normalised field.
	explanation := newExplanation("An aggregate is a consistency boundary.\nIt is not a bag of objects.")
	require.NoError(t, repo.Create(ctx, explanation))

	stored, err := repo.FindByID(ctx, explanation.ID)
	require.NoError(t, err)

	assert.Equal(t, explanation.ID, stored.ID)
	assert.Equal(t, explanation.DocumentID, stored.DocumentID)
	assert.Equal(t, 88, stored.FromPage)
	assert.Equal(t, 114, stored.ToPage)
	assert.Equal(t, explanation.Text, stored.Text, "the explanation must survive verbatim")
	assert.Equal(t, explanation.CreatedAt.UTC(), stored.CreatedAt.UTC())
	require.Len(t, stored.Gaps, 2, "both gaps must be stored with the explanation")

	// Storage order is the producer's ranking, which is what a capped list keeps.
	assert.Equal(t, "the boundary is defined by the invariants that must hold in one transaction", stored.Gaps[0].Point)
	assert.Equal(t, 89, stored.Gaps[0].Page)
	assert.Equal(t, "Cluster the ENTITIES and VALUE OBJECTS into AGGREGATES", stored.Gaps[0].Quote)
	assert.Equal(t, domain.RangeGapStateOpen, stored.Gaps[0].State)
	assert.Equal(t, explanation.ID, stored.Gaps[0].ExplanationID, "a gap must belong to its explanation")
	assert.Equal(t, 108, stored.Gaps[1].Page)

	gaps, err := gapRepo.ListByExplanationID(ctx, explanation.ID)
	require.NoError(t, err)
	assert.Len(t, gaps, 2)
	assert.Equal(t, stored.Gaps[0].ID, gaps[0].ID)
}

// The explanation and its gaps are one write: a gap that cannot be stored must
// leave no explanation behind either. Nothing stored on a failure is the same
// invariant the use case relies on when a refusal or a model failure happens, and
// this is where the repository proves it holds for the write itself.
func TestRangeExplanationRepository_CreateIsAtomic(t *testing.T) {
	ctx := t.Context()
	sqlDB := openMigratedDB(t)
	t.Cleanup(func() { sqlDB.Close() })

	repo := sqlite.NewRangeExplanationRepository(sqlDB)

	explanation := newExplanation("An aggregate is a consistency boundary.")
	explanation.Gaps[1].ID = explanation.Gaps[0].ID // a duplicate primary key

	require.Error(t, repo.Create(ctx, explanation))

	var explanations, gaps int
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM range_explanations`).Scan(&explanations))
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM range_gaps`).Scan(&gaps))
	assert.Zero(t, explanations, "a failed write must not leave an explanation behind")
	assert.Zero(t, gaps, "a failed write must not leave a gap behind")
}

// A complete explanation leaves nothing out, so it stores no gaps — and reads
// back as an empty list rather than as nothing at all.
func TestRangeExplanationRepository_StoresAnExplanationThatLeavesNothingOut(t *testing.T) {
	ctx := t.Context()
	sqlDB := openMigratedDB(t)
	t.Cleanup(func() { sqlDB.Close() })

	repo := sqlite.NewRangeExplanationRepository(sqlDB)

	explanation := newExplanation("Everything the range states is in this sentence.")
	explanation.Gaps = nil
	require.NoError(t, repo.Create(ctx, explanation))

	stored, err := repo.FindByID(ctx, explanation.ID)
	require.NoError(t, err)
	assert.NotNil(t, stored.Gaps, "no gaps must be an empty slice, never nil")
	assert.Empty(t, stored.Gaps)

	var gaps int
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM range_gaps`).Scan(&gaps))
	assert.Zero(t, gaps)
}

func TestRangeExplanationRepository_FindByIDOnAMissIsNotFound(t *testing.T) {
	ctx := t.Context()
	sqlDB := openMigratedDB(t)
	t.Cleanup(func() { sqlDB.Close() })

	_, err := sqlite.NewRangeExplanationRepository(sqlDB).FindByID(ctx, uuid.New().String())
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

// The stored instant keeps the precision every other study table uses, so a
// round trip returns the value that was written.
func TestRangeExplanationRepository_RoundTripsTheInstant(t *testing.T) {
	ctx := t.Context()
	sqlDB := openMigratedDB(t)
	t.Cleanup(func() { sqlDB.Close() })

	repo := sqlite.NewRangeExplanationRepository(sqlDB)

	explanation := newExplanation("An aggregate is a consistency boundary.")
	explanation.CreatedAt = time.Date(2026, 11, 2, 10, 30, 0, 0, time.UTC)
	require.NoError(t, repo.Create(ctx, explanation))

	stored, err := repo.FindByID(ctx, explanation.ID)
	require.NoError(t, err)
	assert.Equal(t, explanation.CreatedAt, stored.CreatedAt)
}

// S13: the projection asks one question of every explanation — is this exact range
// already distilled — so only the pair travels, without the reader's text.
func TestRangeExplanationRepository_ListExplainedRangesReturnsEveryStoredPair(t *testing.T) {
	ctx := t.Context()
	sqlDB := openMigratedDB(t)
	t.Cleanup(func() { sqlDB.Close() })

	repo := sqlite.NewRangeExplanationRepository(sqlDB)

	first := newExplanation("An aggregate is a consistency boundary.")
	require.NoError(t, repo.Create(ctx, first))

	second := newExplanation("A repository per aggregate root.")
	second.DocumentID = "c6d1e9a3-0000-4000-8000-000000000002"
	second.FromPage = 24
	second.ToPage = 36
	second.Gaps = nil
	require.NoError(t, repo.Create(ctx, second))

	ranges, err := repo.ListExplainedRanges(ctx)
	require.NoError(t, err)

	assert.ElementsMatch(t, []domain.ExplainedRange{
		{DocumentID: "c6d1e9a3-0000-4000-8000-000000000001", FromPage: 88, ToPage: 114},
		{DocumentID: "c6d1e9a3-0000-4000-8000-000000000002", FromPage: 24, ToPage: 36},
	}, ranges)
}

// S13: nothing explained is an empty list, not a failed read: a corpus nobody has
// distilled yet is a normal state.
func TestRangeExplanationRepository_ListExplainedRangesWithNothingStoredIsEmpty(t *testing.T) {
	ctx := t.Context()
	sqlDB := openMigratedDB(t)
	t.Cleanup(func() { sqlDB.Close() })

	ranges, err := sqlite.NewRangeExplanationRepository(sqlDB).ListExplainedRanges(ctx)
	require.NoError(t, err)
	assert.NotNil(t, ranges)
	assert.Empty(t, ranges)
}
