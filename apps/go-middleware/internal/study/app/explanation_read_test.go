package app_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

type fakeGatewayRepo struct {
	gaps        []domain.RangeGap
	explanation *domain.RangeExplanation
	updated     []struct {
		gapID string
		state domain.RangeGapState
	}
	err error
}

func (f *fakeGatewayRepo) UpdateState(
	ctx context.Context,
	gapID string,
	state domain.RangeGapState,
) (*domain.RangeGap, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, gap := range f.gaps {
		if gap.ID == gapID {
			gap.State = state
			f.updated = append(f.updated, struct {
				gapID string
				state domain.RangeGapState
			}{gapID, state})
			return &gap, nil
		}
	}
	return nil, apperror.ErrNotFound
}

func (f *fakeGatewayRepo) FindByID(ctx context.Context, gapID string) (*domain.RangeGap, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, gap := range f.gaps {
		if gap.ID == gapID {
			found := gap
			return &found, nil
		}
	}
	return nil, apperror.ErrNotFound
}

func (f *fakeGatewayRepo) ListByExplanationID(ctx context.Context, explanationID string) ([]domain.RangeGap, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.gaps, nil
}

type fakeExplanationRepo struct {
	explanation *domain.RangeExplanation
	err         error
}

func (f *fakeExplanationRepo) Create(ctx context.Context, explanation *domain.RangeExplanation) error {
	return f.err
}

func (f *fakeExplanationRepo) ListExplainedRanges(ctx context.Context) ([]domain.ExplainedRange, error) {
	return nil, f.err
}

func (f *fakeExplanationRepo) FindByID(ctx context.Context, id string) (*domain.RangeExplanation, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.explanation == nil || f.explanation.ID != id {
		return nil, apperror.ErrNotFound
	}
	return f.explanation, nil
}

// S2: a stored explanation is readable without calling the model again.
func TestGetExplanation_ReturnsTheStoredTextAndItsGaps(t *testing.T) {
	stored := &domain.RangeExplanation{
		ID: "e1", DocumentID: "doc-1", FromPage: 15, ToPage: 23, Text: "mis palabras",
		Gaps: []domain.RangeGap{{ID: "g1", Point: "un punto", Page: 19, Quote: "una cita", State: domain.RangeGapStateOpen}},
	}
	useCase := app.NewGetExplanationUseCase(&fakeExplanationRepo{explanation: stored})

	found, err := useCase.Execute(t.Context(), "e1")
	require.NoError(t, err)
	assert.Equal(t, "mis palabras", found.Text)
	require.Len(t, found.Gaps, 1)
	assert.Equal(t, "g1", found.Gaps[0].ID)
}

func TestGetExplanation_UnknownIsNotFound(t *testing.T) {
	useCase := app.NewGetExplanationUseCase(&fakeExplanationRepo{})

	_, err := useCase.Execute(t.Context(), "no-such-explanation")

	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

func TestGetExplanation_RefusesABlankID(t *testing.T) {
	useCase := app.NewGetExplanationUseCase(&fakeExplanationRepo{})

	_, err := useCase.Execute(t.Context(), "  ")

	assert.ErrorIs(t, err, apperror.ErrInvalidInput)
}

// S7: discarding and undoing are the two client-settable states.
func TestSetGapState_AcceptsDiscardedAndOpen(t *testing.T) {
	for _, state := range []string{"discarded", "open"} {
		t.Run(state, func(t *testing.T) {
			repo := &fakeGatewayRepo{gaps: []domain.RangeGap{{ID: "g1", State: domain.RangeGapStateOpen}}}
			useCase := app.NewSetGapStateUseCase(repo)

			updated, err := useCase.Execute(t.Context(), app.SetGapStateInput{GapID: "g1", State: state})
			require.NoError(t, err)
			assert.Equal(t, domain.RangeGapState(state), updated.State)
			require.Len(t, repo.updated, 1)
		})
	}
}

// S6: `carded` is the accept route's fact, not a client's claim. A client that could
// set it could say a card exists when none does.
func TestSetGapState_RefusesCardedAndTheUnknowns(t *testing.T) {
	cases := []struct {
		name  string
		input app.SetGapStateInput
	}{
		{name: "carded", input: app.SetGapStateInput{GapID: "g1", State: "carded"}},
		{name: "unknown word", input: app.SetGapStateInput{GapID: "g1", State: "half"}},
		{name: "blank gap", input: app.SetGapStateInput{GapID: "  ", State: "discarded"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			repo := &fakeGatewayRepo{gaps: []domain.RangeGap{{ID: "g1", State: domain.RangeGapStateOpen}}}
			useCase := app.NewSetGapStateUseCase(repo)

			_, err := useCase.Execute(t.Context(), testCase.input)

			assert.ErrorIs(t, err, apperror.ErrInvalidInput)
			assert.Empty(t, repo.updated, "a refused state change must not be written")
		})
	}
}

// S7: a gap with a card keeps its state. Discarding it would let the deck and the gap
// list disagree about the same reader, and the way back would then be a second card.
func TestSetGapState_RefusesToDiscardAGapThatHasACard(t *testing.T) {
	repo := &fakeGatewayRepo{gaps: []domain.RangeGap{
		{ID: "g1", State: domain.RangeGapStateCarded},
	}}
	useCase := app.NewSetGapStateUseCase(repo)

	_, err := useCase.Execute(t.Context(), app.SetGapStateInput{GapID: "g1", State: "discarded"})

	assert.ErrorIs(t, err, apperror.ErrConflict)
	var conflict *app.GapConflict
	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, "this gap already has a card", conflict.Error())
	assert.Empty(t, repo.updated, "a refused state change must not be written")
}

func TestSetGapState_UnknownGapIsNotFound(t *testing.T) {
	useCase := app.NewSetGapStateUseCase(&fakeGatewayRepo{})

	_, err := useCase.Execute(t.Context(), app.SetGapStateInput{GapID: "no-such-gap", State: "discarded"})

	assert.ErrorIs(t, err, apperror.ErrNotFound)
}
