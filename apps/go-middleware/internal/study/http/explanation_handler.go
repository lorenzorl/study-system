package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// ExplanationHandler handles POST /api/study/explanation.
//
// It is the endpoint the reader's own account arrives at: the range, their words,
// and the gaps their words left out. Nothing here decides what a gap is — that is
// the application layer's one validation — and nothing here writes a card: an
// accepted gap becomes one through the existing flashcards endpoint.
type ExplanationHandler struct {
	useCase ExplainRangeUseCase
}

// NewExplanationHandler creates an ExplanationHandler.
func NewExplanationHandler(uc ExplainRangeUseCase) *ExplanationHandler {
	return &ExplanationHandler{useCase: uc}
}

func (h *ExplanationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req ExplainRangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Blank is "missing" to the reader: an explanation of spaces is not one, and
	// the contract's first refusal names the missing field rather than a length.
	if strings.TrimSpace(req.Text) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "text is required")
		return
	}

	explanation, err := h.useCase.Execute(r.Context(), app.ExplainRangeInput{
		DocumentID: req.DocumentID,
		FromPage:   req.FromPage,
		ToPage:     req.ToPage,
		Text:       req.Text,
	})
	if err != nil {
		// A refusal reads as a sentence naming what to do instead, which the
		// generic mapping would flatten into "invalid input". Everything else is
		// classified by sentinel: a refused range or an unknown document becomes
		// 400 or 404, and a model failure stays a 500 with nothing stored.
		var refusal *app.ExplanationRefusal
		if errors.As(err, &refusal) {
			httpx.WriteError(w, http.StatusBadRequest, refusal.Error())
			return
		}

		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, explanationResponse(explanation))
}

// explanationResponse maps the stored explanation to its wire shape.
//
// The gap list is built with its length, so an explanation that left nothing out
// encodes as [] and never as null: the client ranges over it.
func explanationResponse(explanation domain.RangeExplanation) ExplainRangeResponse {
	response := ExplainRangeResponse{
		ExplanationID: explanation.ID,
		DocumentID:    explanation.DocumentID,
		FromPage:      explanation.FromPage,
		ToPage:        explanation.ToPage,
		Gaps:          make([]RangeGapResponse, 0, len(explanation.Gaps)),
	}

	for _, gap := range explanation.Gaps {
		response.Gaps = append(response.Gaps, RangeGapResponse{
			Point: gap.Point,
			Page:  gap.Page,
			Quote: gap.Quote,
		})
	}

	return response
}
