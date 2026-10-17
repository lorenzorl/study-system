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
