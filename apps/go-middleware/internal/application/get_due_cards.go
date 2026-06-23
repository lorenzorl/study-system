package application

import (
	"context"
	"fmt"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// DueCard is an application-layer response DTO for due flashcards,
// carrying the flashcard content plus Concept and Topic context.
type DueCard struct {
	FlashcardID  string    `json:"flashcard_id"`
	Front        string    `json:"front"`
	Back         string    `json:"back"`
	ConceptTitle string    `json:"concept_title"`
	TopicName    string    `json:"topic_name"`
	NextReview   time.Time `json:"next_review"`
}

// GetDueCardsUseCase returns all flashcards whose next review is due (now or earlier),
// with their Concept and Topic context attached.
type GetDueCardsUseCase struct {
	flashcardRepo domain.FlashcardRepository
}

// NewGetDueCardsUseCase creates a new GetDueCardsUseCase with the required dependencies.
func NewGetDueCardsUseCase(flashcardRepo domain.FlashcardRepository) *GetDueCardsUseCase {
	return &GetDueCardsUseCase{flashcardRepo: flashcardRepo}
}

// Execute returns all due flashcards with their full context.
// Returns an empty slice when no cards are due (not an error).
func (uc *GetDueCardsUseCase) Execute(ctx context.Context) ([]DueCard, error) {
	results, err := uc.flashcardRepo.FindDueWithContext(ctx, time.Now())
	if err != nil {
		return nil, fmt.Errorf("find due cards: %w", err)
	}

	cards := make([]DueCard, 0, len(results))
	for _, r := range results {
		cards = append(cards, DueCard{
			FlashcardID:  r.FlashcardID,
			Front:        r.Front,
			Back:         r.Back,
			ConceptTitle: r.ConceptTitle,
			TopicName:    r.TopicName,
			NextReview:   r.NextReview,
		})
	}

	return cards, nil
}
