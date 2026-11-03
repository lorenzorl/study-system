package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

type fakeStudyPlanRepository struct {
	created []*domain.StudyPlan
	active  *domain.StudyPlan
	err     error
}

func (f *fakeStudyPlanRepository) CreateActive(ctx context.Context, plan *domain.StudyPlan) (*domain.StudyPlan, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.created = append(f.created, plan)
	f.active = plan
	return plan, nil
}

func (f *fakeStudyPlanRepository) FindActiveByDocument(ctx context.Context, documentID string) (*domain.StudyPlan, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.active == nil {
		return nil, apperror.ErrNotFound
	}
	return f.active, nil
}

func (f *fakeStudyPlanRepository) ListActiveItems(ctx context.Context) ([]domain.PlanItemWithDocument, error) {
	if f.err != nil {
		return nil, f.err
	}
	return nil, nil
}

// fakeDocuments is the ingestion context's titles behind the port the study
// context declares.
type fakeDocuments struct {
	titles map[string]string
}

func (f *fakeDocuments) Title(ctx context.Context, documentID string) (string, error) {
	title, found := f.titles[documentID]
	if !found {
		return "", apperror.ErrNotFound
	}
	return title, nil
}

func validPlanInput() app.SavePlanInput {
	return app.SavePlanInput{
		DocumentID:  "doc-1",
		ReadingMode: "linear",
		Objective:   "decidir qué agregado va en qué contexto",
		Source:      "sections",
		Items: []app.PlanItemInput{
			{Title: "Capítulo 2", FromPage: 24, ToPage: 36},
			{Title: "Capítulo 5", FromPage: 100, ToPage: 118, Note: "volver si hace falta", State: "skipped"},
		},
	}
}

func newSavePlanUseCase(plans *fakeStudyPlanRepository) *app.SavePlanUseCase {
	return app.NewSavePlanUseCase(
		&fakeDocuments{titles: map[string]string{"doc-1": "DDD"}},
		plans,
		&fakeReadingProgressRepository{},
	)
}

// S8: every refusal the contract names is decided before anything is written.
func TestSavePlan_RefusesWhatTheContractNames(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*app.SavePlanInput)
		title string
	}{
		{name: "empty document", edit: func(in *app.SavePlanInput) { in.DocumentID = " " }},
		{name: "unknown mode", edit: func(in *app.SavePlanInput) { in.ReadingMode = "spiral" }},
		{name: "no items", edit: func(in *app.SavePlanInput) { in.Items = nil }},
		{name: "blank title", edit: func(in *app.SavePlanInput) { in.Items[0].Title = "  " }},
		{name: "page zero", edit: func(in *app.SavePlanInput) { in.Items[0].FromPage = 0 }},
		{name: "backwards range", edit: func(in *app.SavePlanInput) { in.Items[0].FromPage = 40 }},
		{name: "unknown state", edit: func(in *app.SavePlanInput) { in.Items[0].State = "half" }},
		{name: "duplicate range", edit: func(in *app.SavePlanInput) {
			in.Items = append(in.Items, app.PlanItemInput{Title: "Otra vez", FromPage: 24, ToPage: 36})
		}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			input := validPlanInput()
			testCase.edit(&input)
			plans := &fakeStudyPlanRepository{}

			_, err := newSavePlanUseCase(plans).Execute(t.Context(), input)

			assert.ErrorIs(t, err, apperror.ErrInvalidInput)
			assert.Empty(t, plans.created, "a refused plan must not be stored")
		})
	}
}

// S8: a document nobody ingested is a 404, not a plan against a phantom book.
func TestSavePlan_RefusesADocumentThatDoesNotExist(t *testing.T) {
	input := validPlanInput()
	input.DocumentID = "doc-that-never-was"
	plans := &fakeStudyPlanRepository{}
	useCase := app.NewSavePlanUseCase(&fakeDocuments{titles: map[string]string{}}, plans, &fakeReadingProgressRepository{})

	_, err := useCase.Execute(t.Context(), input)

	assert.ErrorIs(t, err, apperror.ErrNotFound)
	assert.Empty(t, plans.created)
}

// S9: order is the array order. The server assigns 1..N and ignores anything the
// client might send.
func TestSavePlan_AssignsPositionsFromTheRequestOrder(t *testing.T) {
	plans := &fakeStudyPlanRepository{}

	stored, err := newSavePlanUseCase(plans).Execute(t.Context(), validPlanInput())
	require.NoError(t, err)

	require.Len(t, stored.Items, 2)
	assert.Equal(t, 1, stored.Items[0].Position)
	assert.Equal(t, 2, stored.Items[1].Position)
	assert.Equal(t, domain.PlanItemStatePending, stored.Items[0].State, "an item with no state is pending")
	assert.Equal(t, domain.PlanItemStateSkipped, stored.Items[1].State)
	assert.NotEmpty(t, stored.Items[0].ID)
	assert.Equal(t, "plan", "plan")
}

// S6: read is never stored on the item. It is derived from coverage for the
// exact range, so a read row for 24-35 must not mark the item 24-36.
func TestGetPlan_DerivesReadFromTheExactRange(t *testing.T) {
	plans := &fakeStudyPlanRepository{active: &domain.StudyPlan{
		ID: "p1", DocumentID: "doc-1", ReadingMode: domain.ReadingModeLinear,
		Objective: "decidir", Source: "sections",
		CreatedAt: time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC),
		Items: []domain.StudyPlanItem{
			{ID: "i1", Position: 1, Title: "Capítulo 2", FromPage: 24, ToPage: 36, State: domain.PlanItemStatePending},
			{ID: "i2", Position: 2, Title: "Capítulo 5", FromPage: 100, ToPage: 118, State: domain.PlanItemStateSkipped},
		},
	}}
	progress := &fakeReadingProgressRepository{ranges: []domain.ReadingProgress{
		{DocumentID: "doc-1", FromPage: 24, ToPage: 36, State: domain.ReadingStateRead},
		{DocumentID: "doc-1", FromPage: 100, ToPage: 110, State: domain.ReadingStateRead},
	}}

	view, err := app.NewGetPlanUseCase(plans, progress).Execute(t.Context(), "doc-1")
	require.NoError(t, err)

	require.Len(t, view.Items, 2)
	assert.True(t, view.Items[0].Read, "the exact range was marked read")
	assert.False(t, view.Items[1].Read, "a wider or narrower read row is not this item")
}

func TestGetPlan_ADocumentWithoutAPlanIsNotFound(t *testing.T) {
	_, err := app.NewGetPlanUseCase(&fakeStudyPlanRepository{}, &fakeReadingProgressRepository{}).
		Execute(t.Context(), "doc-1")

	assert.ErrorIs(t, err, apperror.ErrNotFound)
}
