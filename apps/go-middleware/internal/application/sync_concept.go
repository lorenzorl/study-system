package application

import (
	"context"
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// SyncConceptUseCase orchestrates the creation or retrieval of a concept
// within its topic. It upserts the topic by name, then upserts the concept
// by file path within that topic. Returns the concept's ID.
type SyncConceptUseCase struct {
	topicRepo   domain.TopicRepository
	conceptRepo domain.ConceptRepository
}

// NewSyncConceptUseCase creates a new SyncConceptUseCase with the required dependencies.
func NewSyncConceptUseCase(topicRepo domain.TopicRepository, conceptRepo domain.ConceptRepository) *SyncConceptUseCase {
	return &SyncConceptUseCase{
		topicRepo:   topicRepo,
		conceptRepo: conceptRepo,
	}
}

// Execute syncs a concept by ensuring its topic exists, then upserting the concept.
// Returns the concept's ID on success.
func (uc *SyncConceptUseCase) Execute(ctx context.Context, topicName, conceptTitle, filePath string) (string, error) {
	topic, err := uc.topicRepo.UpsertByName(ctx, topicName)
	if err != nil {
		return "", fmt.Errorf("upsert topic: %w", err)
	}

	concept, err := uc.conceptRepo.UpsertByPath(ctx, topic.ID, conceptTitle, filePath)
	if err != nil {
		return "", fmt.Errorf("upsert concept: %w", err)
	}

	return concept.ID, nil
}
