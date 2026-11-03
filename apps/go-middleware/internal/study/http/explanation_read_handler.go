package http

// The transport for a stored explanation: read it back, or discard one gap (S2, S7).

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
)

// ExplanationReadHandler handles GET /api/study/explanations/{explanationID}.
//
// It reads what a run stored and never calls the model: reopening an explanation is
// re-reading the reader's own words and the gaps it left, and a reload that asked the
// model again would cost money, answer differently, and forget the gaps already
// discarded.
type ExplanationReadHandler struct {
	useCase GetExplanationUseCase
}

// NewExplanationReadHandler creates an ExplanationReadHandler.
func NewExplanationReadHandler(uc GetExplanationUseCase) *ExplanationReadHandler {
	return &ExplanationReadHandler{useCase: uc}
}

func (h *ExplanationReadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	explanation, err := h.useCase.Execute(r.Context(), r.PathValue("explanationID"))
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, explanationResponse(explanation))
}

// GapStateHandler handles PUT /api/study/gaps/{gapID}/state.
//
// Discarding is the only thing a client decides here. Accepting a gap into a card is
// a different route, because that one writes the card too, and a state that says
// "carded" without a card is a lie the gap list would repeat.
type GapStateHandler struct {
	useCase SetGapStateUseCase
}

// NewGapStateHandler creates a GapStateHandler.
func NewGapStateHandler(uc SetGapStateUseCase) *GapStateHandler {
	return &GapStateHandler{useCase: uc}
}

func (h *GapStateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req SetGapStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// The sentence names the two states a client may set, so a client that tried
	// `carded` learns what to call instead rather than reading "invalid input".
	if !clientSettableGapState(req.State) {
		httpx.WriteError(w, http.StatusBadRequest, "state must be discarded or open")
		return
	}

	gap, err := h.useCase.Execute(r.Context(), app.SetGapStateInput{
		GapID: r.PathValue("gapID"),
		State: req.State,
	})
	if err != nil {
		// A gap with a card keeps its state, and the sentence says why: the generic
		// envelope would answer "conflict", which tells the reader nothing to do.
		var conflict *app.GapConflict
		if errors.As(err, &conflict) {
			httpx.WriteError(w, http.StatusConflict, conflict.Error())
			return
		}

		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, rangeGapResponse(gap))
}

func clientSettableGapState(state string) bool {
	return state == "discarded" || state == "open"
}
