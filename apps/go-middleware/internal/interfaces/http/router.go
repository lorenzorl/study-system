package http

import "net/http"

// corsMiddleware adds permissive CORS headers for local-first development.
// In production behind Obsidian's webview, these headers are harmless.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// NewRouter creates an http.Handler with all API routes registered.
// It accepts the five use case implementations which are wired in main.go.
func NewRouter(
	syncConcept SyncConceptUseCase,
	syncFlashcards SyncFlashcardsUseCase,
	listConcepts ListConceptsUseCase,
	createTopic CreateTopicUseCase,
	createConcept CreateConceptUseCase,
) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /api/sync/concept", NewSyncConceptHandler(syncConcept))
	mux.Handle("POST /api/sync/flashcards", NewSyncFlashcardsHandler(syncFlashcards))
	mux.Handle("GET /api/concepts", NewListConceptsHandler(listConcepts))
	mux.Handle("POST /api/topics", NewCreateTopicHandler(createTopic))
	mux.Handle("POST /api/concepts", NewCreateConceptHandler(createConcept))

	return corsMiddleware(mux)
}
