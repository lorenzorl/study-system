package http

import (
	"encoding/json"
	"net/http"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
)

// ReviewHandler handles POST /api/study/review.
type ReviewHandler struct {
	useCase SubmitReviewUseCase
}

// NewReviewHandler creates a ReviewHandler.
func NewReviewHandler(uc SubmitReviewUseCase) *ReviewHandler {
	return &ReviewHandler{useCase: uc}
}

func (h *ReviewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req ReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.FlashcardID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "flashcard_id is required")
		return
	}

	if req.Grade < 1 || req.Grade > 4 {
		httpx.WriteError(w, http.StatusBadRequest, "grade must be between 1 and 4")
		return
	}

	newState, err := h.useCase.Execute(r.Context(), req.FlashcardID, req.Grade, req.DurationMs)
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, ReviewResponse{
		NextReview: instant(newState.NextReview),
		Stability:  newState.Stability,
		Difficulty: newState.Difficulty,
	})
}
