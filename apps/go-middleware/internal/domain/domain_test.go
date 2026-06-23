package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

func TestTopic_ZeroValue(t *testing.T) {
	var topic domain.Topic

	assert.Empty(t, topic.ID, "zero-value Topic ID should be empty")
	assert.Empty(t, topic.Name, "zero-value Topic Name should be empty")
	assert.True(t, topic.CreatedAt.IsZero(), "zero-value Topic CreatedAt should be zero time")
}

func TestTopic_FieldAssignment(t *testing.T) {
	now := time.Now()
	topic := domain.Topic{
		ID:        "topic-1",
		Name:      "Mathematics",
		CreatedAt: now,
	}

	assert.Equal(t, "topic-1", topic.ID)
	assert.Equal(t, "Mathematics", topic.Name)
	assert.True(t, topic.CreatedAt.Equal(now))
}

func TestConcept_ZeroValue(t *testing.T) {
	var concept domain.Concept

	assert.Empty(t, concept.ID)
	assert.Empty(t, concept.TopicID)
	assert.Empty(t, concept.Title)
	assert.Empty(t, concept.FilePath)
	assert.True(t, concept.CreatedAt.IsZero())
}

func TestConcept_FieldAssignment(t *testing.T) {
	now := time.Now()
	concept := domain.Concept{
		ID:        "concept-1",
		TopicID:   "topic-1",
		Title:     "Derivatives",
		FilePath:  "Math/Derivatives.md",
		CreatedAt: now,
	}

	assert.Equal(t, "concept-1", concept.ID)
	assert.Equal(t, "topic-1", concept.TopicID)
	assert.Equal(t, "Derivatives", concept.Title)
	assert.Equal(t, "Math/Derivatives.md", concept.FilePath)
	assert.True(t, concept.CreatedAt.Equal(now))
}

func TestFlashcard_ZeroValue(t *testing.T) {
	var card domain.Flashcard

	assert.Empty(t, card.ID)
	assert.Empty(t, card.ConceptID)
	assert.Empty(t, card.Question)
	assert.Empty(t, card.Answer)
	assert.Empty(t, card.ObsidianID)
	assert.True(t, card.CreatedAt.IsZero())
}

func TestFlashcard_FieldAssignment(t *testing.T) {
	now := time.Now()
	card := domain.Flashcard{
		ID:         "card-1",
		ConceptID:  "concept-1",
		Question:   "What is a derivative?",
		Answer:     "The rate of change of a function",
		ObsidianID: "obs-123",
		CreatedAt:  now,
	}

	assert.Equal(t, "card-1", card.ID)
	assert.Equal(t, "concept-1", card.ConceptID)
	assert.Equal(t, "What is a derivative?", card.Question)
	assert.Equal(t, "The rate of change of a function", card.Answer)
	assert.Equal(t, "obs-123", card.ObsidianID)
	assert.True(t, card.CreatedAt.Equal(now))
}

func TestDomainErrors(t *testing.T) {
	require.ErrorIs(t, domain.ErrNotFound, domain.ErrNotFound)
	require.ErrorIs(t, domain.ErrConflict, domain.ErrConflict)
	require.ErrorIs(t, domain.ErrInvalidInput, domain.ErrInvalidInput)

	assert.NotEqual(t, domain.ErrNotFound.Error(), domain.ErrConflict.Error())
	assert.NotEqual(t, domain.ErrNotFound.Error(), domain.ErrInvalidInput.Error())
}

func TestTopic_UUIDAssigned(t *testing.T) {
	topic := domain.Topic{
		ID:   "550e8400-e29b-41d4-a716-446655440000",
		Name: "Physics",
	}

	assert.Len(t, topic.ID, 36, "UUID v4 string should be 36 characters")
	assert.NotEmpty(t, topic.ID)
	assert.NotEmpty(t, topic.Name)
}

// --- Resource entity tests ---

func TestResource_ZeroValue(t *testing.T) {
	var r domain.Resource

	assert.Empty(t, r.ID)
	assert.Empty(t, r.TopicID)
	assert.Empty(t, r.Title)
	assert.Empty(t, r.Type)
	assert.Empty(t, r.SourceURI)
	assert.Empty(t, r.DifyDocumentID)
	assert.True(t, r.CreatedAt.IsZero())
}

func TestResource_FieldAssignment(t *testing.T) {
	now := time.Now()
	r := domain.Resource{
		ID:             "res-1",
		TopicID:        "topic-1",
		Title:          "Clean Architecture",
		Type:           domain.ResourceTypeBook,
		SourceURI:      "obsidian://clean-arch.md",
		DifyDocumentID: "dify-abc",
		CreatedAt:      now,
	}

	assert.Equal(t, "res-1", r.ID)
	assert.Equal(t, "topic-1", r.TopicID)
	assert.Equal(t, "Clean Architecture", r.Title)
	assert.Equal(t, domain.ResourceTypeBook, r.Type)
	assert.Equal(t, "obsidian://clean-arch.md", r.SourceURI)
	assert.Equal(t, "dify-abc", r.DifyDocumentID)
	assert.True(t, r.CreatedAt.Equal(now))
}

func TestResourceType_IsValid(t *testing.T) {
	assert.True(t, domain.ResourceTypeBook.IsValid())
	assert.True(t, domain.ResourceTypeNote.IsValid())
	assert.True(t, domain.ResourceTypeArticle.IsValid())
	assert.True(t, domain.ResourceTypeVideo.IsValid())

	assert.False(t, domain.ResourceType("podcast").IsValid())
	assert.False(t, domain.ResourceType("").IsValid())
	assert.False(t, domain.ResourceType("invalid").IsValid())
}

// --- CardState entity tests ---

