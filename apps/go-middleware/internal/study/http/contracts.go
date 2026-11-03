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

// ListResourcesUseCase returns every stored study resource with its topic name.
type ListResourcesUseCase interface {
	Execute(ctx context.Context) ([]app.ResourceSummary, error)
}

// GetDueCardsUseCase returns flashcards whose next review is due.
type GetDueCardsUseCase interface {
	Execute(ctx context.Context) ([]app.DueCard, error)
}

// SubmitReviewUseCase records a review and returns the recomputed card state.
type SubmitReviewUseCase interface {
	Execute(ctx context.Context, flashcardID string, grade, durationMs int) (domain.CardState, error)
}

// ExplainRangeUseCase explains a page range and stores the gaps it left out.
//
// The range is passed as the application's own input, not as raw JSON: deciding
// what a valid explanation is belongs to the application layer, and the transport
// only decides whether the body was JSON with a text at all.
type ExplainRangeUseCase interface {
	// Execute returns the stored explanation with its accepted gaps. A refused
	// explanation is an *app.ExplanationRefusal (a 400 carrying its sentence), a
	// refused or unknown range is apperror.ErrInvalidInput or apperror.ErrNotFound,
	// and a model failure is a plain error, which is a 500 and nothing stored.
	Execute(ctx context.Context, input app.ExplainRangeInput) (domain.RangeExplanation, error)
}
