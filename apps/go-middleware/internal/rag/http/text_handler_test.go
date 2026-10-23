package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/http"
)

// textBodyCeiling is the body budget the handler tests configure. It is small so
// an oversized request stays cheap to build; production passes
// STUDY_MAX_UPLOAD_BYTES.
const textBodyCeiling = 1024

// fakeIngestTextUseCase is a hand-written IngestTextUseCase. It records the last
// request so a test can assert what the handler forwarded, not only what it
// answered, and it is never reached by a rejected request.
type fakeIngestTextUseCase struct {
	report app.IngestTextReport
	err    error

	calls int
	got   app.IngestTextRequest
}

func (f *fakeIngestTextUseCase) Execute(_ context.Context, request app.IngestTextRequest) (app.IngestTextReport, error) {
	f.calls++
	f.got = request
	return f.report, f.err
}

// textPost builds a JSON POST of the pasted-text endpoint.
func textPost(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/rag/documents/text", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func newTextHandler(useCase *fakeIngestTextUseCase) *httppkg.IngestTextHandler {
	return httppkg.NewIngestTextHandler(useCase, textBodyCeiling)
}

// T3/S4: a pasted text is accepted and answered with the upload response shape
// plus the two counts that say whether vectors were written.
func TestIngestTextHandler_StoresThePastedText(t *testing.T) {
	useCase := &fakeIngestTextUseCase{report: app.IngestTextReport{
		DocumentID: "doc-1",
		State:      app.StateIndexing,
		Chunks:     3,
		Embedded:   0,
	}}

	rec := httptest.NewRecorder()
	newTextHandler(useCase).ServeHTTP(rec, textPost(`{"title":"  Notes  ","text":"pasted material"}`))

	require.Equal(t, http.StatusCreated, rec.Code)

	// Assert the exact wire keys, not only the decoded struct: a wrong json tag
	// would still round-trip through the same struct.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	assert.ElementsMatch(t, []string{"document_id", "state", "chunks", "embedded"}, keysOf(raw))

	var resp httppkg.IngestTextResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "doc-1", resp.DocumentID)
	assert.Equal(t, "indexing", resp.State)
	assert.Equal(t, 3, resp.Chunks)
	assert.Equal(t, 0, resp.Embedded)

	require.Equal(t, 1, useCase.calls)
	assert.Equal(t, "Notes", useCase.got.Title, "the title reaches the use case trimmed")
	assert.Equal(t, "pasted material", useCase.got.Text)
}

// T3/S4: each refusal has its own message, so a client can tell a missing title
// from a missing text.
func TestIngestTextHandler_RejectsMissingFields(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		message string
	}{
		{"missing title", `{"text":"material"}`, "title is required"},
		{"blank title", `{"title":"   ","text":"material"}`, "title is required"},
		{"missing text", `{"title":"Notes"}`, "text is required"},
		{"blank text", `{"title":"Notes","text":" \n\t "}`, "text is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useCase := &fakeIngestTextUseCase{}

			rec := httptest.NewRecorder()
			newTextHandler(useCase).ServeHTTP(rec, textPost(tc.body))

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, 0, useCase.calls, "a refused request never reaches the use case")

			var errResp httpx.ErrorResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
			assert.Equal(t, tc.message, errResp.Error)
		})
	}
}

// T3/S4: the title is bounded, and the message names the bound so the caller
// knows what to shorten.
func TestIngestTextHandler_RejectsATitleAboveTheBound(t *testing.T) {
	useCase := &fakeIngestTextUseCase{}
	long := strings.Repeat("a", httppkg.MaxTextDocumentTitleRunes+1)
	body, err := json.Marshal(httppkg.IngestTextRequest{Title: long, Text: "material"})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	newTextHandler(useCase).ServeHTTP(rec, textPost(string(body)))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 0, useCase.calls)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "title must be at most 200 characters", errResp.Error)
}

