package app

// Reading and dismissing what a stored explanation found, one gap at a time.

import (
	"context"
	"fmt"
	"strings"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// GetExplanationUseCase reads a stored explanation with its gaps.
//
// It exists so a page can be opened again without paying for the model: the reader's
// words and the gaps a run produced are stored, and re-reading them is the point. A
// reload that called the model would cost money, answer differently, and lose the
// gaps the reader had already discarded.
type GetExplanationUseCase struct {
	explanations domain.RangeExplanationRepository
}

// NewGetExplanationUseCase creates a GetExplanationUseCase.
func NewGetExplanationUseCase(explanations domain.RangeExplanationRepository) *GetExplanationUseCase {
	return &GetExplanationUseCase{explanations: explanations}
}

// Execute returns the stored explanation and its gaps, or apperror.ErrNotFound.
func (uc *GetExplanationUseCase) Execute(
	ctx context.Context,
	explanationID string,
) (domain.RangeExplanation, error) {
	explanationID = strings.TrimSpace(explanationID)
	if explanationID == "" {
		return domain.RangeExplanation{}, fmt.Errorf("explanation_id: %w", apperror.ErrInvalidInput)
	}

	explanation, err := uc.explanations.FindByID(ctx, explanationID)
	if err != nil {
		return domain.RangeExplanation{}, fmt.Errorf("find explanation: %w", err)
	}

	return *explanation, nil
}

// SetGapStateInput is one request to change a gap's state.
type SetGapStateInput struct {
	GapID string

	// State is `discarded` or `open`.
	State string
}

// SetGapStateUseCase discards a gap, or returns a discarded one to open.
//
// Only those two states are settable here, and `carded` deliberately is not: it
// says a card exists, and only the route that writes the card can say that. A client
// able to set it could make the gap list claim cards the deck does not have.
type SetGapStateUseCase struct {
	gaps domain.RangeGapRepository
}

// NewSetGapStateUseCase creates a SetGapStateUseCase.
func NewSetGapStateUseCase(gaps domain.RangeGapRepository) *SetGapStateUseCase {
	return &SetGapStateUseCase{gaps: gaps}
}

// Execute writes the state and returns the gap as stored, evidence included: the
// point, the page and the quote are what the reader still needs to see after
// discarding it, and they are not part of what the state changes.
func (uc *SetGapStateUseCase) Execute(
	ctx context.Context,
	input SetGapStateInput,
) (domain.RangeGap, error) {
	gapID := strings.TrimSpace(input.GapID)
	if gapID == "" {
		return domain.RangeGap{}, fmt.Errorf("gap_id: %w", apperror.ErrInvalidInput)
	}

	state := domain.RangeGapState(strings.TrimSpace(input.State))
	if state != domain.RangeGapStateOpen && state != domain.RangeGapStateDiscarded {
		return domain.RangeGap{}, fmt.Errorf("state %q: %w", input.State, apperror.ErrInvalidInput)
	}

	gap, err := uc.gaps.UpdateState(ctx, gapID, state)
	if err != nil {
		return domain.RangeGap{}, fmt.Errorf("update gap state: %w", err)
	}

	return *gap, nil
}
