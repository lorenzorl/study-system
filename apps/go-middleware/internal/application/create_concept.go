package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// CreateConceptUseCase handles manual concept creation within a topic.
type CreateConceptUseCase struct {
	topicRepo   domain.TopicRepository
	conceptRepo domain.ConceptRepository
}

// NewCreateConceptUseCase creates a new CreateConceptUseCase with the required dependencies.
func NewCreateConceptUseCase(topicRepo domain.TopicRepository, conceptRepo domain.ConceptRepository) *CreateConceptUseCase {
	return &CreateConceptUseCase{
		topicRepo:   topicRepo,
		conceptRepo: conceptRepo,
	}
}

// Execute creates a new concept under the given topic. Validates that
// the topic exists and generates a synthetic file_path scoped by topic name.
func (uc *CreateConceptUseCase) Execute(ctx context.Context, topicID, title string) (*domain.Concept, error) {
	if title == "" {
		return nil, domain.ErrInvalidInput
	}

	topic, err := uc.topicRepo.FindByID(ctx, topicID)
	if err != nil {
		return nil, fmt.Errorf("find topic: %w", err)
	}

	filePath := fmt.Sprintf("manual/%s/%s.md", topic.Name, title)

	concept := &domain.Concept{
		ID:        uuid.New().String(),
		TopicID:   topicID,
		Title:     title,
		FilePath:  filePath,
		CreatedAt: time.Now().UTC(),
	}

	if err := uc.conceptRepo.Create(ctx, concept); err != nil {
		return nil, fmt.Errorf("create concept: %w", err)
	}

	return concept, nil
}
