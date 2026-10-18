package http

import "net/http"

// NewRouter builds the RAG context's HTTP handler.
//
// It registers the retrieval route, the upload route, and the processing-status
// route, using the same stdlib method+path pattern the study context uses.
// Cross-cutting transport concerns such as CORS are deliberately NOT applied
// here: the composition root wraps this handler, so a context never encodes
// platform policy.
//
// The plugin is the intended consumer; there is no UI.
func NewRouter(search SearchUseCase, upload *UploadDocumentHandler, status *DocumentStatusHandler) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /api/rag/search", NewSearchHandler(search))
	mux.Handle("POST /api/rag/documents", upload)
	mux.Handle("GET /api/rag/documents/{documentID}/status", status)

	return mux
}
