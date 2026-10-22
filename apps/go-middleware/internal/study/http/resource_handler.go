package http

import (
	"encoding/json"
	"net/http"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
)

// SyncResourceHandler handles POST /api/sync/resource.
type SyncResourceHandler struct {
	useCase SyncResourceUseCase
}

// NewSyncResourceHandler creates a SyncResourceHandler.
func NewSyncResourceHandler(uc SyncResourceUseCase) *SyncResourceHandler {
	return &SyncResourceHandler{useCase: uc}
}

func (h *SyncResourceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req SyncResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TopicName == "" || req.ResourceTitle == "" || req.Type == "" || req.SourceURI == "" {
		httpx.WriteError(w, http.StatusBadRequest, "topic_name, resource_title, type, and source_uri are required")
		return
	}

	resourceID, err := h.useCase.Execute(r.Context(), req.TopicName, req.ResourceTitle, req.Type, req.SourceURI)
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, SyncResourceResponse{ResourceID: resourceID})
}

// ListResourcesHandler handles GET /api/resources.
type ListResourcesHandler struct {
	useCase ListResourcesUseCase
}

// NewListResourcesHandler creates a ListResourcesHandler.
func NewListResourcesHandler(uc ListResourcesUseCase) *ListResourcesHandler {
	return &ListResourcesHandler{useCase: uc}
}

func (h *ListResourcesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.useCase.Execute(r.Context())
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	// make with len guarantees an empty corpus encodes as [] and never as null:
	// the launcher ranges over this array, and "no resources yet" must not be a
	// case it has to special-case.
	response := make([]ResourceSummaryResponse, 0, len(summaries))
	for _, summary := range summaries {
		response = append(response, ResourceSummaryResponse{
			ID:        summary.ID,
			Title:     summary.Title,
			Type:      string(summary.Type),
			SourceURI: summary.SourceURI,
			TopicName: summary.TopicName,
			CreatedAt: instant(summary.CreatedAt),
		})
	}

	httpx.WriteJSON(w, http.StatusOK, response)
}
