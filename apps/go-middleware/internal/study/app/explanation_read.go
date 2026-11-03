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

// GapConflict is a gap whose state forbids the request, carrying the sentence the
// caller reads.
//
// It unwraps to apperror.ErrConflict so a transport that only knows the sentinels
// still answers 409, and the sentence says which state stands in the way: "it already
// has a card" and "it was discarded" are the same status and different advice.
type GapConflict struct {
	Sentence string
}

// Error is the caller-facing sentence.
func (c *GapConflict) Error() string { return c.Sentence }

// Unwrap makes the conflict classified by any caller that knows the sentinels.
func (c *GapConflict) Unwrap() error { return apperror.ErrConflict }

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

	// A gap with a card is not dismissed: the card exists, and a state that says
	// otherwise would let the deck and the gap list disagree about the same reader.
	// The read comes first because the state machine needs the current state, and the
	// update is transactional, so the window between them is not a decision.
	current, err := uc.gaps.FindByID(ctx, gapID)
	if err != nil {
		return domain.RangeGap{}, fmt.Errorf("find gap: %w", err)
	}
	if current.State == domain.RangeGapStateCarded {
		return domain.RangeGap{}, &GapConflict{Sentence: "this gap already has a card"}
	}

	gap, err := uc.gaps.UpdateState(ctx, gapID, state)
	if err != nil {
		return domain.RangeGap{}, fmt.Errorf("update gap state: %w", err)
	}

	return *gap, nil
}