// T3/S4: the bound is inclusive, so the longest accepted title still works.
func TestIngestTextHandler_AcceptsATitleAtTheBound(t *testing.T) {
	useCase := &fakeIngestTextUseCase{report: app.IngestTextReport{DocumentID: "doc-1", State: app.StateIndexing}}
	long := strings.Repeat("a", httppkg.MaxTextDocumentTitleRunes)
	body, err := json.Marshal(httppkg.IngestTextRequest{Title: long, Text: "material"})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	newTextHandler(useCase).ServeHTTP(rec, textPost(string(body)))

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, 1, useCase.calls)
	assert.Equal(t, long, useCase.got.Title)
}

// T3/S4: a malformed body is the client's mistake and the decoder's reason is
// what tells them where.
func TestIngestTextHandler_MalformedJSONIsABadRequestWithTheDecodersReason(t *testing.T) {
	useCase := &fakeIngestTextUseCase{}

	rec := httptest.NewRecorder()
	newTextHandler(useCase).ServeHTTP(rec, textPost(`{"title": "Notes",`))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 0, useCase.calls)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	// The exact wording belongs to the stdlib decoder and differs between a
	// truncated stream ("unexpected EOF") and a malformed value; what this test
	// owns is that the reason travels with the 400 instead of a generic message.
	assert.Contains(t, errResp.Error, "invalid JSON body: ")
	assert.NotEqual(t, "invalid JSON body: ", errResp.Error,
		"the decoder's reason is carried, not replaced by a generic message")
}

// T3/S4: an oversized body is refused with the ceiling the PDF path uses, and
// nothing is buffered past it or handed to the use case.
func TestIngestTextHandler_OversizedBodyIsRequestEntityTooLarge(t *testing.T) {
	useCase := &fakeIngestTextUseCase{}
	huge := `{"title":"Notes","text":"` + strings.Repeat("a", textBodyCeiling) + `"}`

	rec := httptest.NewRecorder()
	newTextHandler(useCase).ServeHTTP(rec, textPost(huge))

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, 0, useCase.calls)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Contains(t, errResp.Error, "1024")
}

// An announced length over the ceiling is refused before a byte is read.
func TestIngestTextHandler_OversizedContentLengthIsRefusedUpFront(t *testing.T) {
	useCase := &fakeIngestTextUseCase{}

	req := textPost(`{"title":"Notes","text":"material"}`)
	req.ContentLength = textBodyCeiling + 1
	rec := httptest.NewRecorder()
	newTextHandler(useCase).ServeHTTP(rec, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, 0, useCase.calls)
}

// T3/S4: the same text twice is a conflict, not a crash and not a second row.
func TestIngestTextHandler_DuplicateTextIsAConflict(t *testing.T) {
	useCase := &fakeIngestTextUseCase{err: apperror.ErrConflict}

	rec := httptest.NewRecorder()
	newTextHandler(useCase).ServeHTTP(rec, textPost(`{"title":"Notes","text":"material"}`))

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Equal(t, 1, useCase.calls)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "a document with the same text already exists", errResp.Error)
}

// Any other failure stays an internal error and leaks nothing.
func TestIngestTextHandler_UnexpectedErrorIsInternal(t *testing.T) {
	useCase := &fakeIngestTextUseCase{err: errors.New("db down")}

	rec := httptest.NewRecorder()
	newTextHandler(useCase).ServeHTTP(rec, textPost(`{"title":"Notes","text":"material"}`))

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "internal server error", errResp.Error)
}

// The route is mounted under the collection, so both creation paths share the
// prefix and a client adds one path segment to swap the source of the material.
func TestRouter_RegistersTheTextIngestRoute(t *testing.T) {
	useCase := &fakeIngestTextUseCase{report: app.IngestTextReport{DocumentID: "doc-1", State: app.StateIndexing, Chunks: 1}}

	router := httppkg.NewRouter(
		&mockSearchUseCase{results: []domain.SearchResult{}},
		httppkg.NewUploadDocumentHandler(nil, nil, nil, false),
		httppkg.NewDocumentStatusHandler(nil),
		httppkg.NewDocumentsHandler(&fakeDocumentListUseCase{}),
		newTextHandler(useCase),
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, textPost(`{"title":"Notes","text":"material"}`))

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, 1, useCase.calls)
}
