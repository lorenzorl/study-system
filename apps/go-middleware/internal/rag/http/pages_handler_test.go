package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/http"
)

// fakePagesUseCase is a hand-written DocumentPagesUseCase. It records the
// document and the range the handler forwarded, so a refused parameter can prove
// the use case was never asked.
type fakePagesUseCase struct {
	pages   app.DocumentPages
	err     error
	calls   int
	gotID   string
	gotFrom int
	gotTo   int
}

func (f *fakePagesUseCase) Execute(
	_ context.Context, documentID string, from, to int,
) (app.DocumentPages, error) {
	f.calls++
	f.gotID, f.gotFrom, f.gotTo = documentID, from, to
	return f.pages, f.err
}

// pagesRequest builds a request already carrying the path value the mux would
// have set, so the handler is exercised without the router.
func pagesRequest(target string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.SetPathValue("documentID", "doc-1")
	return req
}

func TestPagesHandler_ReturnsTheRequestedRange(t *testing.T) {
	mock := &fakePagesUseCase{pages: app.DocumentPages{
		DocumentID: "doc-1",
		Title:      "DDD.pdf",
		TotalPages: 359,
		Pages: []app.PageText{
			{PageNumber: 12, Status: domain.PageExtracted, Method: domain.MethodExtract, Text: "el agregado"},
			{PageNumber: 13, Status: domain.PageFailed, Method: domain.MethodVision, Error: "the model answered nothing"},
		},
	}}
	handler := httppkg.NewPagesHandler(mock)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, pagesRequest("/api/rag/documents/doc-1/pages?from=12&to=13"))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "doc-1", mock.gotID)
	assert.Equal(t, 12, mock.gotFrom)
	assert.Equal(t, 13, mock.gotTo)

	// Pin the wire keys, not only the decoded struct: a wrong json tag would
	// still round-trip through the same struct.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	assert.ElementsMatch(t, []string{"document_id", "title", "total_pages", "pages"}, keysOf(raw))

	var pages []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw["pages"], &pages))
	require.Len(t, pages, 2)
	assert.ElementsMatch(t, []string{"page_number", "status", "method", "text"}, keysOf(pages[0]))
	assert.ElementsMatch(t, []string{"page_number", "status", "method", "text", "error"}, keysOf(pages[1]))

	var resp httppkg.DocumentPagesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "doc-1", resp.DocumentID)
	assert.Equal(t, "DDD.pdf", resp.Title)
	assert.Equal(t, 359, resp.TotalPages)
	require.Len(t, resp.Pages, 2)
	assert.Equal(t, 12, resp.Pages[0].PageNumber)
	assert.Equal(t, "extracted", resp.Pages[0].Status)
	assert.Equal(t, "extract", resp.Pages[0].Method)
	assert.Equal(t, "el agregado", resp.Pages[0].Text)
	assert.Empty(t, resp.Pages[0].Error)
	assert.Equal(t, 13, resp.Pages[1].PageNumber)
	assert.Equal(t, "failed", resp.Pages[1].Status)
	assert.Equal(t, "the model answered nothing", resp.Pages[1].Error)
}

func TestPagesHandler_RefusesAnInvalidRange(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		message string
	}{
		{name: "missing from", target: "/api/rag/documents/doc-1/pages?to=3", message: "from is required"},
		{name: "missing to", target: "/api/rag/documents/doc-1/pages?from=1", message: "to is required"},
		{
			name:    "non-numeric from",
			target:  "/api/rag/documents/doc-1/pages?from=uno&to=3",
			message: "from must be an integer",
		},
		{
			name:    "non-numeric to",
			target:  "/api/rag/documents/doc-1/pages?from=1&to=dos",
			message: "to must be an integer",
		},
		{
			name:    "from below the first page",
			target:  "/api/rag/documents/doc-1/pages?from=0&to=3",
			message: "from must be at least 1",
		},
		{
			name:    "to before from",
			target:  "/api/rag/documents/doc-1/pages?from=5&to=4",
			message: "to must be at least from (5)",
		},
		{
			name:   "over the page cap",
			target: "/api/rag/documents/doc-1/pages?from=1&to=51",
			// The cap is named, because a caller who reads "too many pages" has no
			// way to know what to ask for instead.
			message: "at most 50 pages can be requested per call",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mock := &fakePagesUseCase{}
			handler := httppkg.NewPagesHandler(mock)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, pagesRequest(test.target))

			require.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp httpx.ErrorResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
			assert.Equal(t, test.message, errResp.Error)
			assert.Zero(t, mock.calls, "a refused parameter never reaches the use case")
		})
	}
}

// A range that ends past the document's last page is refused by the use case,
// which is the only layer that knows how many pages the document has; the
// transport prints its refusal and names the maximum.
func TestPagesHandler_RangeBeyondTheDocumentNamesTheMaximum(t *testing.T) {
	mock := &fakePagesUseCase{err: app.ValidatePageRange(340, 360, 359)}
	handler := httppkg.NewPagesHandler(mock)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, pagesRequest("/api/rag/documents/doc-1/pages?from=340&to=360"))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "to must be at most 359, the document's last page", errResp.Error)
}

func TestPagesHandler_UnknownDocumentIsNotFound(t *testing.T) {
	mock := &fakePagesUseCase{err: apperror.ErrNotFound}
	handler := httppkg.NewPagesHandler(mock)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, pagesRequest("/api/rag/documents/missing/pages?from=1&to=2"))

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, 1, mock.calls)
}

func TestRouter_RegistersThePagesRoute(t *testing.T) {
	pages := &fakePagesUseCase{pages: app.DocumentPages{DocumentID: "doc-1", TotalPages: 10}}
	router := httppkg.NewRouter(
		&mockSearchUseCase{},
		httppkg.NewUploadDocumentHandler(nil, nil, nil, false),
		httppkg.NewDocumentStatusHandler(nil),
		httppkg.NewPagesHandler(pages),
		httppkg.NewDocumentsHandler(&fakeDocumentListUseCase{}),
		newTextHandler(&fakeIngestTextUseCase{}),
		newURLHandler(&fakeIngestTextUseCase{}),
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rag/documents/doc-1/pages?from=3&to=4", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "doc-1", pages.gotID)
	assert.Equal(t, 3, pages.gotFrom)
	assert.Equal(t, 4, pages.gotTo)
}
