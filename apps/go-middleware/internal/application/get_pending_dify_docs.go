package application

import (
	"context"
	"fmt"
	"log"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/infrastructure/dify"
)

// PendingDifyDocument represents a Dify document not yet assigned to any local resource.
type PendingDifyDocument struct {
	ID   string
	Name string
}

// GetPendingDifyDocumentsUseCase fetches Dify documents and diffs them against
// locally stored Dify document IDs. Returns only documents not yet linked.
type GetPendingDifyDocumentsUseCase struct {
	difyClient dify.DifyClient
	resourceRepo domain.ResourceRepository
}

// NewGetPendingDifyDocumentsUseCase creates a new use case with the given dependencies.
func NewGetPendingDifyDocumentsUseCase(
	difyClient dify.DifyClient,
	resourceRepo domain.ResourceRepository,
) *GetPendingDifyDocumentsUseCase {
	return &GetPendingDifyDocumentsUseCase{
		difyClient: difyClient,
		resourceRepo: resourceRepo,
	}
}

// Execute returns Dify documents that are not yet assigned to any local resource.
// If the Dify API is unreachable, it logs a warning and returns an empty slice (no error).
// If the repository fails, the error is propagated.
func (uc *GetPendingDifyDocumentsUseCase) Execute(ctx context.Context) ([]PendingDifyDocument, error) {
	difyDocs, err := uc.difyClient.FetchAllDocuments(ctx)
	if err != nil {
		log.Printf("dify fetch failed, returning empty: %v", err)
		return nil, nil
	}

	localIDs, err := uc.resourceRepo.FindAllDifyDocumentIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("find local dify document ids: %w", err)
	}

	localIDSet := make(map[string]bool, len(localIDs))
	for _, id := range localIDs {
		localIDSet[id] = true
	}

	var pending []PendingDifyDocument
	for _, doc := range difyDocs {
		if !localIDSet[doc.ID] {
			pending = append(pending, PendingDifyDocument{
				ID:   doc.ID,
				Name: doc.Name,
			})
		}
	}

	return pending, nil
}
