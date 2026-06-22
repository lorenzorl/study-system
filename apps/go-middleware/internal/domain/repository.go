package domain

import "context"

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

// FlashcardRepository manages persistence of Flashcard entities.
type FlashcardRepository interface {
	UpsertByObsidianID(ctx context.Context, conceptID string, cards []Flashcard) (int, error)
}
