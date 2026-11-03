package http_test

// The coverage routes through the whole study mux.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/http"
)

type mockSetReadingProgressUseCase struct {
	progress domain.ReadingProgress
	err      error
	inputs   []app.SetReadingProgressInput
}

func (m *mockSetReadingProgressUseCase) Execute(
	ctx context.Context,
	input app.SetReadingProgressInput,
) (domain.ReadingProgress, error) {
	m.inputs = append(m.inputs, input)
	return m.progress, m.err
}

type mockListReadingProgressUseCase struct {
	ranges      []domain.ReadingProgress
	err         error
	documentIDs []string
}

func (m *mockListReadingProgressUseCase) Execute(
	ctx context.Context,
	documentID string,
) ([]domain.ReadingProgress, error) {
	m.documentIDs = append(m.documentIDs, documentID)
	return m.ranges, m.err
}

func readingRouter(set *mockSetReadingProgressUseCase, list *mockListReadingProgressUseCase) http.Handler {
	useCases := testUseCases()
	useCases.SetReadingProgress = set
	useCases.ListReadingProgress = list
	return httppkg.NewRouter(useCases)
}

func putReading(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPut, "/api/study/reading", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

// S3: marking a range answers the stored row, timestamps included.
func TestPutReading_MarksARangeAndEchoesTheStoredRow(t *testing.T) {
	marked := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	set := &mockSetReadingProgressUseCase{progress: domain.ReadingProgress{
		ID: "rp1", DocumentID: "doc-1", FromPage: 24, ToPage: 36,
		State: domain.ReadingStateRead, CreatedAt: marked, UpdatedAt: marked,
	}}
	rec := httptest.NewRecorder()

	readingRouter(set, &mockListReadingProgressUseCase{}).
		ServeHTTP(rec, putReading(`{"document_id":"doc-1","from_page":24,"to_page":36,"state":"read"}`))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var response httppkg.ReadingProgressResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	assert.Equal(t, "doc-1", response.DocumentID)
	assert.Equal(t, 24, response.FromPage)
	assert.Equal(t, 36, response.ToPage)
	assert.Equal(t, "read", response.State)
	assert.Equal(t, "2026-11-02T10:00:00Z", response.CreatedAt)
	assert.Equal(t, "2026-11-02T10:00:00Z", response.UpdatedAt)

	require.Len(t, set.inputs, 1)
	assert.Equal(t, "read", set.inputs[0].State)
}

// S4: every refusal names what to fix, and a refused body never reaches the use
// case, so nothing is stored.
func TestPutReading_RefusesWithTheSentenceTheContractNames(t *testing.T) {
	valid := `{"document_id":"doc-1","from_page":24,"to_page":36,"state":"read"}`

	cases := []struct {
		name    string
		body    string
		message string
	}{
		{name: "malformed body", body: `{"document_id":`, message: "invalid request body"},
		{name: "no document", body: `{"from_page":24,"to_page":36,"state":"read"}`, message: "document_id is required"},
		{name: "blank document", body: `{"document_id":"   ","from_page":24,"to_page":36,"state":"read"}`, message: "document_id is required"},
		{name: "page zero", body: `{"document_id":"doc-1","from_page":0,"to_page":36,"state":"read"}`, message: "from_page must be at least 1"},
		{name: "backwards range", body: `{"document_id":"doc-1","from_page":36,"to_page":24,"state":"read"}`, message: "to_page must not be before from_page"},
		{name: "unknown state", body: `{"document_id":"doc-1","from_page":24,"to_page":36,"state":"skimmed"}`, message: "state must be read or unread"},
		{name: "no state", body: `{"document_id":"doc-1","from_page":24,"to_page":36}`, message: "state must be read or unread"},
		{name: "valid, for contrast", body: valid, message: ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			set := &mockSetReadingProgressUseCase{progress: domain.ReadingProgress{
				DocumentID: "doc-1", FromPage: 24, ToPage: 36, State: domain.ReadingStateRead,
			}}
			rec := httptest.NewRecorder()

			readingRouter(set, &mockListReadingProgressUseCase{}).ServeHTTP(rec, putReading(testCase.body))

			if testCase.message == "" {
				assert.Equal(t, http.StatusOK, rec.Code)
				assert.Len(t, set.inputs, 1)
				return
			}

			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			var failure httpx.ErrorResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&failure))
			assert.Equal(t, testCase.message, failure.Error)
			assert.Empty(t, set.inputs, "a refused body never reaches the use case")
		})
	}
}

// S5: the list answers an array, and an empty document is [] rather than null.
func TestGetReading_ListsADocumentsRanges(t *testing.T) {
	list := &mockListReadingProgressUseCase{ranges: []domain.ReadingProgress{
		{DocumentID: "doc-1", FromPage: 24, ToPage: 36, State: domain.ReadingStateRead, UpdatedAt: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)},
	}}
	rec := httptest.NewRecorder()

	readingRouter(&mockSetReadingProgressUseCase{}, list).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/study/reading?document_id=doc-1", nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"doc-1"}, list.documentIDs)

	var ranges []httppkg.ReadingProgressResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&ranges))
	require.Len(t, ranges, 1)
	assert.Equal(t, 24, ranges[0].FromPage)
	assert.Equal(t, "2026-11-02T10:00:00Z", ranges[0].UpdatedAt)
}

func TestGetReading_EmptyDocumentEncodesAsAnEmptyArray(t *testing.T) {
	rec := httptest.NewRecorder()

	readingRouter(&mockSetReadingProgressUseCase{}, &mockListReadingProgressUseCase{}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/study/reading?document_id=doc-1", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `[]`, rec.Body.String())
}

func TestGetReading_RefusesAMissingDocumentID(t *testing.T) {
	list := &mockListReadingProgressUseCase{}
	rec := httptest.NewRecorder()

	readingRouter(&mockSetReadingProgressUseCase{}, list).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/study/reading", nil))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var failure httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&failure))
	assert.Equal(t, "document_id is required", failure.Error)
	assert.Empty(t, list.documentIDs)
}
