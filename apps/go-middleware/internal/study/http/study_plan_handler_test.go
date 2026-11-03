package http_test

// The plan routes through the whole study mux.

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

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/http"
)

type mockSavePlanUseCase struct {
	view   app.StudyPlanView
	err    error
	inputs []app.SavePlanInput
}

func (m *mockSavePlanUseCase) Execute(ctx context.Context, input app.SavePlanInput) (app.StudyPlanView, error) {
	m.inputs = append(m.inputs, input)
	return m.view, m.err
}

type mockGetPlanUseCase struct {
	view        app.StudyPlanView
	err         error
	errSingle   error
	documentIDs []string
}

func (m *mockGetPlanUseCase) Execute(ctx context.Context, documentID string) (app.StudyPlanView, error) {
	m.documentIDs = append(m.documentIDs, documentID)
	if m.err != nil {
		return app.StudyPlanView{}, m.err
	}
	return m.view, nil
}

func planRouter(save *mockSavePlanUseCase, get *mockGetPlanUseCase) http.Handler {
	useCases := testUseCases()
	useCases.SavePlan = save
	useCases.GetPlan = get
	return httppkg.NewRouter(useCases)
}

func samplePlanView() app.StudyPlanView {
	return app.StudyPlanView{
		ID:          "p1",
		DocumentID:  "doc-1",
		ReadingMode: domain.ReadingModeLinear,
		Objective:   "decidir qué agregado va en qué contexto",
		Source:      "sections",
		CreatedAt:   time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC),
		Items: []app.PlanItemView{
			{StudyPlanItem: domain.StudyPlanItem{
				ID: "i1", Position: 1, Title: "Capítulo 2", FromPage: 24, ToPage: 36,
				State: domain.PlanItemStatePending,
			}, Read: true},
			{StudyPlanItem: domain.StudyPlanItem{
				ID: "i2", Position: 2, Title: "Capítulo 5", FromPage: 100, ToPage: 118,
				Note: "volver si hace falta", State: domain.PlanItemStateSkipped,
			}},
		},
	}
}

func savePlanRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/study/plan", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

const validPlanBody = `{"document_id":"doc-1","reading_mode":"linear","objective":"decidir","source":"sections","items":[{"title":"Capítulo 2","from_page":24,"to_page":36,"note":""}]}`

// S7: a stored plan answers 201 with every item and its derived read flag.
func TestPostPlan_StoresAndEchoesThePlan(t *testing.T) {
	save := &mockSavePlanUseCase{view: samplePlanView()}
	rec := httptest.NewRecorder()

	planRouter(save, &mockGetPlanUseCase{}).ServeHTTP(rec, savePlanRequest(validPlanBody))

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var response httppkg.StudyPlanResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	assert.Equal(t, "p1", response.PlanID)
	assert.Equal(t, "doc-1", response.DocumentID)
	assert.Equal(t, "linear", response.ReadingMode)
	assert.Equal(t, "2026-11-03T09:00:00Z", response.CreatedAt)
	require.Len(t, response.Items, 2)
	assert.Equal(t, 1, response.Items[0].Position)
	assert.Equal(t, "Capítulo 2", response.Items[0].Title)
	assert.True(t, response.Items[0].Read)
	assert.False(t, response.Items[1].Read)
	assert.Equal(t, "skipped", response.Items[1].State)
	assert.Equal(t, "volver si hace falta", response.Items[1].Note)

	require.Len(t, save.inputs, 1)
	assert.Equal(t, "doc-1", save.inputs[0].DocumentID)
	require.Len(t, save.inputs[0].Items, 1)
	assert.Equal(t, 24, save.inputs[0].Items[0].FromPage)
}

