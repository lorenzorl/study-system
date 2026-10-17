package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// SyncResourceUseCase upserts an external study resource by its SourceURI.
// It finds or creates the topic by name, then creates or updates the resource.
type SyncResourceUseCase struct {
	topicRepo    domain.TopicRepository
	resourceRepo domain.ResourceRepository
}

// NewSyncResourceUseCase creates a new SyncResourceUseCase with the required dependencies.
func NewSyncResourceUseCase(topicRepo domain.TopicRepository, resourceRepo domain.ResourceRepository) *SyncResourceUseCase {
	return &SyncResourceUseCase{
		topicRepo:    topicRepo,
		resourceRepo: resourceRepo,
	}
}

// Execute upserts a resource for the given topic. If the topic does not exist,
// it is created. If a resource already exists with the same SourceURI, it is
// returned as-is. Returns the resource ID on success.
func (uc *SyncResourceUseCase) Execute(ctx context.Context, topicName, title, resourceType, sourceURI string) (string, error) {
	rt := domain.ResourceType(resourceType)
	if !rt.IsValid() {
		return "", fmt.Errorf("invalid resource type %q: %w", resourceType, apperror.ErrInvalidInput)
	}

	topic, err := uc.topicRepo.UpsertByName(ctx, topicName)
	if err != nil {
		return "", fmt.Errorf("upsert topic: %w", err)
	}

	existing, err := uc.resourceRepo.FindBySourceURI(ctx, sourceURI)
	if err != nil {
		return "", fmt.Errorf("find resource by source URI: %w", err)
	}

	if existing != nil {
		return existing.ID, nil
	}

	resource := &domain.Resource{
		ID:        uuid.New().String(),
		TopicID:   topic.ID,
		Title:     title,
		Type:      rt,
		SourceURI: sourceURI,
		CreatedAt: domain.Now(),
	}

	if err := uc.resourceRepo.Create(ctx, resource); err != nil {
		return "", fmt.Errorf("create resource: %w", err)
	}

	return resource.ID, nil
}
