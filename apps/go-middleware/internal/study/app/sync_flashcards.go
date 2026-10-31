package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// SyncFlashcardsUseCase batch-upserts flashcards by their ObsidianID
// for a given concept. For each new flashcard (one without a CardState),
// it auto-creates an initial CardState. Returns the number of cards processed.
type SyncFlashcardsUseCase struct {
	conceptRepo   domain.ConceptRepository
	flashcardRepo domain.FlashcardRepository
	cardStateRepo domain.CardStateRepository
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
//
// A card may arrive without an ObsidianID: an agent proposing from the corpus has
// no note to mirror and cannot know one. Those cards are identified here, before
// anything is written, so that writing the same proposal twice updates one card
// instead of appending a second.
func (uc *SyncFlashcardsUseCase) Execute(ctx context.Context, conceptID string, cards []domain.Flashcard) (int, error) {
	identified, err := identifyCards(conceptID, cards)
	if err != nil {
		return 0, err
	}

	count, err := uc.flashcardRepo.UpsertByObsidianID(ctx, conceptID, identified)
	if err != nil {
		return 0, fmt.Errorf("upsert flashcards: %w", err)
	}

	now := domain.Now()

	for _, card := range identified {
		f, err := uc.flashcardRepo.FindByObsidianID(ctx, card.ObsidianID)
		if err != nil {
			return count, fmt.Errorf("find flashcard by obsidian id %q: %w", card.ObsidianID, err)
		}

		cs, err := uc.cardStateRepo.FindByFlashcardID(ctx, f.ID)
		if err != nil && !errors.Is(err, apperror.ErrNotFound) {
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

// identifyCards gives every card that arrived without an ObsidianID the identity
// this server derives, and refuses the ones that cannot have one.
//
// The validation is scoped to the derived path on purpose. A caller that supplies
// its own id is the note sync, whose acceptance rules predate this path; refusing
// its cards here would change a contract nobody asked to change. A card with no
// question cannot be identified at all, and one with no answer is a question the
// review queue can never resolve, so both are refused before the write.
func identifyCards(conceptID string, cards []domain.Flashcard) ([]domain.Flashcard, error) {
	identified := make([]domain.Flashcard, len(cards))
	copy(identified, cards)

	for index, card := range identified {
		if card.ObsidianID != "" {
			continue
		}

		if strings.TrimSpace(card.Question) == "" {
			return nil, fmt.Errorf("a card without an id needs a question: %w", apperror.ErrInvalidInput)
		}

		if strings.TrimSpace(card.Answer) == "" {
			return nil, fmt.Errorf("a card without an id needs an answer: %w", apperror.ErrInvalidInput)
		}

		identified[index].ObsidianID = domain.CardIdentity(conceptID, card.Question)
	}

	return identified, nil
}
