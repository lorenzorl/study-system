package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// SubmitReviewUseCase records a review attempt for a flashcard, recalculates
// the next review state via the FSRS algorithm, and persists both the updated
// CardState and the ReviewLog.
type SubmitReviewUseCase struct {
	cardStateRepo domain.CardStateRepository
	reviewLogRepo domain.ReviewLogRepository
	fsrsAlgo      domain.FSRSAlgorithm
}

// NewSubmitReviewUseCase creates a new SubmitReviewUseCase with the required dependencies.
func NewSubmitReviewUseCase(
	cardStateRepo domain.CardStateRepository,
	reviewLogRepo domain.ReviewLogRepository,
	fsrsAlgo domain.FSRSAlgorithm,
) *SubmitReviewUseCase {
	return &SubmitReviewUseCase{
		cardStateRepo: cardStateRepo,
		reviewLogRepo: reviewLogRepo,
		fsrsAlgo:      fsrsAlgo,
	}
}

// Execute processes a flashcard review, updates the scheduling state, and persists
// a ReviewLog. Returns the updated CardState so the caller can inspect the new
// NextReview time.
func (uc *SubmitReviewUseCase) Execute(ctx context.Context, flashcardID string, grade int, durationMs int) (domain.CardState, error) {
	if grade < 1 || grade > 4 {
		return domain.CardState{}, fmt.Errorf("grade %d: %w", grade, domain.ErrInvalidInput)
	}

	current, err := uc.cardStateRepo.FindByFlashcardID(ctx, flashcardID)
	if err != nil {
		return domain.CardState{}, fmt.Errorf("find card state: %w", err)
	}
	if current == nil {
		return domain.CardState{}, fmt.Errorf("flashcard %q has no card state: %w", flashcardID, domain.ErrNotFound)
	}

	newState := uc.fsrsAlgo.CalculateNextState(*current, grade)

	updatedState := domain.CardState{
		ID:          current.ID,
		FlashcardID: current.FlashcardID,
		Stability:   newState.Stability,
		Difficulty:  newState.Difficulty,
		NextReview:  newState.NextReview,
		LastReview:  newState.LastReview,
	}

	if err := uc.cardStateRepo.Update(ctx, &updatedState); err != nil {
		return domain.CardState{}, fmt.Errorf("update card state: %w", err)
	}

	reviewLog := &domain.ReviewLog{
		ID:          uuid.New().String(),
		FlashcardID: flashcardID,
		Grade:       grade,
		DurationMs:  durationMs,
		CreatedAt:   time.Now().UTC(),
	}

	if err := uc.reviewLogRepo.Create(ctx, reviewLog); err != nil {
		return domain.CardState{}, fmt.Errorf("create review log: %w", err)
	}

	return newState, nil
}
