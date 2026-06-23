package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
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
// it is created. If the resource already exists by SourceURI, its DifyDocumentID
// is updated. Returns the resource ID on success.
func (uc *SyncResourceUseCase) Execute(ctx context.Context, topicName, title, resourceType, sourceURI, difyDocumentID string) (string, error) {
	rt := domain.ResourceType(resourceType)
	if !rt.IsValid() {
		return "", fmt.Errorf("invalid resource type %q: %w", resourceType, domain.ErrInvalidInput)
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
		if err := uc.resourceRepo.UpdateDifyDocumentID(ctx, existing.ID, difyDocumentID); err != nil {
			return "", fmt.Errorf("update dify document id: %w", err)
		}
		return existing.ID, nil
	}

	resource := &domain.Resource{
		ID:             uuid.New().String(),
		TopicID:        topic.ID,
		Title:          title,
		Type:           rt,
		SourceURI:      sourceURI,
		DifyDocumentID: difyDocumentID,
		CreatedAt:      time.Now().UTC(),
	}

	if err := uc.resourceRepo.Create(ctx, resource); err != nil {
		return "", fmt.Errorf("create resource: %w", err)
	}

	return resource.ID, nil
}
