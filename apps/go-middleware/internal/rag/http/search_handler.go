package http

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
)

// SearchHandler handles GET /api/rag/search.
type SearchHandler struct {
	useCase SearchUseCase
}

// NewSearchHandler creates a SearchHandler.
func NewSearchHandler(uc SearchUseCase) *SearchHandler {
	return &SearchHandler{useCase: uc}
}

func (h *SearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		httpx.WriteError(w, http.StatusBadRequest, "q is required")
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	results, err := h.useCase.Search(r.Context(), query, limit)
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	// make with len guarantees an empty result encodes as [] and never as null:
	// the consumer ranges over this array, and "no matches" must not be a case it
	// has to special-case.
	response := make([]SearchResultResponse, 0, len(results))
	for _, result := range results {
		response = append(response, SearchResultResponse{
			ChunkID:       result.ChunkID,
			DocumentID:    result.DocumentID,
			DocumentTitle: result.DocumentTitle,
			PageNumber:    result.PageNumber,
			Snippet:       result.Snippet,
			Stage:         string(result.Stage),
			// bm25 is smaller-is-better, so the magnitude is what a reader can
			// compare: bigger is better, the same direction the command prints.
			BM25:       math.Abs(result.Score),
			Similarity: result.Similarity,
		})
	}

	httpx.WriteJSON(w, http.StatusOK, response)
}

// parseLimit resolves the optional limit parameter.
//
// Absent or empty means the application default. Anything else must be an
// integer in [1, MaxSearchLimit]; a non-number, a value below 1, and a value
// above the maximum are the same 400, and the message names the bound so the
// caller knows what to fix.
func parseLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return app.DefaultSearchLimit, nil
	}

	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > MaxSearchLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", MaxSearchLimit)
	}
	return limit, nil
}
