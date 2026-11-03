package http_test

// The projection through the whole study mux.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/http"
)

type mockTodayUseCase struct {
	today app.Today
	err   error
}

func (m *mockTodayUseCase) Execute(ctx context.Context) (app.Today, error) {
	return m.today, m.err
}

func todayRouter(today *mockTodayUseCase) http.Handler {
	useCases := testUseCases()
	useCases.Today = today
	return httppkg.NewRouter(useCases)
}

func getToday(t *testing.T, today *mockTodayUseCase) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	todayRouter(today).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/study/today", nil))

	return rec
}

// S13: both kinds in one answer, each carrying the fields it owns and omitting the
// ones it does not. read_at belongs to a reading, title and note to a plan item.
func TestGetToday_ProjectsBothKinds(t *testing.T) {
	today := app.Today{
		DueCount: 3,
		Entries: []app.TodayEntry{
			{
				Kind:          app.TodayKindReadNotDistilled,
				DocumentID:    "doc-1",
				DocumentTitle: "DDD",
				FromPage:      24,
				ToPage:        36,
				ReadAt:        time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC),
			},
			{
				Kind:          app.TodayKindPlanNext,
				DocumentID:    "doc-1",
				DocumentTitle: "DDD",
				Title:         "Capítulo 5",
				FromPage:      100,
				ToPage:        118,
				Note:          "volver si hace falta",
			},
		},
	}

	rec := getToday(t, &mockTodayUseCase{today: today})

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{
		"due_count": 3,
		"entries": [
			{"kind":"read_not_distilled","document_id":"doc-1","document_title":"DDD","from_page":24,"to_page":36,"read_at":"2026-11-02T10:00:00Z"},
			{"kind":"plan_next","document_id":"doc-1","document_title":"DDD","title":"Capítulo 5","from_page":100,"to_page":118,"note":"volver si hace falta"}
		]
	}`, rec.Body.String())
}

// S13: nothing to do is a normal answer, and entries is [] rather than null, so a
// client can range over it without a guard.
func TestGetToday_NothingToDoIsAnEmptyList(t *testing.T) {
	rec := getToday(t, &mockTodayUseCase{today: app.Today{}})

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"due_count":0,"entries":[]}`, rec.Body.String())
}

// S13: a due count with nothing to read is a count and nothing else — there is no
// range to name, so no entry.
func TestGetToday_ADueCountAloneHasNoEntry(t *testing.T) {
	rec := getToday(t, &mockTodayUseCase{today: app.Today{DueCount: 4}})

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"due_count":4,"entries":[]}`, rec.Body.String())
}

// S13 rule 4: a title nobody can resolve travels as an empty string, not as a
// missing key and not as a failed request.
func TestGetToday_AnUnresolvedTitleIsAnEmptyString(t *testing.T) {
	rec := getToday(t, &mockTodayUseCase{today: app.Today{Entries: []app.TodayEntry{
		{
			Kind: app.TodayKindPlanNext, DocumentID: "doc-1",
			Title: "Capítulo 5", FromPage: 100, ToPage: 118,
		},
	}}})

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"due_count":0,"entries":[
		{"kind":"plan_next","document_id":"doc-1","document_title":"","title":"Capítulo 5","from_page":100,"to_page":118}
	]}`, rec.Body.String())
}
