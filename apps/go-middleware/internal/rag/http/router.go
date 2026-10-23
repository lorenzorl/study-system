package http

import "net/http"

// NewRouter builds the RAG context's HTTP handler.
//
// It registers the retrieval route, the two document-creation routes, the
// corpus-list route, and the processing-status route, using the same stdlib
// method+path pattern the study context uses. Cross-cutting transport concerns
// such as CORS are deliberately NOT applied here: the composition root wraps this
// handler, so a context never encodes platform policy.
//
// The plugin is the intended consumer; there is no UI.
func NewRouter(
	search SearchUseCase,
	upload *UploadDocumentHandler,
	status *DocumentStatusHandler,
	documents *DocumentsHandler,
	text *IngestTextHandler,
) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /api/rag/search", NewSearchHandler(search))
	// The collection and its members share the path prefix: GET lists the corpus
	// and POST uploads, and the status route names its member. The text route is a
	// second creation path on the same collection, so a client swaps the source of
	// the material by changing one segment and nothing else.
	mux.Handle("GET /api/rag/documents", documents)
	mux.Handle("POST /api/rag/documents", upload)
	mux.Handle("POST /api/rag/documents/text", text)
	mux.Handle("GET /api/rag/documents/{documentID}/status", status)

	return mux
}
