package http

import (
	"net/http"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
)

// DocumentsHandler handles GET /api/rag/documents.
type DocumentsHandler struct {
	useCase DocumentListUseCase
}

// NewDocumentsHandler creates a DocumentsHandler.
func NewDocumentsHandler(uc DocumentListUseCase) *DocumentsHandler {
	return &DocumentsHandler{useCase: uc}
}

func (h *DocumentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.useCase.Execute(r.Context())
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	// make with len guarantees an empty corpus encodes as [] and never as null:
	// the consumer ranges over this array, and "the corpus is empty" must not be a
	// case it has to special-case.
	response := make([]DocumentSummaryResponse, 0, len(summaries))
	for _, summary := range summaries {
		response = append(response, DocumentSummaryResponse{
			ID:        summary.ID,
			Title:     summary.Title,
			CreatedAt: summary.CreatedAt.UTC().Format(time.RFC3339),
			Pages:     summary.Pages,
			Chunks:    summary.Chunks,
		})
	}

	httpx.WriteJSON(w, http.StatusOK, response)
}
