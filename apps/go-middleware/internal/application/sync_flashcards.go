package application

import (
	"context"
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// SyncFlashcardsUseCase batch-upserts flashcards by their ObsidianID
// for a given concept. Returns the number of cards processed.
type SyncFlashcardsUseCase struct {
	conceptRepo    domain.ConceptRepository
	flashcardRepo  domain.FlashcardRepository
}

// NewSyncFlashcardsUseCase creates a new SyncFlashcardsUseCase with the required dependencies.
func NewSyncFlashcardsUseCase(conceptRepo domain.ConceptRepository, flashcardRepo domain.FlashcardRepository) *SyncFlashcardsUseCase {
	return &SyncFlashcardsUseCase{
		conceptRepo:   conceptRepo,
		flashcardRepo: flashcardRepo,
	}
}

// Execute upserts flashcards for a concept and returns the count of cards processed.
func (uc *SyncFlashcardsUseCase) Execute(ctx context.Context, conceptID string, cards []domain.Flashcard) (int, error) {
	count, err := uc.flashcardRepo.UpsertByObsidianID(ctx, conceptID, cards)
	if err != nil {
		return 0, fmt.Errorf("upsert flashcards: %w", err)
	}

	return count, nil
}
