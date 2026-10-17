package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// CreateTopicUseCase handles manual topic creation.
type CreateTopicUseCase struct {
	topicRepo domain.TopicRepository
}

// NewCreateTopicUseCase creates a new CreateTopicUseCase with the required dependencies.
func NewCreateTopicUseCase(topicRepo domain.TopicRepository) *CreateTopicUseCase {
	return &CreateTopicUseCase{topicRepo: topicRepo}
}

// Execute creates a new topic with the given name. Returns the created topic
// or a domain error on failure.
func (uc *CreateTopicUseCase) Execute(ctx context.Context, name string) (*domain.Topic, error) {
	if name == "" {
		return nil, apperror.ErrInvalidInput
	}

	topic := &domain.Topic{
		ID:        uuid.New().String(),
		Name:      name,
		CreatedAt: domain.Now(),
	}

	if err := uc.topicRepo.Create(ctx, topic); err != nil {
		return nil, fmt.Errorf("create topic: %w", err)
	}

	return topic, nil
}
