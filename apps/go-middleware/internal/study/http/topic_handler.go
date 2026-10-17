package http

import (
	"encoding/json"
	"net/http"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
)

// CreateTopicHandler handles POST /api/topics.
type CreateTopicHandler struct {
	useCase CreateTopicUseCase
}

// NewCreateTopicHandler creates a CreateTopicHandler.
func NewCreateTopicHandler(uc CreateTopicUseCase) *CreateTopicHandler {
	return &CreateTopicHandler{useCase: uc}
}

func (h *CreateTopicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req CreateTopicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	topic, err := h.useCase.Execute(r.Context(), req.Name)
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, CreateTopicResponse{
		ID:        topic.ID,
		Name:      topic.Name,
		CreatedAt: instant(topic.CreatedAt),
	})
}
