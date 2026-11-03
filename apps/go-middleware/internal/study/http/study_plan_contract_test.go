package http_test

// Contract tests for the plan routes (S6-S11 of odd/tasks/reading-coverage-and-plan.md),
// wired over a real migrated database so "stores nothing" and "one transaction" are
// asserted on rows and not on a mock.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/http"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// ctDocs is the study context's Documents port over a fixed catalogue.
type ctDocs struct{ ids map[string]string }

func (d *ctDocs) Title(ctx context.Context, documentID string) (string, error) {
	if title, ok := d.ids[documentID]; ok {
		return title, nil
	}
	return "", apperror.ErrNotFound
}

type ctHarness struct {
	sqlDB    *sql.DB
	router   http.Handler
	plans    *sqlite.StudyPlanRepository
	progress *sqlite.ReadingProgressRepository
}

func ctNew(t *testing.T) *ctHarness {
	t.Helper()
	ctx := t.Context()
	sqlDB, err := db.Open(ctx, ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx, sqlDB, sqlite.Migrations()))

	plans := sqlite.NewStudyPlanRepository(sqlDB)
	progress := sqlite.NewReadingProgressRepository(sqlDB)
	docs := &ctDocs{ids: map[string]string{"doc-1": "DDD"}}

	return &ctHarness{
		sqlDB:    sqlDB,
		plans:    plans,
		progress: progress,
		router: httppkg.NewRouter(httppkg.UseCases{
			SavePlan: app.NewSavePlanUseCase(docs, plans, progress),
			GetPlan:  app.NewGetPlanUseCase(plans, progress),
		}),
	}
}

// ctState digests every stored plan and item row, so a refusal can be proven to
// change nothing.
func ctState(t *testing.T, sqlDB *sql.DB) string {
	t.Helper()

	var plans, items string
	require.NoError(t, sqlDB.QueryRow(`
		SELECT COALESCE(group_concat(id||'|'||document_id||'|'||reading_mode||'|'||objective||'|'||
			source||'|'||created_at||'|'||COALESCE(superseded_at,'-'), ';'), '')
		FROM (SELECT * FROM study_plans ORDER BY id)`).Scan(&plans))
	require.NoError(t, sqlDB.QueryRow(`
		SELECT COALESCE(group_concat(id||'|'||plan_id||'|'||position||'|'||title||'|'||from_page||'|'||
			to_page||'|'||note||'|'||state, ';'), '')
		FROM (SELECT * FROM study_plan_items ORDER BY id)`).Scan(&items))

	return plans + " ## " + items
}

func ctPost(t *testing.T, router http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/api/study/plan", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, request)

	return rec
}

func ctGet(t *testing.T, router http.Handler, query string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/study/plan"+query, nil))

	return rec
}

const ctPlan = `{"document_id":"doc-1","reading_mode":"linear","objective":"decidir qué agregado",
"source":"sections","items":[{"title":"Capítulo 2","from_page":24,"to_page":36,"note":"primero"},
{"title":"Capítulo 5","from_page":100,"to_page":118,"note":"","state":"skipped"}]}`

// S7, S10: the 201 body, the stored rows, supersede-keeps-rows, and the read back.
func TestPlanPostAndGet_RoundTrip(t *testing.T) {
	h := ctNew(t)

	rec := ctPost(t, h.router, ctPlan)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var created httppkg.StudyPlanResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&created))
	assert.NotEmpty(t, created.PlanID)
	assert.Equal(t, "doc-1", created.DocumentID)
	assert.Equal(t, "linear", created.ReadingMode)
	assert.Equal(t, "decidir qué agregado", created.Objective)
	assert.Equal(t, "sections", created.Source)
	assert.NotEmpty(t, created.CreatedAt)
	require.Len(t, created.Items, 2)
	assert.Equal(t, 1, created.Items[0].Position)
	assert.Equal(t, "pending", created.Items[0].State)
	assert.False(t, created.Items[0].Read)
	assert.Equal(t, "skipped", created.Items[1].State)

	var plans, items int
	require.NoError(t, h.sqlDB.QueryRow(`SELECT count(*) FROM study_plans`).Scan(&plans))
	require.NoError(t, h.sqlDB.QueryRow(`SELECT count(*) FROM study_plan_items`).Scan(&items))
	assert.Equal(t, 1, plans)
	assert.Equal(t, 2, items, "the items must be stored, not only echoed")

	require.Equal(t, http.StatusCreated, ctPost(t, h.router, ctPlan).Code)
	require.NoError(t, h.sqlDB.QueryRow(`SELECT count(*) FROM study_plans`).Scan(&plans))
	require.NoError(t, h.sqlDB.QueryRow(`SELECT count(*) FROM study_plan_items`).Scan(&items))
	assert.Equal(t, 2, plans, "the superseded plan is kept")
	assert.Equal(t, 4, items, "the superseded plan's items are kept")

	var active int
	require.NoError(t, h.sqlDB.QueryRow(
		`SELECT count(*) FROM study_plans WHERE superseded_at IS NULL`).Scan(&active))
	assert.Equal(t, 1, active, "exactly one active plan per document")

	_, err := h.progress.Upsert(t.Context(), &domain.ReadingProgress{
		ID: "rp1", DocumentID: "doc-1", FromPage: 24, ToPage: 36,
		State: domain.ReadingStateRead, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	require.NoError(t, err)

	getRec := ctGet(t, h.router, "?document_id=doc-1")
	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())

	var fetched httppkg.StudyPlanResponse
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fetched))
	require.Len(t, fetched.Items, 2)
	assert.True(t, fetched.Items[0].Read, "the exact range is read")
	assert.False(t, fetched.Items[1].Read)
	assert.Equal(t, []int{1, 2}, []int{fetched.Items[0].Position, fetched.Items[1].Position})
}

