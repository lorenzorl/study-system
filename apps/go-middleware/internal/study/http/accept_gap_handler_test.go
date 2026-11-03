package http_test

// POST /api/study/gaps/{gapID}/card: the reader writes the question, the server
// writes the card and claims the gap (S6, S10).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/http"
)

type mockAcceptGapUseCase struct {
	result app.AcceptGapResult
	err    error
	inputs []app.AcceptGapInput
}

func (m *mockAcceptGapUseCase) Execute(
	ctx context.Context,
	input app.AcceptGapInput,
) (app.AcceptGapResult, error) {
	m.inputs = append(m.inputs, input)
	return m.result, m.err
}

func acceptGapRouter(accept *mockAcceptGapUseCase) http.Handler {
	useCases := testUseCasesWithExplanation()
	useCases.AcceptGap = accept
	return httppkg.NewRouter(useCases)
}

func acceptedGap() app.AcceptGapResult {
	return app.AcceptGapResult{
		GapID:     "g1",
		Question:  "¿Por qué el prototipo ata modelo e implementación?",
		Answer:    "That crude prototype forged the essential link. (p.18)",
		ConceptID: "concept-1",
		TopicID:   "topic-1",
		State:     domain.RangeGapStateCarded,
	}
}

// S6: one request turns an accepted gap into a card, and the answer names what was
// written so the page can show it without a second read.
func TestPostGapCard_AnswersTheCardItWrote(t *testing.T) {
	accept := &mockAcceptGapUseCase{result: acceptedGap()}
	rec := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPost, "/api/study/gaps/g1/card",
		strings.NewReader(`{"question":"¿Por qué el prototipo ata modelo e implementación?"}`))
	request.Header.Set("Content-Type", "application/json")
	acceptGapRouter(accept).ServeHTTP(rec, request)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var response httppkg.AcceptGapResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	assert.Equal(t, "g1", response.GapID)
	assert.Equal(t, "¿Por qué el prototipo ata modelo e implementación?", response.Question)
	assert.Equal(t, "That crude prototype forged the essential link. (p.18)", response.Answer)
	assert.Equal(t, "concept-1", response.ConceptID)
	assert.Equal(t, "topic-1", response.TopicID)
	assert.Equal(t, "carded", response.State)

	require.Len(t, accept.inputs, 1)
	assert.Equal(t, "g1", accept.inputs[0].GapID, "the gap comes from the path")
	assert.Equal(t, "¿Por qué el prototipo ata modelo e implementación?", accept.inputs[0].Question)
}

// S6: every refusal names what was missing, and none of them reaches the use case —
// so none of them writes a topic, a concept or a card.
func TestPostGapCard_RefusesABodyThatCannotBecomeACard(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{name: "not JSON", body: `{"question":`, message: "invalid request body"},
		{name: "missing", body: `{}`, message: "question is required"},
		{name: "blank", body: `{"question":"   "}`, message: "question is required"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			accept := &mockAcceptGapUseCase{result: acceptedGap()}
			rec := httptest.NewRecorder()

			request := httptest.NewRequest(http.MethodPost, "/api/study/gaps/g1/card",
				strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			acceptGapRouter(accept).ServeHTTP(rec, request)

			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

			var failure httpx.ErrorResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&failure))
			assert.Equal(t, testCase.message, failure.Error)
			assert.Empty(t, accept.inputs, "a refused body never reaches the use case")
		})
	}
}

func TestPostGapCard_UnknownGapIsNotFound(t *testing.T) {
	accept := &mockAcceptGapUseCase{err: apperror.ErrNotFound}
	rec := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPost, "/api/study/gaps/ghost/card",
		strings.NewReader(`{"question":"¿Qué es un agregado?"}`))
	request.Header.Set("Content-Type", "application/json")
	acceptGapRouter(accept).ServeHTTP(rec, request)

	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"error":"not found"}`, rec.Body.String())
}

// S6: the 409 is written here, from the conflict sentinel, so a gap that already has
// a card says so instead of the shared envelope's generic "conflict".
func TestPostGapCard_AlreadyCardedGapIsAConflict(t *testing.T) {
	accept := &mockAcceptGapUseCase{err: apperror.ErrConflict}
	rec := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPost, "/api/study/gaps/g1/card",
		strings.NewReader(`{"question":"¿Qué es un agregado?"}`))
	request.Header.Set("Content-Type", "application/json")
	acceptGapRouter(accept).ServeHTTP(rec, request)

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"error":"this gap already has a card"}`, rec.Body.String())
}
