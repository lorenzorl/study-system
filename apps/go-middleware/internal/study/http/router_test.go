package http_test

import (
	"net/http"
	"time"

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
		SetReadingProgress:  &mockSetReadingProgressUseCase{},
		ListReadingProgress: &mockListReadingProgressUseCase{},
		SavePlan:            &mockSavePlanUseCase{},
		GetPlan:             &mockGetPlanUseCase{},
		Today:               &mockTodayUseCase{},
	}
}

func newStudyRouter(explain httppkg.ExplainRangeUseCase) http.Handler {
	useCases := testUseCases()
	useCases.ExplainRange = explain
	return httppkg.NewRouter(useCases)
}