// S8: every sentence the contract names, and no row written by any of them.
//
// This is the real-row counterpart of the handler test's mock assertion: the
// handler test proves the use case was not called, this one proves the database is
// byte-identical afterwards, which is the claim "nothing is stored" actually makes.
func TestPlanPost_EveryRefusalStoresNothing(t *testing.T) {
	h := ctNew(t)
	require.Equal(t, http.StatusCreated, ctPost(t, h.router, ctPlan).Code, "seed one plan")

	cases := []struct {
		name   string
		body   string
		status int
		error  string
	}{
		{"malformed", `{"document_id":`, 400, "invalid request body"},
		{"no document", `{"reading_mode":"linear","items":[{"title":"a","from_page":2,"to_page":3}]}`, 400, "document_id is required"},
		{"unknown mode", `{"document_id":"doc-1","reading_mode":"spiral","items":[{"title":"a","from_page":2,"to_page":3}]}`, 400, "reading_mode must be linear, project, survey or consult"},
		{"no items", `{"document_id":"doc-1","reading_mode":"linear","items":[]}`, 400, "items is required"},
		{"blank title", `{"document_id":"doc-1","reading_mode":"linear","items":[{"title":" ","from_page":2,"to_page":3}]}`, 400, "item 1: title is required"},
		{"page zero", `{"document_id":"doc-1","reading_mode":"linear","items":[{"title":"a","from_page":0,"to_page":3}]}`, 400, "item 1: from_page must be at least 1"},
		{"backwards", `{"document_id":"doc-1","reading_mode":"linear","items":[{"title":"a","from_page":9,"to_page":3}]}`, 400, "item 1: to_page must not be before from_page"},
		{"unknown state", `{"document_id":"doc-1","reading_mode":"linear","items":[{"title":"a","from_page":2,"to_page":3,"state":"read"}]}`, 400, "item 1: state must be pending or skipped"},
		{"duplicate", `{"document_id":"doc-1","reading_mode":"linear","items":[{"title":"a","from_page":24,"to_page":36},{"title":"b","from_page":24,"to_page":36}]}`, 400, "duplicate range 24-36"},
		{"second item", `{"document_id":"doc-1","reading_mode":"linear","items":[{"title":"a","from_page":2,"to_page":3},{"title":"b","from_page":30,"to_page":0}]}`, 400, "item 2: to_page must not be before from_page"},
		{"unknown document", `{"document_id":"doc-ghost","reading_mode":"linear","items":[{"title":"a","from_page":2,"to_page":3}]}`, 404, "not found"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			before := ctState(t, h.sqlDB)

			rec := ctPost(t, h.router, testCase.body)

			assert.Equal(t, testCase.status, rec.Code, rec.Body.String())
			assert.JSONEq(t, `{"error":"`+testCase.error+`"}`, rec.Body.String())
			assert.Equal(t, before, ctState(t, h.sqlDB), "a refused plan must store nothing")
		})
	}
}

// S9, S11: order is the array order, a client position is ignored, and a
// descending, partial plan is stored exactly as posted.
func TestPlanPost_OrderIsArrayOrderAndPagesAreNotReordered(t *testing.T) {
	h := ctNew(t)
	body := `{"document_id":"doc-1","reading_mode":"project","objective":"","source":"printed-contents",
	"items":[{"title":"Capítulo 5","from_page":100,"to_page":118,"position":7},
	{"title":"Capítulo 2","from_page":24,"to_page":36,"position":2}]}`

	rec := ctPost(t, h.router, body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var created httppkg.StudyPlanResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&created))
	require.Len(t, created.Items, 2)
	assert.Equal(t, 1, created.Items[0].Position, "position comes from the array")
	assert.Equal(t, "Capítulo 5", created.Items[0].Title, "pages are not reordered")
	assert.Equal(t, 2, created.Items[1].Position)

	fetched := ctGet(t, h.router, "?document_id=doc-1")
	require.Equal(t, http.StatusOK, fetched.Code)

	var got httppkg.StudyPlanResponse
	require.NoError(t, json.NewDecoder(fetched.Body).Decode(&got))
	assert.Equal(t, []int{100, 24}, []int{got.Items[0].FromPage, got.Items[1].FromPage})
}

