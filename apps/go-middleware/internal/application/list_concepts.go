package application

import (
	"context"
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// TopicWithConcepts groups a topic with its nested concepts for the API response.
type TopicWithConcepts struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Concepts []ConceptListItem `json:"concepts"`
}

// ConceptListItem is a lightweight concept representation for listing.
type ConceptListItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	FilePath string `json:"file_path"`
}

// ListConceptsUseCase builds a topic→concept tree from the database.
type ListConceptsUseCase struct {
	topicRepo   domain.TopicRepository
	conceptRepo domain.ConceptRepository
}

// NewListConceptsUseCase creates a new ListConceptsUseCase with the required dependencies.
func NewListConceptsUseCase(topicRepo domain.TopicRepository, conceptRepo domain.ConceptRepository) *ListConceptsUseCase {
	return &ListConceptsUseCase{
		topicRepo:   topicRepo,
		conceptRepo: conceptRepo,
	}
}

// Execute returns all topics with their nested concepts as a tree structure.
func (uc *ListConceptsUseCase) Execute(ctx context.Context) ([]TopicWithConcepts, error) {
	topics, err := uc.topicRepo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}

	result := make([]TopicWithConcepts, 0, len(topics))
	for _, topic := range topics {
		concepts, err := uc.conceptRepo.ListByTopicID(ctx, topic.ID)
		if err != nil {
			return nil, fmt.Errorf("list concepts for topic %q: %w", topic.ID, err)
		}

		var items []ConceptListItem
		if concepts != nil {
			items = make([]ConceptListItem, 0, len(concepts))
		}
		for _, c := range concepts {
			items = append(items, ConceptListItem{
				ID:       c.ID,
				Title:    c.Title,
				FilePath: c.FilePath,
			})
		}

		result = append(result, TopicWithConcepts{
			ID:       topic.ID,
			Name:     topic.Name,
			Concepts: items,
		})
	}

	return result, nil
}