func TestCardState_ZeroValue(t *testing.T) {
	var cs domain.CardState

	assert.Empty(t, cs.ID)
	assert.Empty(t, cs.FlashcardID)
	assert.Zero(t, cs.Stability)
	assert.Zero(t, cs.Difficulty)
	assert.True(t, cs.NextReview.IsZero())
	assert.True(t, cs.LastReview.IsZero())
}

func TestCardState_FieldAssignment(t *testing.T) {
	now := time.Now()
	future := now.Add(24 * time.Hour)
	cs := domain.CardState{
		ID:          "cs-1",
		FlashcardID: "flash-1",
		Stability:   2.5,
		Difficulty:  0.8,
		NextReview:  future,
		LastReview:  now,
	}

	assert.Equal(t, "cs-1", cs.ID)
	assert.Equal(t, "flash-1", cs.FlashcardID)
	assert.Equal(t, 2.5, cs.Stability)
	assert.Equal(t, 0.8, cs.Difficulty)
	assert.True(t, cs.NextReview.Equal(future))
	assert.True(t, cs.LastReview.Equal(now))
}

// --- ReviewLog entity tests ---

func TestReviewLog_ZeroValue(t *testing.T) {
	var rl domain.ReviewLog

	assert.Empty(t, rl.ID)
	assert.Empty(t, rl.FlashcardID)
	assert.Zero(t, rl.Grade)
	assert.Zero(t, rl.DurationMs)
	assert.True(t, rl.CreatedAt.IsZero())
}

func TestReviewLog_FieldAssignment(t *testing.T) {
	now := time.Now()
	rl := domain.ReviewLog{
		ID:          "log-1",
		FlashcardID: "flash-1",
		Grade:       3,
		DurationMs:  5000,
		CreatedAt:   now,
	}

	assert.Equal(t, "log-1", rl.ID)
	assert.Equal(t, "flash-1", rl.FlashcardID)
	assert.Equal(t, 3, rl.Grade)
	assert.Equal(t, 5000, rl.DurationMs)
	assert.True(t, rl.CreatedAt.Equal(now))
}

// --- DummyFSRS tests ---

func TestDummyFSRS_CalculateNextState_Again(t *testing.T) {
	// Grade 1 (Again): +0 days → NextReview unchanged from now
	algo := domain.DummyFSRS{}
	now := time.Now()
	current := domain.CardState{
		FlashcardID: "f1",
		Stability:   1.5,
		Difficulty:  0.7,
		NextReview:  now,
		LastReview:  now.Add(-24 * time.Hour),
	}

	result := algo.CalculateNextState(current, 1)

	assert.Equal(t, "f1", result.FlashcardID)
	assert.InDelta(t, 1.6, result.Stability, 0.01)
	assert.Equal(t, 0.7, result.Difficulty)
	assert.True(t, result.NextReview.After(now.Add(-1*time.Second)) && result.NextReview.Before(now.Add(1*time.Second)),
		"Again (grade 1) should set NextReview ≈ now (+0 days)")
}

func TestDummyFSRS_CalculateNextState_Good(t *testing.T) {
	// Grade 3 (Good): +3 days
	algo := domain.DummyFSRS{}
	now := time.Now()
	current := domain.CardState{
		FlashcardID: "f2",
		Stability:   2.0,
		Difficulty:  0.5,
		NextReview:  now,
		LastReview:  now.Add(-48 * time.Hour),
	}

	result := algo.CalculateNextState(current, 3)

	assert.Equal(t, "f2", result.FlashcardID)
	assert.InDelta(t, 2.1, result.Stability, 0.01)
	assert.Equal(t, 0.5, result.Difficulty)

	expectedNext := now.AddDate(0, 0, 3)
	diff := result.NextReview.Sub(expectedNext)
	assert.True(t, diff < 2*time.Second && diff > -2*time.Second,
		"Good (grade 3) should set NextReview to now + 3 days")
}

func TestDummyFSRS_CalculateNextState_Easy(t *testing.T) {
	// Grade 4 (Easy): +4 days
	algo := domain.DummyFSRS{}
	now := time.Now()
	current := domain.CardState{
		FlashcardID: "f3",
		Stability:   0.5,
		Difficulty:  0.9,
		NextReview:  now,
		LastReview:  now,
	}

	result := algo.CalculateNextState(current, 4)

	assert.Equal(t, "f3", result.FlashcardID)
	assert.InDelta(t, 0.6, result.Stability, 0.01)
	assert.Equal(t, 0.9, result.Difficulty)

	expectedNext := now.AddDate(0, 0, 4)
	diff := result.NextReview.Sub(expectedNext)
	assert.True(t, diff < 2*time.Second && diff > -2*time.Second,
		"Easy (grade 4) should set NextReview to now + 4 days")
}

func TestDummyFSRS_CalculateNextState_Hard(t *testing.T) {
	// Grade 2 (Hard): +1 day
	algo := domain.DummyFSRS{}
	now := time.Now()
	current := domain.CardState{
		FlashcardID: "f4",
		Stability:   3.0,
		Difficulty:  0.3,
		NextReview:  now.Add(-1 * time.Hour),
		LastReview:  now.Add(-2 * time.Hour),
	}

	result := algo.CalculateNextState(current, 2)

	assert.Equal(t, "f4", result.FlashcardID)
	assert.InDelta(t, 3.1, result.Stability, 0.01)
	assert.Equal(t, 0.3, result.Difficulty)

	expectedNext := now.AddDate(0, 0, 1)
	diff := result.NextReview.Sub(expectedNext)
	assert.True(t, diff < 2*time.Second && diff > -2*time.Second,
		"Hard (grade 2) should set NextReview to now + 1 day")
}
