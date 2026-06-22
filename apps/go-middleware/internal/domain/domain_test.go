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
