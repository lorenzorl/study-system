package http

import (
	"net/http"
)

// NewRouter builds the study context's HTTP handler.
//
// It accepts the use case implementations, which cmd/server wires together. The
// context owns its own routes; a future top-level composition point can mount
// this alongside other contexts.
//
// Cross-cutting transport concerns such as CORS are deliberately NOT applied
// here. The composition root wraps this handler, so a context never encodes
// platform policy.
func NewRouter(
	syncConcept SyncConceptUseCase,
	syncFlashcards SyncFlashcardsUseCase,
	listConcepts ListConceptsUseCase,
	createTopic CreateTopicUseCase,
	createConcept CreateConceptUseCase,
	syncResource SyncResourceUseCase,
	getDueCards GetDueCardsUseCase,
	submitReview SubmitReviewUseCase,
) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /api/sync/concept", NewSyncConceptHandler(syncConcept))
	mux.Handle("POST /api/sync/flashcards", NewSyncFlashcardsHandler(syncFlashcards))
	mux.Handle("GET /api/concepts", NewListConceptsHandler(listConcepts))
	mux.Handle("POST /api/topics", NewCreateTopicHandler(createTopic))
	mux.Handle("POST /api/concepts", NewCreateConceptHandler(createConcept))
	mux.Handle("POST /api/sync/resource", NewSyncResourceHandler(syncResource))
	mux.Handle("GET /api/study/due", NewDueCardsHandler(getDueCards))
	mux.Handle("POST /api/study/review", NewReviewHandler(submitReview))

	return mux
}