// S7 atomicity: a failure while writing the items must leave the previous plan
// active and store nothing of the new one.
//
// The duplicate position is what makes the second insert fail inside the
// transaction, which is the only way to reach the rollback the spec promises.
func TestPlanCreate_FailedItemInsertRollsBack(t *testing.T) {
	h := ctNew(t)
	ctx := t.Context()

	_, err := h.plans.CreateActive(ctx, &domain.StudyPlan{
		ID: "p1", DocumentID: "doc-1", ReadingMode: domain.ReadingModeLinear, CreatedAt: time.Now(),
		Items: []domain.StudyPlanItem{
			{ID: "i1", Position: 1, Title: "Uno", FromPage: 1, ToPage: 10, State: domain.PlanItemStatePending},
		},
	})
	require.NoError(t, err)

	_, err = h.plans.CreateActive(ctx, &domain.StudyPlan{
		ID: "p2", DocumentID: "doc-1", ReadingMode: domain.ReadingModeLinear, CreatedAt: time.Now(),
		Items: []domain.StudyPlanItem{
			{ID: "i2", Position: 1, Title: "Dos", FromPage: 11, ToPage: 20, State: domain.PlanItemStatePending},
			{ID: "i3", Position: 1, Title: "Tres", FromPage: 21, ToPage: 30, State: domain.PlanItemStatePending},
		},
	})
	require.Error(t, err, "the duplicate position must fail the insert")

	var plans, superseded, items int
	require.NoError(t, h.sqlDB.QueryRow(`SELECT count(*) FROM study_plans`).Scan(&plans))
	require.NoError(t, h.sqlDB.QueryRow(
		`SELECT count(*) FROM study_plans WHERE superseded_at IS NOT NULL`).Scan(&superseded))
	require.NoError(t, h.sqlDB.QueryRow(`SELECT count(*) FROM study_plan_items`).Scan(&items))

	assert.Equal(t, 1, plans, "the half-written plan must be rolled back")
	assert.Equal(t, 0, superseded, "the supersede must roll back with it")
	assert.Equal(t, 1, items)

	active, err := h.plans.FindActiveByDocument(ctx, "doc-1")
	require.NoError(t, err)
	assert.Equal(t, "p1", active.ID, "the previous plan is still the active one")
}

// S10: items come back in position order even when inserted out of it.
func TestPlanGet_OrdersByPositionNotInsertion(t *testing.T) {
	h := ctNew(t)

	_, err := h.plans.CreateActive(t.Context(), &domain.StudyPlan{
		ID: "p1", DocumentID: "doc-1", ReadingMode: domain.ReadingModeSurvey, CreatedAt: time.Now(),
		Items: []domain.StudyPlanItem{
			{ID: "i2", Position: 2, Title: "Segundo", FromPage: 10, ToPage: 20, State: domain.PlanItemStatePending},
			{ID: "i1", Position: 1, Title: "Primero", FromPage: 50, ToPage: 60, State: domain.PlanItemStatePending},
		},
	})
	require.NoError(t, err)

	found, err := h.plans.FindActiveByDocument(t.Context(), "doc-1")
	require.NoError(t, err)
	require.Len(t, found.Items, 2)
	assert.Equal(t, []string{"Primero", "Segundo"}, []string{found.Items[0].Title, found.Items[1].Title})
}

// S10: no active plan is 404 not found; a missing document_id is 400.
func TestPlanGet_WithoutActivePlanIsNotFound(t *testing.T) {
	h := ctNew(t)

	rec := ctGet(t, h.router, "?document_id=doc-1")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.JSONEq(t, `{"error":"not found"}`, rec.Body.String())

	missing := ctGet(t, h.router, "")
	assert.Equal(t, http.StatusBadRequest, missing.Code)
	assert.JSONEq(t, `{"error":"document_id is required"}`, missing.Body.String())
}

// S6: read is not a column on the item, and items cascade with their plan.
func TestPlanSchema_NoReadColumnAndCascade(t *testing.T) {
	h := ctNew(t)

	_, err := h.plans.CreateActive(t.Context(), &domain.StudyPlan{
		ID: "p1", DocumentID: "doc-1", ReadingMode: domain.ReadingModeLinear, CreatedAt: time.Now(),
		Items: []domain.StudyPlanItem{
			{ID: "i1", Position: 1, Title: "Uno", FromPage: 1, ToPage: 10, State: domain.PlanItemStatePending},
		},
	})
	require.NoError(t, err)

	var columns string
	require.NoError(t, h.sqlDB.QueryRow(
		`SELECT group_concat(name) FROM pragma_table_info('study_plan_items')`).Scan(&columns))
	assert.NotContains(t, columns, "read")

	_, err = h.sqlDB.Exec(`DELETE FROM study_plans WHERE id = 'p1'`)
	require.NoError(t, err)

	var items int
	require.NoError(t, h.sqlDB.QueryRow(
		`SELECT count(*) FROM study_plan_items WHERE plan_id = 'p1'`).Scan(&items))
	assert.Equal(t, 0, items, "ON DELETE CASCADE must remove the plan's items")
}