// S8: each refusal names the item and the field. Nothing reaches the use case.
func TestPostPlan_RefusesWithTheSentenceTheContractNames(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{name: "malformed body", body: `{"document_id":`, message: "invalid request body"},
		{name: "no document", body: `{"reading_mode":"linear","items":[{"title":"a","from_page":1,"to_page":2}]}`, message: "document_id is required"},
		{name: "unknown mode", body: `{"document_id":"doc-1","reading_mode":"spiral","items":[{"title":"a","from_page":1,"to_page":2}]}`, message: "reading_mode must be linear, project, survey or consult"},
		{name: "no mode", body: `{"document_id":"doc-1","items":[{"title":"a","from_page":1,"to_page":2}]}`, message: "reading_mode must be linear, project, survey or consult"},
		{name: "no items", body: `{"document_id":"doc-1","reading_mode":"linear","items":[]}`, message: "items is required"},
		{name: "blank title", body: `{"document_id":"doc-1","reading_mode":"linear","items":[{"title":"  ","from_page":1,"to_page":2}]}`, message: "item 1: title is required"},
		{name: "page zero", body: `{"document_id":"doc-1","reading_mode":"linear","items":[{"title":"a","from_page":0,"to_page":2}]}`, message: "item 1: from_page must be at least 1"},
		{name: "backwards range", body: `{"document_id":"doc-1","reading_mode":"linear","items":[{"title":"a","from_page":9,"to_page":2}]}`, message: "item 1: to_page must not be before from_page"},
		{name: "unknown state", body: `{"document_id":"doc-1","reading_mode":"linear","items":[{"title":"a","from_page":1,"to_page":2,"state":"half"}]}`, message: "item 1: state must be pending or skipped"},
		{name: "duplicate range", body: `{"document_id":"doc-1","reading_mode":"linear","items":[{"title":"a","from_page":24,"to_page":36},{"title":"b","from_page":24,"to_page":36}]}`, message: "duplicate range 24-36"},
		{name: "second item named second", body: `{"document_id":"doc-1","reading_mode":"linear","items":[{"title":"a","from_page":1,"to_page":2},{"title":"b","from_page":3,"to_page":0}]}`, message: "item 2: to_page must not be before from_page"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			save := &mockSavePlanUseCase{}
			rec := httptest.NewRecorder()

			planRouter(save, &mockGetPlanUseCase{}).ServeHTTP(rec, savePlanRequest(testCase.body))

			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			var failure httpx.ErrorResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&failure))
			assert.Equal(t, testCase.message, failure.Error)
			assert.Empty(t, save.inputs, "a refused plan never reaches the use case")
		})
	}
}

// S8: an unknown document is the 404 the use case signals, not a 400.
func TestPostPlan_UnknownDocumentIsNotFound(t *testing.T) {
	save := &mockSavePlanUseCase{err: apperror.ErrNotFound}
	rec := httptest.NewRecorder()

	planRouter(save, &mockGetPlanUseCase{}).ServeHTTP(rec, savePlanRequest(validPlanBody))

	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"error":"not found"}`, rec.Body.String())
}

// S10: the read answers the active plan with its derived read flags.
func TestGetPlan_ReturnsTheActivePlan(t *testing.T) {
	get := &mockGetPlanUseCase{view: samplePlanView()}
	rec := httptest.NewRecorder()

	planRouter(&mockSavePlanUseCase{}, get).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/study/plan?document_id=doc-1", nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"doc-1"}, get.documentIDs)

	var response httppkg.StudyPlanResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	require.Len(t, response.Items, 2)
	assert.True(t, response.Items[0].Read)
	assert.False(t, response.Items[1].Read)
}

func TestGetPlan_ADocumentWithoutAPlanIsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()

	planRouter(&mockSavePlanUseCase{}, &mockGetPlanUseCase{err: apperror.ErrNotFound}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/study/plan?document_id=doc-1", nil))

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.JSONEq(t, `{"error":"not found"}`, rec.Body.String())
}

func TestGetPlan_RefusesAMissingDocumentID(t *testing.T) {
	get := &mockGetPlanUseCase{}
	rec := httptest.NewRecorder()

	planRouter(&mockSavePlanUseCase{}, get).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/study/plan", nil))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var failure httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&failure))
	assert.Equal(t, "document_id is required", failure.Error)
	assert.Empty(t, get.documentIDs)
}
