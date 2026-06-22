package http

import "net/http"

// NewRouter creates an http.Handler with all API routes registered.
// It accepts the three use case implementations which are wired in main.go.
func NewRouter(
	syncConcept SyncConceptUseCase,
	syncFlashcards SyncFlashcardsUseCase,
	listConcepts ListConceptsUseCase,
) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /api/sync/concept", NewSyncConceptHandler(syncConcept))
	mux.Handle("POST /api/sync/flashcards", NewSyncFlashcardsHandler(syncFlashcards))
	mux.Handle("GET /api/concepts", NewListConceptsHandler(listConcepts))

	return mux
}
