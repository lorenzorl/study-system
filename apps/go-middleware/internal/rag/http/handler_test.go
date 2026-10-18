package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/http"
)

// mockSearchUseCase is a hand-written SearchUseCase. It records the last call so
// a test can assert what the handler forwarded, not only what it answered. The
// tests are hermetic: no network, no API key, no database.
type mockSearchUseCase struct {
	results []domain.SearchResult
	err     error

	gotQuery string
	gotLimit int
	calls    int
}

func (m *mockSearchUseCase) Search(ctx context.Context, query string, limit int) ([]domain.SearchResult, error) {
	m.calls++
	m.gotQuery = query
	m.gotLimit = limit
	return m.results, m.err
}

// keysOf returns the JSON object's keys, to pin the wire format independently of
// the struct tags that produced it.
func keysOf(object map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	return keys
}

func TestSearchHandler_Success(t *testing.T) {
	mock := &mockSearchUseCase{results: []domain.SearchResult{{
		ChunkID:       "chunk-1",
		DocumentID:    "doc-1",
		DocumentTitle: "Attention Is All You Need",
		PageNumber:    3,
		Snippet:       "the [attention] mechanism",
		Text:          "the attention mechanism, in full, for the model to read",
		Stage:         domain.StageHybrid,
		Score:         -1.25,
		Similarity:    0.75,
	}}}
	handler := httppkg.NewSearchHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/search?q=attention&limit=5", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	// Assert the exact wire keys, not only the decoded struct: a wrong json tag
	// would still round-trip through the same struct.
	var raw []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	require.Len(t, raw, 1)
	assert.ElementsMatch(t, []string{
		"chunk_id", "document_id", "document_title", "page_number",
		"snippet", "text", "stage", "bm25", "similarity",
	}, keysOf(raw[0]))

	var resp []httppkg.SearchResultResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "chunk-1", resp[0].ChunkID)
	assert.Equal(t, "the attention mechanism, in full, for the model to read", resp[0].Text,
		"the wire carries the chunk text next to the snippet")
	assert.Equal(t, "doc-1", resp[0].DocumentID)
	assert.Equal(t, "Attention Is All You Need", resp[0].DocumentTitle)
	assert.Equal(t, 3, resp[0].PageNumber)
	assert.Equal(t, "the [attention] mechanism", resp[0].Snippet)
	assert.Equal(t, "hybrid", resp[0].Stage)
	// bm25 is reported as the magnitude: the index's -1.25 becomes 1.25, so a
	// reader never has to invert it, and the direction matches the command.
	assert.Equal(t, 1.25, resp[0].BM25)
	assert.Equal(t, 0.75, resp[0].Similarity)

	assert.Equal(t, "attention", mock.gotQuery)
	assert.Equal(t, 5, mock.gotLimit)
}

func TestSearchHandler_EmptyEncodesAsArray(t *testing.T) {
	mock := &mockSearchUseCase{results: []domain.SearchResult{}}
	handler := httppkg.NewSearchHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/search?q=nothing", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// A nil slice would encode as null; the consumer ranges over this, so it must
	// be a bare array.
	assert.Equal(t, "[]", strings.TrimSpace(rec.Body.String()))

	var resp []httppkg.SearchResultResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp)
}

func TestSearchHandler_MissingQ(t *testing.T) {
	mock := &mockSearchUseCase{}
	handler := httppkg.NewSearchHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/search", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 0, mock.calls)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "q is required", errResp.Error)
}

func TestSearchHandler_WhitespaceOnlyQ(t *testing.T) {
	mock := &mockSearchUseCase{}
	handler := httppkg.NewSearchHandler(mock)

	// "+" decodes to a space, so this q is whitespace-only.
	req := httptest.NewRequest(http.MethodGet, "/api/rag/search?q=+++", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 0, mock.calls)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "q is required", errResp.Error)
}

func TestSearchHandler_PunctuationOnlyQ(t *testing.T) {
	mock := &mockSearchUseCase{results: []domain.SearchResult{}}
	handler := httppkg.NewSearchHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/search?q=...", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Punctuation is not an error: the use case returns no match and the handler
	// answers 200 with an empty array, exactly as the CLI exits 0 with 0 results.
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "[]", strings.TrimSpace(rec.Body.String()))
	assert.Equal(t, "...", mock.gotQuery)
}

func TestSearchHandler_InvalidLimit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit string
	}{
		{"not a number", "abc"},
		{"zero", "0"},
		{"negative", "-3"},
		{"above the maximum", "51"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockSearchUseCase{}
			handler := httppkg.NewSearchHandler(mock)

			req := httptest.NewRequest(http.MethodGet, "/api/rag/search?q=x&limit="+tc.limit, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, 0, mock.calls)

			var errResp httpx.ErrorResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
			assert.Contains(t, errResp.Error, "limit")
			// The message names the bound so the caller knows what to change.
			assert.Contains(t, errResp.Error, "50")
		})
	}
}

func TestSearchHandler_AcceptsLimitBounds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
	}{
		{"the minimum", 1},
		{"the maximum", 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockSearchUseCase{results: []domain.SearchResult{}}
			handler := httppkg.NewSearchHandler(mock)

			req := httptest.NewRequest(http.MethodGet,
				"/api/rag/search?q=x&limit="+strconv.Itoa(tc.limit), nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tc.limit, mock.gotLimit)
		})
	}
}

func TestSearchHandler_DefaultLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target string
	}{
		{"absent", "/api/rag/search?q=x"},
		{"empty", "/api/rag/search?q=x&limit="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockSearchUseCase{results: []domain.SearchResult{}}
			handler := httppkg.NewSearchHandler(mock)

			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, app.DefaultSearchLimit, mock.gotLimit)
		})
	}
}

func TestSearchHandler_TrimsQuery(t *testing.T) {
	mock := &mockSearchUseCase{results: []domain.SearchResult{}}
	handler := httppkg.NewSearchHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/search?q=%20%20attention%20%20", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "attention", mock.gotQuery)
}

func TestSearchHandler_UseCaseError(t *testing.T) {
	mock := &mockSearchUseCase{err: errors.New("db down")}
	handler := httppkg.NewSearchHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/search?q=x", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "internal server error", errResp.Error)
}

// newSearchRouter builds the RAG router with a search use case and the document
// routes stubbed out, so the search-route tests stay focused on search.
func newSearchRouter(search httppkg.SearchUseCase) http.Handler {
	return httppkg.NewRouter(search,
		httppkg.NewUploadDocumentHandler(nil, nil, nil, false),
		httppkg.NewDocumentStatusHandler(nil))
}

func TestRouter_RegistersSearchRoute(t *testing.T) {
	mock := &mockSearchUseCase{results: []domain.SearchResult{}}
	router := newSearchRouter(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/search?q=x", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, mock.calls)
}

func TestRouter_RejectsOtherMethods(t *testing.T) {
	mock := &mockSearchUseCase{}
	router := newSearchRouter(mock)

	req := httptest.NewRequest(http.MethodPost, "/api/rag/search?q=x", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// The stdlib mux answers 405 when the path matches a route but the method
	// does not, so the one route stays method-scoped.
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, 0, mock.calls)
}
