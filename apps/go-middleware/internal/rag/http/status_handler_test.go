package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/http"
)

// fakeStatusUseCase is a hand-written DocumentStatusUseCase. It records the
// document ID the handler forwarded, not only what it answered.
type fakeStatusUseCase struct {
	status app.DocumentStatus
	err    error
	gotID  string
}

func (f *fakeStatusUseCase) Execute(_ context.Context, documentID string) (app.DocumentStatus, error) {
	f.gotID = documentID
	return f.status, f.err
}

func startedAt() *time.Time {
	instant := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	return &instant
}

func TestStatusHandler_ReturnsTheStatusObject(t *testing.T) {
	started := startedAt()
	finished := started.Add(20 * time.Minute)
	mock := &fakeStatusUseCase{status: app.DocumentStatus{
		DocumentID: "doc-1",
		Title:      "DDD.pdf",
		State:      app.StateCompleted,
		Pages: domain.Progress{
			Total: 359, Extracted: 333, Transcribed: 26, Failed: 0, Pending: 0,
		},
		Chunks:     1045,
		Embedded:   1045,
		StartedAt:  started,
		UpdatedAt:  &finished,
		FinishedAt: &finished,
	}}
	handler := httppkg.NewDocumentStatusHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/documents/doc-1/status", nil)
	req.SetPathValue("documentID", "doc-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "doc-1", mock.gotID)

	// Assert the exact wire keys, not only the decoded struct: a wrong json tag
	// would still round-trip through the same struct.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	assert.ElementsMatch(t, []string{
		"document_id", "title", "state", "pages", "chunks", "embedded",
		"started_at", "updated_at", "finished_at",
	}, keysOf(raw))

	var pages map[string]int
	require.NoError(t, json.Unmarshal(raw["pages"], &pages))
	assert.Equal(t, map[string]int{
		"total": 359, "extracted": 333, "transcribed": 26, "failed": 0, "pending": 0,
	}, pages)

	var resp httppkg.DocumentStatusResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "doc-1", resp.DocumentID)
	assert.Equal(t, "DDD.pdf", resp.Title)
	assert.Equal(t, "completed", resp.State)
	assert.Equal(t, 1045, resp.Chunks)
	assert.Equal(t, 1045, resp.Embedded)
	assert.Equal(t, started.Format(time.RFC3339), resp.StartedAt)
	assert.Equal(t, finished.Format(time.RFC3339), resp.UpdatedAt)
	assert.Equal(t, finished.Format(time.RFC3339), resp.FinishedAt)
}

func TestStatusHandler_OmitsEmptyErrorAndTimestamps(t *testing.T) {
	mock := &fakeStatusUseCase{status: app.DocumentStatus{
		DocumentID: "doc-1",
		Title:      "DDD.pdf",
		State:      app.StateQueued,
	}}
	handler := httppkg.NewDocumentStatusHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/documents/doc-1/status", nil)
	req.SetPathValue("documentID", "doc-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	assert.NotContains(t, keysOf(raw), "error")
	assert.NotContains(t, keysOf(raw), "started_at")
	assert.NotContains(t, keysOf(raw), "updated_at")
	assert.NotContains(t, keysOf(raw), "finished_at")
}

func TestStatusHandler_UnknownDocumentIsNotFound(t *testing.T) {
	mock := &fakeStatusUseCase{err: apperror.ErrNotFound}
	handler := httppkg.NewDocumentStatusHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/rag/documents/missing/status", nil)
	req.SetPathValue("documentID", "missing")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "missing", mock.gotID)
}

func TestRouter_RegistersTheDocumentRoutes(t *testing.T) {
	store := &fakeStore{}
	documents := newFakeDocuments()
	upload := httppkg.NewUploadDocumentHandler(store, documents, &fakeProcessor{}, true)
	status := &fakeStatusUseCase{status: app.DocumentStatus{DocumentID: "doc-1", State: app.StateQueued}}
	router := httppkg.NewRouter(&mockSearchUseCase{}, upload,
		httppkg.NewDocumentStatusHandler(status), httppkg.NewDocumentsHandler(&fakeDocumentListUseCase{}),
		// The text and URL routes are the other creation paths; this test owns the
		// PDF one.
		newTextHandler(&fakeIngestTextUseCase{}),
		newURLHandler(&fakeIngestTextUseCase{}))

	post := httptest.NewRecorder()
	router.ServeHTTP(post, multipartRequest(t, "/api/rag/documents", "file", "book.pdf", []byte("%PDF-1.7")))
	require.Equal(t, http.StatusAccepted, post.Code, "POST /api/rag/documents reaches the upload handler")

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/rag/documents/doc-1/status", nil))
	require.Equal(t, http.StatusOK, get.Code, "GET /api/rag/documents/{id}/status reaches the status handler")
	assert.Equal(t, "doc-1", status.gotID)
}
