package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/http"
)

// testUseCases returns the whole study context's application surface over mocks.
//
// A test that exercises one route still builds the complete mux: several of them
// exist to prove the new route shares the mux with the existing ones and changes
// none of them.
func testUseCases() httppkg.UseCases {
	return httppkg.UseCases{
		SyncConcept:    &mockSyncConceptUseCase{id: "concept-1"},
		SyncFlashcards: &mockSyncFlashcardsUseCase{count: 1},
		ListConcepts:   &mockListConceptsUseCase{topics: []app.TopicWithConcepts{}},
		CreateTopic:    &mockCreateTopicUseCase{topic: &domain.Topic{ID: "t1", Name: "DDD", CreatedAt: time.Now().UTC()}},
		CreateConcept:  &mockCreateConceptUseCase{concept: &domain.Concept{ID: "c1", TopicID: "t1", Title: "Aggregates"}},
		SyncResource:   &mockSyncResourceUseCase{resourceID: "r1"},
		ListResources:  &mockListResourcesUseCase{summaries: []app.ResourceSummary{}},
		GetDueCards: &mockGetDueCardsUseCase{cards: []app.DueCard{{
			FlashcardID:  "f1",
			Front:        "What is an aggregate?",
			Back:         "A consistency boundary",
			ConceptTitle: "Aggregates",
			TopicName:    "DDD",
			NextReview:   time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC),
		}}},
		SubmitReview: &mockSubmitReviewUseCase{state: domain.CardState{
			ID: "cs1", FlashcardID: "f1", Stability: 1.1, Difficulty: 0.5,
			NextReview: time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC),
			LastReview: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC),
		}},
		ExplainRange:        &mockExplainRangeUseCase{},
		SetReadingProgress:  &mockSetReadingProgressUseCase{},
		ListReadingProgress: &mockListReadingProgressUseCase{},
		SavePlan:            &mockSavePlanUseCase{},
		GetPlan:             &mockGetPlanUseCase{},
		GetExplanation:      &mockGetExplanationUseCase{},
		SetGapState:         &mockSetGapStateUseCase{},
		AcceptGap:           &mockAcceptGapUseCase{},
		Today:               &mockTodayUseCase{},
	}
}

func newStudyRouter(explain httppkg.ExplainRangeUseCase) http.Handler {
	useCases := testUseCases()
	useCases.ExplainRange = explain
	return httppkg.NewRouter(useCases)
}

// testUseCasesWithExplanation is the whole surface, explanation included, which is what
// a composition root builds.
func testUseCasesWithExplanation() httppkg.UseCases {
	return testUseCases()
}

// A use case the composition root forgot is a boot failure that names the field.
//
// SetGapState is the field this test proves, and it is not an arbitrary one: it is one of
// the two use cases internal/server/server.go left out of its UseCases literal, and a
// nil there used to reach the handler and panic on the first request instead.
func TestNewRouter_PanicsNamingTheUseCaseThatWasNotWired(t *testing.T) {
	useCases := testUseCases()
	useCases.SetGapState = nil

	assert.PanicsWithValue(t,
		"study http: use case SetGapState is not wired",
		func() { httppkg.NewRouter(useCases) },
	)
}

// The two U1 routes answer through the router a complete composition builds, which is
// the evidence that neither handler holds a nil use case.
func TestNewRouter_AnswersTheExplanationReadAndGapStateRoutes(t *testing.T) {
	router := httppkg.NewRouter(testUseCasesWithExplanation())

	read := httptest.NewRecorder()
	router.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/api/study/explanations/x", nil))
	require.Equal(t, http.StatusOK, read.Code, read.Body.String())

	state := httptest.NewRecorder()
	write := httptest.NewRequest(http.MethodPut, "/api/study/gaps/x/state",
		strings.NewReader(`{"state":"discarded"}`))
	write.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(state, write)
	require.Equal(t, http.StatusOK, state.Code, state.Body.String())
}
