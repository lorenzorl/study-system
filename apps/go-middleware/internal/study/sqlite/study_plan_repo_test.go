package sqlite_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

func samplePlan() *domain.StudyPlan {
	return &domain.StudyPlan{
		ID:          "p1",
		DocumentID:  "doc-1",
		ReadingMode: domain.ReadingModeLinear,
		Objective:   "decidir qué agregado va en qué contexto",
		Source:      "sections",
		CreatedAt:   time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC),
		Items: []domain.StudyPlanItem{
			{ID: "i1", Position: 1, Title: "Capítulo 2", FromPage: 24, ToPage: 36, Note: "", State: domain.PlanItemStatePending},
			{ID: "i2", Position: 2, Title: "Capítulo 5", FromPage: 100, ToPage: 118, Note: "volver si hace falta", State: domain.PlanItemStateSkipped},
		},
	}
}

// S7: the plan and its items land together and come back in position order.
func TestStudyPlanRepository_CreateActiveStoresThePlanWithItsItemsInOrder(t *testing.T) {
	sqlDB := openMigratedDB(t)
	repo := sqlite.NewStudyPlanRepository(sqlDB)

	stored, err := repo.CreateActive(t.Context(), samplePlan())
	require.NoError(t, err)
	assert.Equal(t, "p1", stored.ID)
	require.Len(t, stored.Items, 2)

	found, err := repo.FindActiveByDocument(t.Context(), "doc-1")
	require.NoError(t, err)

	assert.Equal(t, domain.ReadingModeLinear, found.ReadingMode)
	assert.Equal(t, "decidir qué agregado va en qué contexto", found.Objective)
	assert.Equal(t, "sections", found.Source)
	assert.Equal(t, time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC), found.CreatedAt)
	assert.Nil(t, found.SupersededAt)

	require.Len(t, found.Items, 2)
	assert.Equal(t, []int{1, 2}, []int{found.Items[0].Position, found.Items[1].Position})
	assert.Equal(t, "Capítulo 2", found.Items[0].Title)
	assert.Equal(t, domain.PlanItemStatePending, found.Items[0].State)
	assert.Equal(t, domain.PlanItemStateSkipped, found.Items[1].State)
	assert.Equal(t, "volver si hace falta", found.Items[1].Note)
}

// S7 and L4: a new plan supersedes the active one instead of deleting it, so the
// reader can throw a plan away without losing what it said.
func TestStudyPlanRepository_ANewPlanSupersedesTheActiveOneAndKeepsTheOldRows(t *testing.T) {
	sqlDB := openMigratedDB(t)
	repo := sqlite.NewStudyPlanRepository(sqlDB)
	ctx := t.Context()

	_, err := repo.CreateActive(ctx, samplePlan())
	require.NoError(t, err)

	second := samplePlan()
	second.ID = "p2"
	second.CreatedAt = second.CreatedAt.Add(time.Hour)
	second.Items = []domain.StudyPlanItem{
		{ID: "i3", Position: 1, Title: "Capítulo 1", FromPage: 5, ToPage: 23, State: domain.PlanItemStatePending},
	}
	_, err = repo.CreateActive(ctx, second)
	require.NoError(t, err)

	active, err := repo.FindActiveByDocument(ctx, "doc-1")
	require.NoError(t, err)
	assert.Equal(t, "p2", active.ID)
	require.Len(t, active.Items, 1, "the active plan carries only its own items")
	assert.Equal(t, "Capítulo 1", active.Items[0].Title)

	var plans, superseded, items int
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM study_plans`).Scan(&plans))
	require.NoError(t, sqlDB.QueryRow(
		`SELECT count(*) FROM study_plans WHERE superseded_at IS NOT NULL`).Scan(&superseded))
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM study_plan_items`).Scan(&items))

	assert.Equal(t, 2, plans, "the superseded plan is kept, not deleted")
	assert.Equal(t, 1, superseded, "exactly one plan is superseded")
	assert.Equal(t, 3, items, "the superseded plan's items are kept with it")
}

// S10: a document with no active plan is not an empty plan.
func TestStudyPlanRepository_ADocumentWithoutAnActivePlanIsNotFound(t *testing.T) {
	sqlDB := openMigratedDB(t)
	repo := sqlite.NewStudyPlanRepository(sqlDB)

	_, err := repo.FindActiveByDocument(t.Context(), "doc-nobody-planned")

	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

// S13: the projection reads the items of every active plan together, because "the
// next thing to read" is a question across books. A superseded plan must not
// answer it: those rows are kept so the reader does not lose what they said, not
// so they keep recommending chapters.
func TestStudyPlanRepository_ListActiveItemsReturnsOnlyActivePlansItems(t *testing.T) {
	sqlDB := openMigratedDB(t)
	repo := sqlite.NewStudyPlanRepository(sqlDB)
	ctx := t.Context()

	base := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	superseded := &domain.StudyPlan{
		ID: "p-old", DocumentID: "doc-1", ReadingMode: domain.ReadingModeLinear, CreatedAt: base,
		Items: []domain.StudyPlanItem{
			{ID: "old-1", Position: 1, Title: "Viejo", FromPage: 1, ToPage: 10, State: domain.PlanItemStatePending},
		},
	}
	_, err := repo.CreateActive(ctx, superseded)
	require.NoError(t, err)

	active := &domain.StudyPlan{
		ID: "p-new", DocumentID: "doc-1", ReadingMode: domain.ReadingModeLinear,
		CreatedAt: base.Add(time.Hour),
		Items: []domain.StudyPlanItem{
			{ID: "new-1", Position: 1, Title: "Capítulo 1", FromPage: 5, ToPage: 23, State: domain.PlanItemStatePending},
			{ID: "new-2", Position: 2, Title: "Capítulo 2", FromPage: 24, ToPage: 36, State: domain.PlanItemStateSkipped},
		},
	}
	_, err = repo.CreateActive(ctx, active)
	require.NoError(t, err)

	other := &domain.StudyPlan{
		ID: "p-other", DocumentID: "doc-0", ReadingMode: domain.ReadingModeProject,
		CreatedAt: base,
		Items: []domain.StudyPlanItem{
			{ID: "other-1", Position: 1, Title: "Cap 1", FromPage: 1, ToPage: 2, State: domain.PlanItemStatePending},
		},
	}
	_, err = repo.CreateActive(ctx, other)
	require.NoError(t, err)

	items, err := repo.ListActiveItems(ctx)
	require.NoError(t, err)

	require.Len(t, items, 3, "the superseded plan contributes no item")

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.Item.ID)
	}
	assert.Equal(t, []string{"other-1", "new-1", "new-2"}, ids, "document order, then position")
	assert.Equal(t, "doc-0", items[0].DocumentID)
	assert.Equal(t, "doc-1", items[1].DocumentID)
	assert.Equal(t, "Capítulo 2", items[2].Item.Title)
	assert.Equal(t, domain.PlanItemStateSkipped, items[2].Item.State)
}
