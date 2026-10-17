package http

import (
	"context"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// The HTTP layer declares the use case contracts it needs, rather than
// importing concrete use case types. That keeps handlers mockable and lets the
// application layer change shape without breaking transport tests.

// SyncConceptUseCase syncs a concept and returns its ID.
type SyncConceptUseCase interface {
	Execute(ctx context.Context, topicName, conceptTitle, filePath string) (string, error)
}

// SyncFlashcardsUseCase upserts flashcards for a concept and returns how many
// were processed.
type SyncFlashcardsUseCase interface {
	Execute(ctx context.Context, conceptID string, cards []domain.Flashcard) (int, error)
}

// ListConceptsUseCase returns the topic-to-concept tree.
type ListConceptsUseCase interface {
	Execute(ctx context.Context) ([]app.TopicWithConcepts, error)
}

// CreateTopicUseCase creates a topic.
type CreateTopicUseCase interface {
	Execute(ctx context.Context, name string) (*domain.Topic, error)
}

// CreateConceptUseCase creates a concept inside an existing topic.
type CreateConceptUseCase interface {
	Execute(ctx context.Context, topicID, title string) (*domain.Concept, error)
}

// SyncResourceUseCase upserts an external study resource.
type SyncResourceUseCase interface {
	Execute(ctx context.Context, topicName, title, resourceType, sourceURI string) (string, error)
}

// GetDueCardsUseCase returns flashcards whose next review is due.
type GetDueCardsUseCase interface {
	Execute(ctx context.Context) ([]app.DueCard, error)
}

// SubmitReviewUseCase records a review and returns the recomputed card state.
type SubmitReviewUseCase interface {
	Execute(ctx context.Context, flashcardID string, grade, durationMs int) (domain.CardState, error)
}
