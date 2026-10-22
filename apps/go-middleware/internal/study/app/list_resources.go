package app

import (
	"context"
	"fmt"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// ResourceSummary is one row of the resource list the launcher renders.
//
// It is a projection, not the domain entity: the topic name arrives resolved by
// the repository and the timestamp travels as a value the transport formats, so
// the read path never needs a second lookup per row.
type ResourceSummary struct {
	ID        string
	Title     string
	Type      domain.ResourceType
	SourceURI string
	TopicName string
	CreatedAt time.Time
}

// ListResourcesUseCase reads every stored study resource for the launcher.
type ListResourcesUseCase struct {
	resourceRepo domain.ResourceRepository
}

// NewListResourcesUseCase creates a new ListResourcesUseCase with the required dependencies.
func NewListResourcesUseCase(resourceRepo domain.ResourceRepository) *ListResourcesUseCase {
	return &ListResourcesUseCase{resourceRepo: resourceRepo}
}

// Execute returns every resource with its topic name, in the repository's order.
//
// An empty corpus is an empty slice and no error: a launcher with nothing to show
// is a normal state, and the transport turns the empty slice into `[]`.
func (uc *ListResourcesUseCase) Execute(ctx context.Context) ([]ResourceSummary, error) {
	resources, err := uc.resourceRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list resources: %w", err)
	}

	summaries := make([]ResourceSummary, 0, len(resources))
	for _, resource := range resources {
		summaries = append(summaries, ResourceSummary{
			ID:        resource.ID,
			Title:     resource.Title,
			Type:      resource.Type,
			SourceURI: resource.SourceURI,
			TopicName: resource.TopicName,
			CreatedAt: resource.CreatedAt,
		})
	}

	return summaries, nil
}
