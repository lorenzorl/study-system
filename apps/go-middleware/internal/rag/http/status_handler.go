package http

import (
	"net/http"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
)

// DocumentStatusHandler handles GET /api/rag/documents/{documentID}/status.
type DocumentStatusHandler struct {
	useCase DocumentStatusUseCase
}

// NewDocumentStatusHandler creates a DocumentStatusHandler.
func NewDocumentStatusHandler(uc DocumentStatusUseCase) *DocumentStatusHandler {
	return &DocumentStatusHandler{useCase: uc}
}

func (h *DocumentStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	status, err := h.useCase.Execute(r.Context(), r.PathValue("documentID"))
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, documentStatusResponse(status))
}

// documentStatusResponse maps the use case's status to its wire shape. The
// timestamps are RFC3339 strings and stay empty when absent, so omitempty drops
// them rather than emitting a zero instant.
func documentStatusResponse(status app.DocumentStatus) DocumentStatusResponse {
	response := DocumentStatusResponse{
		DocumentID: status.DocumentID,
		Title:      status.Title,
		State:      string(status.State),
		Pages: PageProgressResponse{
			Total:       status.Pages.Total,
			Extracted:   status.Pages.Extracted,
			Transcribed: status.Pages.Transcribed,
			Failed:      status.Pages.Failed,
			Pending:     status.Pages.Pending,
		},
		Chunks:   status.Chunks,
		Embedded: status.Embedded,
		Error:    status.Error,
	}

	if status.StartedAt != nil {
		response.StartedAt = status.StartedAt.UTC().Format(time.RFC3339)
	}
	if status.UpdatedAt != nil {
		response.UpdatedAt = status.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if status.FinishedAt != nil {
		response.FinishedAt = status.FinishedAt.UTC().Format(time.RFC3339)
	}

	return response
}
