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

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/application"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/interfaces/http"
)

// --- Mock use cases ---

type mockSyncConceptUseCase struct {
	id  string
	err error
}

func (m *mockSyncConceptUseCase) Execute(ctx context.Context, topicName, conceptTitle, filePath string) (string, error) {
	return m.id, m.err
}

type mockSyncFlashcardsUseCase struct {
	count int
	err   error
}

func (m *mockSyncFlashcardsUseCase) Execute(ctx context.Context, conceptID string, cards []domain.Flashcard) (int, error) {
	return m.count, m.err
}

type mockListConceptsUseCase struct {
	topics []application.TopicWithConcepts
	err    error
}

func (m *mockListConceptsUseCase) Execute(ctx context.Context) ([]application.TopicWithConcepts, error) {
	return m.topics, m.err
}

func TestHandleSyncConcept_Success(t *testing.T) {
	handler := httppkg.NewSyncConceptHandler(&mockSyncConceptUseCase{id: "concept-uuid-123"})

	body := `{"topic_name":"Math","concept_title":"Algebra","file_path":"math/algebra.md"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync/concept", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp httppkg.SyncConceptResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "concept-uuid-123", resp.ConceptID)
}

func TestHandleSyncConcept_Conflict(t *testing.T) {
	handler := httppkg.NewSyncConceptHandler(&mockSyncConceptUseCase{err: domain.ErrConflict})

	body := `{"topic_name":"Math","concept_title":"Algebra","file_path":"math/algebra.md"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync/concept", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var errResp httppkg.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "conflict", errResp.Error)
}

func TestHandleSyncConcept_InvalidJSON(t *testing.T) {
	handler := httppkg.NewSyncConceptHandler(&mockSyncConceptUseCase{id: "x"})

	body := `{invalid}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync/concept", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleSyncConcept_InternalError(t *testing.T) {
	handler := httppkg.NewSyncConceptHandler(&mockSyncConceptUseCase{err: errors.New("db connection lost")})

	body := `{"topic_name":"Math","concept_title":"Algebra","file_path":"math/algebra.md"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync/concept", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp httppkg.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "internal server error", errResp.Error)
}

func TestHandleSyncFlashcards_Success(t *testing.T) {
	handler := httppkg.NewSyncFlashcardsHandler(&mockSyncFlashcardsUseCase{count: 3})

	body := `{"concept_id":"c1","cards":[{"obsidian_id":"a","question":"Q","answer":"A"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync/flashcards", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp httppkg.SyncFlashcardsResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 3, resp.Synced)
}

func TestHandleSyncFlashcards_InvalidJSON(t *testing.T) {
	handler := httppkg.NewSyncFlashcardsHandler(&mockSyncFlashcardsUseCase{count: 0})

	body := `not json`
	req := httptest.NewRequest(http.MethodPost, "/api/sync/flashcards", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleSyncFlashcards_InternalError(t *testing.T) {
	handler := httppkg.NewSyncFlashcardsHandler(&mockSyncFlashcardsUseCase{err: errors.New("db down")})

	body := `{"concept_id":"c1","cards":[{"obsidian_id":"a","question":"Q","answer":"A"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync/flashcards", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleListConcepts_Success(t *testing.T) {
	expected := []application.TopicWithConcepts{
		{
			ID:   "t1",
			Name: "Math",
			Concepts: []application.ConceptListItem{
				{ID: "c1", Title: "Algebra", FilePath: "math/algebra.md"},
			},
		},
	}
	handler := httppkg.NewListConceptsHandler(&mockListConceptsUseCase{topics: expected})

	req := httptest.NewRequest(http.MethodGet, "/api/concepts", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var topics []httppkg.TopicResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&topics))
	assert.Len(t, topics, 1)
	assert.Equal(t, "Math", topics[0].Name)
	assert.Len(t, topics[0].Concepts, 1)
	assert.Equal(t, "Algebra", topics[0].Concepts[0].Title)
}

func TestHandleListConcepts_Empty(t *testing.T) {
	handler := httppkg.NewListConceptsHandler(&mockListConceptsUseCase{topics: []application.TopicWithConcepts{}})

	req := httptest.NewRequest(http.MethodGet, "/api/concepts", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var topics []httppkg.TopicResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&topics))
	assert.Empty(t, topics)
}

func TestHandleListConcepts_InternalError(t *testing.T) {
	handler := httppkg.NewListConceptsHandler(&mockListConceptsUseCase{err: errors.New("db down")})

	req := httptest.NewRequest(http.MethodGet, "/api/concepts", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
