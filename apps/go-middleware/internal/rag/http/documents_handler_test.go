package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/http"
)

// fakeDocumentListUseCase is a hand-written DocumentListUseCase. It records that
// it was called, so a router test can tell the route reached it.
type fakeDocumentListUseCase struct {
	summaries []app.DocumentSummary
	err       error
	calls     int
}

func (f *fakeDocumentListUseCase) Execute(_ context.Context) ([]app.DocumentSummary, error) {
	f.calls++
	return f.summaries, f.err
}

func TestDocumentsHandler_ReturnsTheSummaries(t *testing.T) {
	createdAt := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	mock := &fakeDocumentListUseCase{summaries: []app.DocumentSummary{{
		ID:        "doc-1",
		Title:     "DDD.pdf",
		CreatedAt: createdAt,
		Pages:     359,
		Chunks:    1045,
	}}}
	handler := httppkg.NewDocumentsHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/documents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	// Assert the exact wire keys, not only the decoded struct: a wrong json tag
	// would still round-trip through the same struct, and the server internals
	// must not leak.
	var raw []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	require.Len(t, raw, 1)
	assert.ElementsMatch(t, []string{"id", "title", "created_at", "pages", "chunks"}, keysOf(raw[0]))
	assert.NotContains(t, keysOf(raw[0]), "artifact_id")
	assert.NotContains(t, keysOf(raw[0]), "source_uri")
	assert.NotContains(t, keysOf(raw[0]), "content_sha256")

	var resp []httppkg.DocumentSummaryResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "doc-1", resp[0].ID)
	assert.Equal(t, "DDD.pdf", resp[0].Title)
	assert.Equal(t, createdAt.Format(time.RFC3339), resp[0].CreatedAt)
	assert.Equal(t, 359, resp[0].Pages)
	assert.Equal(t, 1045, resp[0].Chunks)
}

func TestDocumentsHandler_EmptyCorpusEncodesAsArray(t *testing.T) {
	mock := &fakeDocumentListUseCase{summaries: []app.DocumentSummary{}}
	handler := httppkg.NewDocumentsHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/documents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// A nil slice would encode as null; the consumer ranges over this, so it must
	// be a bare array.
	assert.Equal(t, "[]", strings.TrimSpace(rec.Body.String()))
}

func TestDocumentsHandler_UseCaseError(t *testing.T) {
	mock := &fakeDocumentListUseCase{err: errors.New("db down")}
	handler := httppkg.NewDocumentsHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/documents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestRouter_RegistersTheDocumentsListRoute(t *testing.T) {
	mock := &fakeDocumentListUseCase{summaries: []app.DocumentSummary{}}
	router := newSearchRouterWithDocuments(&mockSearchUseCase{}, mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/documents", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, mock.calls, "GET /api/rag/documents reaches the list handler")
}
