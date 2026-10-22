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

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/http"
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
	topics []app.TopicWithConcepts
	err    error
}

func (m *mockListConceptsUseCase) Execute(ctx context.Context) ([]app.TopicWithConcepts, error) {
	return m.topics, m.err
}

type mockCreateTopicUseCase struct {
	topic *domain.Topic
	err   error
}

func (m *mockCreateTopicUseCase) Execute(ctx context.Context, name string) (*domain.Topic, error) {
	return m.topic, m.err
}

type mockCreateConceptUseCase struct {
	concept *domain.Concept
	err     error
}

func (m *mockCreateConceptUseCase) Execute(ctx context.Context, topicID, title string) (*domain.Concept, error) {
	return m.concept, m.err
}

type mockSyncResourceUseCase struct {
	resourceID string
	err        error
}

type mockListResourcesUseCase struct {
	summaries []app.ResourceSummary
	err       error
}

func (m *mockListResourcesUseCase) Execute(ctx context.Context) ([]app.ResourceSummary, error) {
	return m.summaries, m.err
}

func (m *mockSyncResourceUseCase) Execute(ctx context.Context, topicName, title, resourceType, sourceURI string) (string, error) {
	return m.resourceID, m.err
}

type mockGetDueCardsUseCase struct {
	cards []app.DueCard
	err   error
}

func (m *mockGetDueCardsUseCase) Execute(ctx context.Context) ([]app.DueCard, error) {
	return m.cards, m.err
}

type mockSubmitReviewUseCase struct {
	state domain.CardState
	err   error
}

func (m *mockSubmitReviewUseCase) Execute(ctx context.Context, flashcardID string, grade, durationMs int) (domain.CardState, error) {
	return m.state, m.err
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
	handler := httppkg.NewSyncConceptHandler(&mockSyncConceptUseCase{err: apperror.ErrConflict})

	body := `{"topic_name":"Math","concept_title":"Algebra","file_path":"math/algebra.md"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync/concept", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var errResp httpx.ErrorResponse
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

	var errResp httpx.ErrorResponse
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
	expected := []app.TopicWithConcepts{
		{
			ID:   "t1",
			Name: "Math",
			Concepts: []app.ConceptListItem{
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
	handler := httppkg.NewListConceptsHandler(&mockListConceptsUseCase{topics: []app.TopicWithConcepts{}})

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

// --- CreateTopicHandler tests ---

func TestHandleCreateTopic_Success(t *testing.T) {
	now := time.Now().UTC()
	topic := &domain.Topic{ID: "uuid-1", Name: "DDD", CreatedAt: now}
	handler := httppkg.NewCreateTopicHandler(&mockCreateTopicUseCase{topic: topic})

	body := `{"name":"DDD"}`
	req := httptest.NewRequest(http.MethodPost, "/api/topics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp httppkg.CreateTopicResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "uuid-1", resp.ID)
	assert.Equal(t, "DDD", resp.Name)
	assert.NotEmpty(t, resp.CreatedAt)
}

func TestHandleCreateTopic_EmptyName(t *testing.T) {
	handler := httppkg.NewCreateTopicHandler(&mockCreateTopicUseCase{err: apperror.ErrInvalidInput})

	body := `{"name":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/topics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "invalid input", errResp.Error)
}

