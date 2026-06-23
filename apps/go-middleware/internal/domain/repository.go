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
	UpdateDifyDocumentID(ctx context.Context, id, difyDocumentID string) error
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
