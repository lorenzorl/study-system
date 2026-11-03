package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
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

// mockExplainRangeUseCase records the input it was given, so a refusal at the
// transport can prove the use case was never reached.
type mockExplainRangeUseCase struct {
	explanation domain.RangeExplanation
	err         error
	inputs      []app.ExplainRangeInput
}

func (m *mockExplainRangeUseCase) Execute(
	ctx context.Context,
	input app.ExplainRangeInput,
) (domain.RangeExplanation, error) {
	m.inputs = append(m.inputs, input)
	return m.explanation, m.err
}

func explanationRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/study/explanation", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

// S1 and S8: the route exists under the study context and answers the shape the
// contract documents, verbatim.
func TestRouter_PostExplanation_Created(t *testing.T) {
	tests := []struct {
		name     string
		explain  *mockExplainRangeUseCase
		wantGaps int
	}{
		{
			name: "with gaps",
			explain: &mockExplainRangeUseCase{explanation: domain.RangeExplanation{
				ID:         "e1",
				DocumentID: "c6d1e9a3-0000-4000-8000-000000000001",
				FromPage:   88,
				ToPage:     114,
				Text:       "an explanation long enough",
				CreatedAt:  domain.Now(),
				Gaps: []domain.RangeGap{{
					ID:    "g1",
					Point: "the boundary is defined by the invariants that must hold in one transaction",
					Page:  89,
					Quote: "Cluster the ENTITIES and VALUE OBJECTS into AGGREGATES",
					State: domain.RangeGapStateOpen,
				}, {
					ID:    "g2",
					Point: "a repository is per aggregate root",
					Page:  108,
					Quote: "only AGGREGATE roots may be obtained from the database directly",
					State: domain.RangeGapStateOpen,
				}},
			}},
			wantGaps: 2,
		},
		{
			name: "leaving nothing out",
			explain: &mockExplainRangeUseCase{explanation: domain.RangeExplanation{
				ID:         "e2",
				DocumentID: "c6d1e9a3-0000-4000-8000-000000000001",
				FromPage:   88,
				ToPage:     114,
				Text:       "an explanation long enough",
				CreatedAt:  domain.Now(),
			}},
			wantGaps: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()

			newStudyRouter(test.explain).ServeHTTP(rec, explanationRequest(
				`{"document_id":"c6d1e9a3-0000-4000-8000-000000000001","from_page":88,"to_page":114,"text":"an explanation long enough"}`))

			require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())

			var response map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))

			keys := make([]string, 0, len(response))
			for key := range response {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			assert.Equal(t, []string{"document_id", "explanation_id", "from_page", "gaps", "to_page"}, keys,
				"the response carries exactly the fields the contract names")

			var decoded struct {
				ExplanationID string `json:"explanation_id"`
				DocumentID    string `json:"document_id"`
				FromPage      int    `json:"from_page"`
				ToPage        int    `json:"to_page"`
				Gaps          []struct {
					Point string `json:"point"`
					Page  int    `json:"page"`
					Quote string `json:"quote"`
				} `json:"gaps"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &decoded))
			assert.Equal(t, test.explain.explanation.ID, decoded.ExplanationID)
			assert.Equal(t, "c6d1e9a3-0000-4000-8000-000000000001", decoded.DocumentID)
			assert.Equal(t, 88, decoded.FromPage)
			assert.Equal(t, 114, decoded.ToPage)

			require.NotNil(t, decoded.Gaps, "a response always carries a gaps list")
			require.Len(t, decoded.Gaps, test.wantGaps)
			if test.wantGaps > 0 {
				assert.Equal(t, "the boundary is defined by the invariants that must hold in one transaction", decoded.Gaps[0].Point)
				assert.Equal(t, 89, decoded.Gaps[0].Page)
				assert.Equal(t, "Cluster the ENTITIES and VALUE OBJECTS into AGGREGATES", decoded.Gaps[0].Quote)
			}

			// The transport hands the use case the request's own values, so nothing
			// is reshaped between the wire and the application.
			require.Len(t, test.explain.inputs, 1)
			assert.Equal(t, app.ExplainRangeInput{
				DocumentID: "c6d1e9a3-0000-4000-8000-000000000001",
				FromPage:   88,
				ToPage:     114,
				Text:       "an explanation long enough",
			}, test.explain.inputs[0])
		})
	}
}

// The refusal table of the contract, end to end through the mux.
func TestRouter_PostExplanation_Refusals(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
		wantError  string
	}{
		{
			name:       "text missing",
			body:       `{"document_id":"d1","from_page":88,"to_page":114}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "text is required",
		},
		{
			name:       "text blank",
			body:       `{"document_id":"d1","from_page":88,"to_page":114,"text":"   \n  "}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "text is required",
		},
		{
			name: "text too short for the range",
			body: `{"document_id":"d1","from_page":88,"to_page":114,"text":"ok."}`,
			err: app.NewExplanationRefusal(
				"this range is 27 pages and your explanation covers 3 characters: explain one section, or say more"),
			wantStatus: http.StatusBadRequest,
			wantError:  "this range is 27 pages and your explanation covers 3 characters: explain one section, or say more",
		},
		{
			name:       "range outside the pages contract",
			body:       `{"document_id":"d1","from_page":88,"to_page":140,"text":"a long enough explanation"}`,
			err:        fmt.Errorf("read pages 88 to 140: %w", apperror.ErrInvalidInput),
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid input",
		},
		{
			name:       "unknown document",
			body:       `{"document_id":"missing","from_page":88,"to_page":114,"text":"a long enough explanation"}`,
			err:        fmt.Errorf("find document: %w", apperror.ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantError:  "not found",
		},
		{
			name:       "the model fails",
			body:       `{"document_id":"d1","from_page":88,"to_page":114,"text":"a long enough explanation"}`,
			err:        errors.New("openrouter returned 502: upstream timeout"),
			wantStatus: http.StatusInternalServerError,
			wantError:  "internal server error",
		},
		{
			name:       "invalid JSON",
			body:       `{broken`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request body",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			explain := &mockExplainRangeUseCase{err: test.err}
			rec := httptest.NewRecorder()

			newStudyRouter(explain).ServeHTTP(rec, explanationRequest(test.body))

			assert.Equal(t, test.wantStatus, rec.Code)

			var errResp httpx.ErrorResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
			assert.Equal(t, test.wantError, errResp.Error)

			// A model failure is the server's, and its text never reaches the
			// reader: the envelope is the generic one.
			assert.NotContains(t, rec.Body.String(), "openrouter")
			assert.NotContains(t, rec.Body.String(), "upstream timeout")

			switch test.name {
			case "text missing", "text blank", "invalid JSON":
				assert.Empty(t, explain.inputs, "a malformed request must never reach the use case")
			}
		})
	}
}

// The study router is shared code: the two routes that were already on it must
// answer exactly as before.
func TestRouter_ReviewRouteStillBehaves(t *testing.T) {
	rec := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodPost, "/api/study/review",
		strings.NewReader(`{"flashcard_id":"f1","grade":3,"duration_ms":5000}`))
	request.Header.Set("Content-Type", "application/json")

	newStudyRouter(&mockExplainRangeUseCase{}).ServeHTTP(rec, request)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var response httppkg.ReviewResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	assert.Equal(t, "2026-11-05T10:00:00Z", response.NextReview)
	assert.Equal(t, 1.1, response.Stability)
	assert.Equal(t, 0.5, response.Difficulty)
}

func TestRouter_DueRouteStillBehaves(t *testing.T) {
	rec := httptest.NewRecorder()

	newStudyRouter(&mockExplainRangeUseCase{}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/study/due", nil))

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var response []httppkg.DueCardResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	require.Len(t, response, 1)
	assert.Equal(t, "f1", response[0].ID)
	assert.Equal(t, "What is an aggregate?", response[0].Question)
	assert.Equal(t, "A consistency boundary", response[0].Answer)
	assert.Equal(t, "Aggregates", response[0].ConceptTitle)
	assert.Equal(t, "DDD", response[0].TopicName)
	assert.Equal(t, "2026-11-02T10:00:00Z", response[0].NextReview)
}
