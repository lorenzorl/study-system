package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/http"
)

// inNonUTCZone pins time.Local to a fixed non-UTC zone for the duration of a
// test. Argentina is UTC-3, which is where a format string that hardcodes "Z"
// starts telling the client the wrong thing.
func inNonUTCZone(t *testing.T, offsetHours int) {
	t.Helper()

	original := time.Local
	time.Local = time.FixedZone("TEST", offsetHours*60*60)
	t.Cleanup(func() { time.Local = original })
}

// The client is told about an instant. Whatever zone the server's clock happens
// to be in, the string it sends must resolve back to that same instant.
func TestReviewHandler_NextReviewResolvesToTheInstantScheduled(t *testing.T) {
	inNonUTCZone(t, -3)

	want := time.Date(2026, 10, 20, 18, 0, 0, 0, time.UTC)

	handler := httppkg.NewReviewHandler(&mockSubmitReviewUseCase{
		state: domain.CardState{NextReview: want.In(time.Local)},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/study/review",
		strings.NewReader(`{"flashcard_id":"f1","grade":3,"duration_ms":1200}`))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)

	var response httppkg.ReviewResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	got, err := time.Parse(time.RFC3339, response.NextReview)
	require.NoError(t, err, "next_review must be a parseable instant, got %q", response.NextReview)

	assert.True(t, got.Equal(want),
		"next_review %q resolves to %s, but the card was scheduled for %s",
		response.NextReview, got.UTC().Format(time.RFC3339), want.Format(time.RFC3339))
}

func TestDueCardsHandler_NextReviewResolvesToTheStoredInstant(t *testing.T) {
	inNonUTCZone(t, -3)

	want := time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)

	handler := httppkg.NewDueCardsHandler(&mockGetDueCardsUseCase{
		cards: []app.DueCard{{
			FlashcardID: "f1",
			Front:       "q",
			Back:        "a",
			NextReview:  want.In(time.Local),
		}},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/study/due", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)

	var response []httppkg.DueCardResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response, 1)

	got, err := time.Parse(time.RFC3339, response[0].NextReview)
	require.NoError(t, err, "next_review must be a parseable instant, got %q", response[0].NextReview)

	assert.True(t, got.Equal(want),
		"next_review %q resolves to %s, but the card was due at %s",
		response[0].NextReview, got.UTC().Format(time.RFC3339), want.Format(time.RFC3339))
}

// created_at values happen to be UTC already, so a hardcoded "Z" is right by
// accident. The format must still not be able to lie if that changes.
func TestCreateTopicHandler_CreatedAtResolvesToTheStoredInstant(t *testing.T) {
	inNonUTCZone(t, -3)

	want := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	handler := httppkg.NewCreateTopicHandler(&mockCreateTopicUseCase{
		topic: &domain.Topic{ID: "t1", Name: "DDD", CreatedAt: want.In(time.Local)},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/topics",
		strings.NewReader(`{"name":"DDD"}`))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusCreated, recorder.Code)

	var response httppkg.CreateTopicResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	got, err := time.Parse(time.RFC3339, response.CreatedAt)
	require.NoError(t, err, "created_at must be a parseable instant, got %q", response.CreatedAt)

	assert.True(t, got.Equal(want),
		"created_at %q resolves to %s, but the topic was created at %s",
		response.CreatedAt, got.UTC().Format(time.RFC3339), want.Format(time.RFC3339))
}