func TestHandleCreateTopic_Conflict(t *testing.T) {
	handler := httppkg.NewCreateTopicHandler(&mockCreateTopicUseCase{err: apperror.ErrConflict})

	body := `{"name":"DDD"}`
	req := httptest.NewRequest(http.MethodPost, "/api/topics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "conflict", errResp.Error)
}

func TestHandleCreateTopic_InvalidJSON(t *testing.T) {
	handler := httppkg.NewCreateTopicHandler(&mockCreateTopicUseCase{topic: &domain.Topic{}})

	body := `not json`
	req := httptest.NewRequest(http.MethodPost, "/api/topics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleCreateTopic_InternalError(t *testing.T) {
	handler := httppkg.NewCreateTopicHandler(&mockCreateTopicUseCase{err: errors.New("db down")})

	body := `{"name":"DDD"}`
	req := httptest.NewRequest(http.MethodPost, "/api/topics", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// --- CreateConceptHandler tests ---

func TestHandleCreateConcept_Success(t *testing.T) {
	now := time.Now().UTC()
	concept := &domain.Concept{
		ID: "c-uuid", TopicID: "t1", Title: "Aggregates",
		FilePath: "manual/DDD/Aggregates.md", CreatedAt: now,
	}
	handler := httppkg.NewCreateConceptHandler(&mockCreateConceptUseCase{concept: concept})

	body := `{"topic_id":"t1","title":"Aggregates"}`
	req := httptest.NewRequest(http.MethodPost, "/api/concepts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp httppkg.CreateConceptResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "c-uuid", resp.ID)
	assert.Equal(t, "t1", resp.TopicID)
	assert.Equal(t, "Aggregates", resp.Title)
	assert.Equal(t, "manual/DDD/Aggregates.md", resp.FilePath)
	assert.NotEmpty(t, resp.CreatedAt)
}

func TestHandleCreateConcept_EmptyTitle(t *testing.T) {
	handler := httppkg.NewCreateConceptHandler(&mockCreateConceptUseCase{err: apperror.ErrInvalidInput})

	body := `{"topic_id":"t1","title":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/concepts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "invalid input", errResp.Error)
}

func TestHandleCreateConcept_TopicNotFound(t *testing.T) {
	handler := httppkg.NewCreateConceptHandler(&mockCreateConceptUseCase{err: apperror.ErrNotFound})

	body := `{"topic_id":"t99","title":"X"}`
	req := httptest.NewRequest(http.MethodPost, "/api/concepts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "not found", errResp.Error)
}

func TestHandleCreateConcept_InvalidJSON(t *testing.T) {
	handler := httppkg.NewCreateConceptHandler(&mockCreateConceptUseCase{concept: &domain.Concept{}})

	body := `{broken`
	req := httptest.NewRequest(http.MethodPost, "/api/concepts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleCreateConcept_InternalError(t *testing.T) {
	handler := httppkg.NewCreateConceptHandler(&mockCreateConceptUseCase{err: errors.New("db down")})

	body := `{"topic_id":"t1","title":"Aggregates"}`
	req := httptest.NewRequest(http.MethodPost, "/api/concepts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// --- SyncResourceHandler tests ---

func TestHandleSyncResource_Success(t *testing.T) {
	handler := httppkg.NewSyncResourceHandler(&mockSyncResourceUseCase{resourceID: "r-uuid-123"})

	body := `{"topic_name":"DDD","resource_title":"DDD Book","type":"book","source_uri":"obsidian://ddd.md"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync/resource", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp httppkg.SyncResourceResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "r-uuid-123", resp.ResourceID)
}

func TestHandleSyncResource_MissingTopicName(t *testing.T) {
	handler := httppkg.NewSyncResourceHandler(&mockSyncResourceUseCase{resourceID: "x"})

	body := `{"resource_title":"Book","type":"book","source_uri":"obsidian://x.md"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync/resource", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Contains(t, errResp.Error, "topic_name")
}

func TestHandleSyncResource_InvalidType(t *testing.T) {
	handler := httppkg.NewSyncResourceHandler(&mockSyncResourceUseCase{err: apperror.ErrInvalidInput})

	body := `{"topic_name":"DDD","resource_title":"Podcast","type":"podcast","source_uri":"obsidian://x.md"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync/resource", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "invalid input", errResp.Error)
}

func TestHandleSyncResource_InvalidJSON(t *testing.T) {
	handler := httppkg.NewSyncResourceHandler(&mockSyncResourceUseCase{resourceID: "x"})

	body := `{broken`
	req := httptest.NewRequest(http.MethodPost, "/api/sync/resource", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleSyncResource_InternalError(t *testing.T) {
	handler := httppkg.NewSyncResourceHandler(&mockSyncResourceUseCase{err: errors.New("db down")})

	body := `{"topic_name":"DDD","resource_title":"Book","type":"book","source_uri":"obsidian://x.md"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync/resource", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// --- ListResourcesHandler tests ---

func TestHandleListResources_Success(t *testing.T) {
	created := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	handler := httppkg.NewListResourcesHandler(&mockListResourcesUseCase{
		summaries: []app.ResourceSummary{
			{
				ID:        "r-1",
				Title:     "Clean Architecture",
				Type:      domain.ResourceTypeBook,
				SourceURI: "obsidian://clean-arch.md",
				TopicName: "DDD",
				CreatedAt: created,
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/resources", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resources []httppkg.ResourceSummaryResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resources))
	require.Len(t, resources, 1)
	assert.Equal(t, "r-1", resources[0].ID)
	assert.Equal(t, "Clean Architecture", resources[0].Title)
	assert.Equal(t, "book", resources[0].Type)
	assert.Equal(t, "obsidian://clean-arch.md", resources[0].SourceURI)
	assert.Equal(t, "DDD", resources[0].TopicName)
	assert.Equal(t, "2026-03-01T12:00:00Z", resources[0].CreatedAt)
}

func TestHandleListResources_EmptyCorpusEncodesAsArray(t *testing.T) {
	handler := httppkg.NewListResourcesHandler(&mockListResourcesUseCase{
		summaries: []app.ResourceSummary{},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/resources", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "[]", strings.TrimSpace(rec.Body.String()))
}

func TestHandleListResources_InternalError(t *testing.T) {
	handler := httppkg.NewListResourcesHandler(&mockListResourcesUseCase{err: errors.New("db down")})

	req := httptest.NewRequest(http.MethodGet, "/api/resources", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// --- DueCardsHandler tests ---

func TestHandleDueCards_Success(t *testing.T) {
	cards := []app.DueCard{
		{
			FlashcardID:  "f1",
			Front:        "What is DDD?",
			Back:         "Domain-Driven Design",
			ConceptTitle: "DDD Intro",
			TopicName:    "Architecture",
			NextReview:   time.Now().UTC(),
		},
	}
	handler := httppkg.NewDueCardsHandler(&mockGetDueCardsUseCase{cards: cards})

	req := httptest.NewRequest(http.MethodGet, "/api/study/due", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp []httppkg.DueCardResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Len(t, resp, 1)
	assert.Equal(t, "f1", resp[0].ID)
	assert.Equal(t, "What is DDD?", resp[0].Question)
	assert.Equal(t, "Domain-Driven Design", resp[0].Answer)
	assert.Equal(t, "DDD Intro", resp[0].ConceptTitle)
	assert.Equal(t, "Architecture", resp[0].TopicName)
	assert.NotEmpty(t, resp[0].NextReview)
}

func TestHandleDueCards_Empty(t *testing.T) {
	handler := httppkg.NewDueCardsHandler(&mockGetDueCardsUseCase{cards: []app.DueCard{}})

	req := httptest.NewRequest(http.MethodGet, "/api/study/due", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp []httppkg.DueCardResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Empty(t, resp)
}

func TestHandleDueCards_InternalError(t *testing.T) {
	handler := httppkg.NewDueCardsHandler(&mockGetDueCardsUseCase{err: errors.New("db down")})

	req := httptest.NewRequest(http.MethodGet, "/api/study/due", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// --- ReviewHandler tests ---

func TestHandleReview_Success(t *testing.T) {
	now := time.Now().UTC()
	nextReview := now.AddDate(0, 0, 3)
	state := domain.CardState{
		ID:          "cs-1",
		FlashcardID: "f1",
		Stability:   1.1,
		Difficulty:  0.5,
		NextReview:  nextReview,
		LastReview:  now,
	}
	handler := httppkg.NewReviewHandler(&mockSubmitReviewUseCase{state: state})

	body := `{"flashcard_id":"f1","grade":3,"duration_ms":5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/study/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp httppkg.ReviewResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.NotEmpty(t, resp.NextReview)
	assert.Equal(t, 1.1, resp.Stability)
	assert.Equal(t, 0.5, resp.Difficulty)
}

func TestHandleReview_MissingFlashcardID(t *testing.T) {
	handler := httppkg.NewReviewHandler(&mockSubmitReviewUseCase{
		state: domain.CardState{FlashcardID: "f1"},
	})

	body := `{"flashcard_id":"","grade":3,"duration_ms":5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/study/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Contains(t, errResp.Error, "flashcard_id")
}

func TestHandleReview_InvalidGrade(t *testing.T) {
	handler := httppkg.NewReviewHandler(&mockSubmitReviewUseCase{
		state: domain.CardState{FlashcardID: "f1"},
	})

	body := `{"flashcard_id":"f1","grade":5,"duration_ms":5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/study/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Contains(t, errResp.Error, "grade")
}

func TestHandleReview_NotFound(t *testing.T) {
	handler := httppkg.NewReviewHandler(&mockSubmitReviewUseCase{err: apperror.ErrNotFound})

	body := `{"flashcard_id":"f-missing","grade":3,"duration_ms":5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/study/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var errResp httpx.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "not found", errResp.Error)
}

func TestHandleReview_InvalidJSON(t *testing.T) {
	handler := httppkg.NewReviewHandler(&mockSubmitReviewUseCase{
		state: domain.CardState{FlashcardID: "f1"},
	})

	body := `{broken`
	req := httptest.NewRequest(http.MethodPost, "/api/study/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleReview_InternalError(t *testing.T) {
	handler := httppkg.NewReviewHandler(&mockSubmitReviewUseCase{err: errors.New("db down")})

	body := `{"flashcard_id":"f1","grade":3,"duration_ms":5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/study/review", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
