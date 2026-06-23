package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// SyncFlashcardsUseCase batch-upserts flashcards by their ObsidianID
// for a given concept. For each new flashcard (one without a CardState),
// it auto-creates an initial CardState. Returns the number of cards processed.
type SyncFlashcardsUseCase struct {
	conceptRepo    domain.ConceptRepository
	flashcardRepo  domain.FlashcardRepository
	cardStateRepo  domain.CardStateRepository
}

// NewSyncFlashcardsUseCase creates a new SyncFlashcardsUseCase with the required dependencies.
func NewSyncFlashcardsUseCase(
	conceptRepo domain.ConceptRepository,
	flashcardRepo domain.FlashcardRepository,
	cardStateRepo domain.CardStateRepository,
) *SyncFlashcardsUseCase {
	return &SyncFlashcardsUseCase{
		conceptRepo:   conceptRepo,
		flashcardRepo: flashcardRepo,
		cardStateRepo: cardStateRepo,
	}
}

// Execute upserts flashcards for a concept and auto-creates CardState entries
// for newly inserted flashcards. Returns the count of cards processed.
func (uc *SyncFlashcardsUseCase) Execute(ctx context.Context, conceptID string, cards []domain.Flashcard) (int, error) {
	count, err := uc.flashcardRepo.UpsertByObsidianID(ctx, conceptID, cards)
	if err != nil {
		return 0, fmt.Errorf("upsert flashcards: %w", err)
	}

	now := time.Now().UTC()

	for _, card := range cards {
		f, err := uc.flashcardRepo.FindByObsidianID(ctx, card.ObsidianID)
		if err != nil {
			return count, fmt.Errorf("find flashcard by obsidian id %q: %w", card.ObsidianID, err)
		}

		cs, err := uc.cardStateRepo.FindByFlashcardID(ctx, f.ID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return count, fmt.Errorf("find card state for flashcard %q: %w", f.ID, err)
		}

		if cs == nil {
			initialState := domain.CardState{
				ID:          uuid.New().String(),
				FlashcardID: f.ID,
				Stability:   0,
				Difficulty:  0,
				NextReview:  now,
				LastReview:  now,
			}
			if err := uc.cardStateRepo.Create(ctx, &initialState); err != nil {
				return count, fmt.Errorf("create initial card state for flashcard %q: %w", f.ID, err)
			}
		}
	}

	return count, nil
}
