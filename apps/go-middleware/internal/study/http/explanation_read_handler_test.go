package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/http"
)

type mockGetExplanationUseCase struct {
	explanation domain.RangeExplanation
	err         error
	ids         []string
}

func (m *mockGetExplanationUseCase) Execute(
	ctx context.Context,
	explanationID string,
) (domain.RangeExplanation, error) {
	m.ids = append(m.ids, explanationID)
	return m.explanation, m.err
}

type mockSetGapStateUseCase struct {
	gap    domain.RangeGap
	err    error
	inputs []app.SetGapStateInput
}

func (m *mockSetGapStateUseCase) Execute(
	ctx context.Context,
	input app.SetGapStateInput,
) (domain.RangeGap, error) {
	m.inputs = append(m.inputs, input)
	return m.gap, m.err
}

func explanationReadRouter(get *mockGetExplanationUseCase, set *mockSetGapStateUseCase) http.Handler {
	useCases := testUseCases()
	useCases.GetExplanation = get
	useCases.SetGapState = set
	return httppkg.NewRouter(useCases)
}

func storedExplanation() domain.RangeExplanation {
	return domain.RangeExplanation{
		ID:         "e1",
		DocumentID: "doc-1",
		FromPage:   15,
		ToPage:     23,
		Text:       "mis palabras sobre el capítulo",
		CreatedAt:  time.Date(2026, 11, 3, 15, 0, 0, 0, time.UTC),
		Gaps: []domain.RangeGap{
			{ID: "g1", Point: "el prototipo ata modelo e implementación", Page: 18, Quote: "That crude prototype forged the essential link.", State: domain.RangeGapStateOpen},
			{ID: "g2", Point: "el conocimiento se fuga cuando la gente se va", Page: 20, Quote: "all projects leak knowledge", State: domain.RangeGapStateDiscarded},
		},
	}
}

// S2: a stored explanation is readable without paying for the model again, and its
// gaps are addressable.
func TestGetExplanation_ReturnsStoredGapsWithTheirIdentifiers(t *testing.T) {
	get := &mockGetExplanationUseCase{explanation: storedExplanation()}
	rec := httptest.NewRecorder()

	explanationReadRouter(get, &mockSetGapStateUseCase{}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/study/explanations/e1", nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"e1"}, get.ids)

	var response httppkg.ExplainRangeResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	assert.Equal(t, "e1", response.ExplanationID)
	assert.Equal(t, 15, response.FromPage)
	require.Len(t, response.Gaps, 2)
	assert.Equal(t, "g1", response.Gaps[0].GapID)
	assert.Equal(t, "open", response.Gaps[0].State)
	assert.Equal(t, "discarded", response.Gaps[1].State)
	assert.Equal(t, 18, response.Gaps[0].Page)
	assert.Equal(t, "That crude prototype forged the essential link.", response.Gaps[0].Quote)
}

func TestGetExplanation_UnknownIsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()

	explanationReadRouter(&mockGetExplanationUseCase{err: apperror.ErrNotFound}, &mockSetGapStateUseCase{}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/study/explanations/e404", nil))

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.JSONEq(t, `{"error":"not found"}`, rec.Body.String())
}

// S7: discarding a gap answers the gap as stored, evidence included, so the client
// can render it without a second read.
func TestPutGapState_DiscardsAndAnswersTheStoredGap(t *testing.T) {
	set := &mockSetGapStateUseCase{gap: domain.RangeGap{
		ID: "g1", Point: "el prototipo ata modelo e implementación", Page: 18,
		Quote: "That crude prototype forged the essential link.", State: domain.RangeGapStateDiscarded,
	}}
	rec := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPut, "/api/study/gaps/g1/state",
		strings.NewReader(`{"state":"discarded"}`))
	request.Header.Set("Content-Type", "application/json")
	explanationReadRouter(&mockGetExplanationUseCase{}, set).ServeHTTP(rec, request)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var response httppkg.RangeGapResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	assert.Equal(t, "g1", response.GapID)
	assert.Equal(t, "discarded", response.State)
	assert.Equal(t, 18, response.Page)

	require.Len(t, set.inputs, 1)
	assert.Equal(t, "g1", set.inputs[0].GapID)
	assert.Equal(t, "discarded", set.inputs[0].State)
}

// S6 and S7: the refusals name what a client may set. `carded` is refused here on
// purpose: only the accept route can write it, because only it writes the card.
func TestPutGapState_RefusesWhatAClientMayNotSet(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{name: "malformed", body: `{"state":`, message: "invalid request body"},
		{name: "carded", body: `{"state":"carded"}`, message: "state must be discarded or open"},
		{name: "unknown word", body: `{"state":"half"}`, message: "state must be discarded or open"},
		{name: "no state", body: `{}`, message: "state must be discarded or open"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			set := &mockSetGapStateUseCase{}
			rec := httptest.NewRecorder()

			request := httptest.NewRequest(http.MethodPut, "/api/study/gaps/g1/state", strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			explanationReadRouter(&mockGetExplanationUseCase{}, set).ServeHTTP(rec, request)

			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			var failure httpx.ErrorResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&failure))
			assert.Equal(t, testCase.message, failure.Error)
			assert.Empty(t, set.inputs, "a refused state never reaches the use case")
		})
	}
}

func TestPutGapState_UnknownGapIsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPut, "/api/study/gaps/ghost/state",
		strings.NewReader(`{"state":"discarded"}`))
	request.Header.Set("Content-Type", "application/json")
	explanationReadRouter(&mockGetExplanationUseCase{}, &mockSetGapStateUseCase{err: apperror.ErrNotFound}).
		ServeHTTP(rec, request)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.JSONEq(t, `{"error":"not found"}`, rec.Body.String())
}

// S7: a gap with a card keeps its state, and the answer says why instead of the
// envelope's generic "conflict".
func TestPutGapState_AGapWithACardIsAConflict(t *testing.T) {
	set := &mockSetGapStateUseCase{err: &app.GapConflict{Sentence: "this gap already has a card"}}
	rec := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPut, "/api/study/gaps/g1/state",
		strings.NewReader(`{"state":"discarded"}`))
	request.Header.Set("Content-Type", "application/json")
	explanationReadRouter(&mockGetExplanationUseCase{}, set).ServeHTTP(rec, request)

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"error":"this gap already has a card"}`, rec.Body.String())
}
