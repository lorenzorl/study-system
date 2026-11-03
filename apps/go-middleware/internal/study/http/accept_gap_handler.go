package http

// The transport for accepting one gap into a card (S6, S10).

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
)

// AcceptGapHandler handles POST /api/study/gaps/{gapID}/card.
//
// It is the one route that writes `carded`, because it is the one route that writes the
// card. The body carries the reader's question and nothing else: the evidence on the
// card's back comes from the gap the server stored, so a client cannot put words in the
// book's mouth.
type AcceptGapHandler struct {
	useCase AcceptGapUseCase
}

// NewAcceptGapHandler creates an AcceptGapHandler.
func NewAcceptGapHandler(uc AcceptGapUseCase) *AcceptGapHandler {
	return &AcceptGapHandler{useCase: uc}
}

func (h *AcceptGapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req AcceptGapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// The sentence names the field, so a client that sent `text` or nothing learns what
	// this route wants. The use case re-checks it: it is the layer that owns the rule.
	if strings.TrimSpace(req.Question) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "question is required")
		return
	}

	result, err := h.useCase.Execute(r.Context(), app.AcceptGapInput{
		GapID:    r.PathValue("gapID"),
		Question: req.Question,
	})
	if err != nil {
		// The conflict is written here rather than in the shared envelope: every other
		// route's conflict is a generic "conflict", and this one has to say what it is
		// about, because the reader's next move depends on knowing the gap already has
		// a card.
		if errors.Is(err, apperror.ErrConflict) {
			httpx.WriteError(w, http.StatusConflict, "this gap already has a card")
			return
		}

		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, AcceptGapResponse{
		GapID:     result.GapID,
		Question:  result.Question,
		Answer:    result.Answer,
		ConceptID: result.ConceptID,
		TopicID:   result.TopicID,
		State:     string(result.State),
	})
}
