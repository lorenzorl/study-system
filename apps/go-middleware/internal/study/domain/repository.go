package domain

import (
	"context"
	"time"
)

// TopicRepository manages persistence of Topic entities.
type TopicRepository interface {
	UpsertByName(ctx context.Context, name string) (*Topic, error)
	ListAll(ctx context.Context) ([]Topic, error)
	Create(ctx context.Context, topic *Topic) error
	FindByID(ctx context.Context, id string) (*Topic, error)
}

// ConceptRepository manages persistence of Concept entities.
type ConceptRepository interface {
	UpsertByPath(ctx context.Context, topicID, title, filePath string) (*Concept, error)
	ListByTopicID(ctx context.Context, topicID string) ([]Concept, error)
	Create(ctx context.Context, concept *Concept) error
}

// DueCardResult is a denormalized result from the due cards query,
// joining flashcards with their concept and topic for context.
type DueCardResult struct {
	FlashcardID  string
	Front        string
	Back         string
	ConceptTitle string
	TopicName    string
	NextReview   time.Time
}

// FlashcardRepository manages persistence of Flashcard entities.
type FlashcardRepository interface {
	UpsertByObsidianID(ctx context.Context, conceptID string, cards []Flashcard) (int, error)
	FindByObsidianID(ctx context.Context, obsidianID string) (*Flashcard, error)
	FindDueWithContext(ctx context.Context, now time.Time) ([]DueCardResult, error)
}

// ResourceRepository manages persistence of Resource entities.
type ResourceRepository interface {
	FindBySourceURI(ctx context.Context, sourceURI string) (*Resource, error) // nil,nil on miss
	Create(ctx context.Context, resource *Resource) error
	// List returns every resource with its topic name, newest first.
	List(ctx context.Context) ([]ResourceWithTopic, error)
}

// CardStateRepository manages persistence of CardState entities.
type CardStateRepository interface {
	Create(ctx context.Context, state *CardState) error
	FindByFlashcardID(ctx context.Context, flashcardID string) (*CardState, error) // nil,ErrNotFound on miss
	Update(ctx context.Context, state *CardState) error
}

// ReviewLogRepository manages persistence of ReviewLog entities.
type ReviewLogRepository interface {
	Create(ctx context.Context, log *ReviewLog) error
	FindByFlashcardID(ctx context.Context, flashcardID string) ([]ReviewLog, error)
}

// RangeExplanationRepository manages persistence of RangeExplanation entities.
type RangeExplanationRepository interface {
	// Create writes the explanation and its gaps in one transaction, so a failure
	// can never leave an explanation without the gaps it was stored for.
	Create(ctx context.Context, explanation *RangeExplanation) error

	// FindByID returns the explanation with its gaps in stored order, or
	// apperror.ErrNotFound on a miss.
	FindByID(ctx context.Context, id string) (*RangeExplanation, error)
}

// RangeGapRepository manages persistence of RangeGap entities.
type RangeGapRepository interface {
	// ListByExplanationID returns the gaps of one explanation in stored order, as
	// an empty slice when it has none.
	ListByExplanationID(ctx context.Context, explanationID string) ([]RangeGap, error)
}

// ReadingProgressRepository manages persistence of ReadingProgress entities.
//
// It is the coverage axis: a range is read or not read, and there is no score.
type ReadingProgressRepository interface {
	// Upsert writes the state of one range, creating the row on its first marking.
	// The stored row is returned because created_at belongs to that first marking
	// and only the database knows it.
	Upsert(ctx context.Context, progress *ReadingProgress) (*ReadingProgress, error)

	// ListByDocument returns one document's ranges in page order, as an empty slice
	// when nothing was marked.
	ListByDocument(ctx context.Context, documentID string) ([]ReadingProgress, error)
}

// StudyPlanRepository stores the plan the agent produced and reads back the one
// active plan of a document.
type StudyPlanRepository interface {
	// CreateActive writes the plan with its items in one transaction and marks the
	// document's previous active plan superseded, keeping its rows. Two live plans
	// for one book would make "the next item" ambiguous.
	CreateActive(ctx context.Context, plan *StudyPlan) (*StudyPlan, error)

	// FindActiveByDocument returns the document's active plan with its items in
	// position order, or apperror.ErrNotFound when nobody planned it.
	FindActiveByDocument(ctx context.Context, documentID string) (*StudyPlan, error)
}
