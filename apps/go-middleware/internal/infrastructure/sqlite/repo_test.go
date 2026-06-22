package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/infrastructure/sqlite"
)

func openTestDB(t *testing.T) (domain.TopicRepository, domain.ConceptRepository, domain.FlashcardRepository, func()) {
	t.Helper()

	db, err := sqlite.Open(":memory:")
	require.NoError(t, err, "failed to open in-memory test database")

	topicRepo := sqlite.NewTopicRepository(db)
	conceptRepo := sqlite.NewConceptRepository(db)
	flashcardRepo := sqlite.NewFlashcardRepository(db)

	cleanup := func() {
		db.Close()
	}

	return topicRepo, conceptRepo, flashcardRepo, cleanup
}

func TestTopicRepository_UpsertByName_Create(t *testing.T) {
	topicRepo, _, _, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	topic, err := topicRepo.UpsertByName(ctx, "Mathematics")
	require.NoError(t, err)
	assert.NotEmpty(t, topic.ID, "new topic should have an ID assigned")
	assert.Equal(t, "Mathematics", topic.Name)
	assert.False(t, topic.CreatedAt.IsZero(), "new topic should have CreatedAt set")
}

func TestTopicRepository_UpsertByName_Idempotent(t *testing.T) {
	// R2: upsert by Name — should return same ID on second call
	topicRepo, _, _, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	first, err := topicRepo.UpsertByName(ctx, "Science")
	require.NoError(t, err)

	second, err := topicRepo.UpsertByName(ctx, "Science")
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID, "same name should return same topic ID")
	assert.Equal(t, first.Name, second.Name)
}

func TestTopicRepository_ListAll(t *testing.T) {
	topicRepo, _, _, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	_, err := topicRepo.UpsertByName(ctx, "Math")
	require.NoError(t, err)
	_, err = topicRepo.UpsertByName(ctx, "Science")
	require.NoError(t, err)

	topics, err := topicRepo.ListAll(ctx)
	require.NoError(t, err)
	assert.Len(t, topics, 2)

	namesFound := make(map[string]bool)
	for _, t := range topics {
		namesFound[t.Name] = true
	}
	assert.True(t, namesFound["Math"])
	assert.True(t, namesFound["Science"])
}

func TestConceptRepository_UpsertByPath_Create(t *testing.T) {
	// R3: create concept and retrieve
	topicRepo, conceptRepo, _, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	topic, err := topicRepo.UpsertByName(ctx, "Math")
	require.NoError(t, err)

	concept, err := conceptRepo.UpsertByPath(ctx, topic.ID, "Algebra", "math/algebra.md")
	require.NoError(t, err)
	assert.NotEmpty(t, concept.ID)
	assert.Equal(t, topic.ID, concept.TopicID)
	assert.Equal(t, "Algebra", concept.Title)
	assert.Equal(t, "math/algebra.md", concept.FilePath)
	assert.False(t, concept.CreatedAt.IsZero())
}

func TestConceptRepository_UpsertByPath_Idempotent(t *testing.T) {
	topicRepo, conceptRepo, _, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	topic, err := topicRepo.UpsertByName(ctx, "Math")
	require.NoError(t, err)

	first, err := conceptRepo.UpsertByPath(ctx, topic.ID, "Algebra", "math/algebra.md")
	require.NoError(t, err)

	second, err := conceptRepo.UpsertByPath(ctx, topic.ID, "Algebra", "math/algebra.md")
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID, "same file path should return same concept ID")
	assert.Equal(t, first.Title, second.Title)
}

func TestConceptRepository_ListByTopicID(t *testing.T) {
	topicRepo, conceptRepo, _, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	topic, err := topicRepo.UpsertByName(ctx, "Math")
	require.NoError(t, err)

	_, err = conceptRepo.UpsertByPath(ctx, topic.ID, "Algebra", "math/algebra.md")
	require.NoError(t, err)
	_, err = conceptRepo.UpsertByPath(ctx, topic.ID, "Geometry", "math/geometry.md")
	require.NoError(t, err)

	concepts, err := conceptRepo.ListByTopicID(ctx, topic.ID)
	require.NoError(t, err)
	assert.Len(t, concepts, 2)

	titles := make(map[string]bool)
	for _, c := range concepts {
		titles[c.Title] = true
	}
	assert.True(t, titles["Algebra"])
	assert.True(t, titles["Geometry"])
}

func TestFlashcardRepository_UpsertByObsidianID_Create(t *testing.T) {
	// R4: create card with ObsidianID
	topicRepo, conceptRepo, flashcardRepo, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	topic, err := topicRepo.UpsertByName(ctx, "Math")
	require.NoError(t, err)
	concept, err := conceptRepo.UpsertByPath(ctx, topic.ID, "Algebra", "math/algebra.md")
	require.NoError(t, err)

	count, err := flashcardRepo.UpsertByObsidianID(ctx, concept.ID, []domain.Flashcard{
		{
			ID:         uuid.New().String(),
			ConceptID:  concept.ID,
			Question:   "What is x?",
			Answer:     "A variable",
			ObsidianID: "obs-card-1",
			CreatedAt:  time.Now(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestFlashcardRepository_UpsertByObsidianID_Update(t *testing.T) {
	// R4: update existing card by ObsidianID
	topicRepo, conceptRepo, flashcardRepo, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	topic, err := topicRepo.UpsertByName(ctx, "Math")
	require.NoError(t, err)
	concept, err := conceptRepo.UpsertByPath(ctx, topic.ID, "Algebra", "math/algebra.md")
	require.NoError(t, err)

	// Create first
	_, err = flashcardRepo.UpsertByObsidianID(ctx, concept.ID, []domain.Flashcard{
		{
			ID:         uuid.New().String(),
			ConceptID:  concept.ID,
			Question:   "2+2?",
			Answer:     "4",
			ObsidianID: "obs-update-1",
			CreatedAt:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Update same ObsidianID
	count, err := flashcardRepo.UpsertByObsidianID(ctx, concept.ID, []domain.Flashcard{
		{
			ID:         uuid.New().String(),
			ConceptID:  concept.ID,
			Question:   "3+3?",
			Answer:     "6",
			ObsidianID: "obs-update-1",
			CreatedAt:  time.Now(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, count, "upsert should return 1 for the updated card")
}

func TestFlashcardRepository_UpsertByObsidianID_BatchCreate(t *testing.T) {
	// R6: batch upsert scenario
	topicRepo, conceptRepo, flashcardRepo, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	topic, err := topicRepo.UpsertByName(ctx, "Science")
	require.NoError(t, err)
	concept, err := conceptRepo.UpsertByPath(ctx, topic.ID, "Biology", "science/biology.md")
	require.NoError(t, err)

	count, err := flashcardRepo.UpsertByObsidianID(ctx, concept.ID, []domain.Flashcard{
		{
			ID:         uuid.New().String(),
			ConceptID:  concept.ID,
			Question:   "Q1",
			Answer:     "A1",
			ObsidianID: "obs-batch-a",
			CreatedAt:  time.Now(),
		},
		{
			ID:         uuid.New().String(),
			ConceptID:  concept.ID,
			Question:   "Q2",
			Answer:     "A2",
			ObsidianID: "obs-batch-b",
			CreatedAt:  time.Now(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestTopicRepository_Create_Success(t *testing.T) {
	topicRepo, _, _, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	topic := &domain.Topic{
		ID:        uuid.New().String(),
		Name:      "Computer Science",
		CreatedAt: time.Now().UTC(),
	}

	err := topicRepo.Create(ctx, topic)
	require.NoError(t, err)

	// Verify it can be found
	found, err := topicRepo.FindByID(ctx, topic.ID)
	require.NoError(t, err)
	assert.Equal(t, topic.Name, found.Name)
	assert.Equal(t, topic.ID, found.ID)
}

func TestTopicRepository_Create_DuplicateName(t *testing.T) {
	topicRepo, _, _, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	first := &domain.Topic{
		ID:        uuid.New().String(),
		Name:      "Physics",
		CreatedAt: time.Now().UTC(),
	}
	err := topicRepo.Create(ctx, first)
	require.NoError(t, err)

	second := &domain.Topic{
		ID:        uuid.New().String(),
		Name:      "Physics",
		CreatedAt: time.Now().UTC(),
	}
	err = topicRepo.Create(ctx, second)
	assert.ErrorIs(t, err, domain.ErrConflict)
}

func TestTopicRepository_FindByID_Existing(t *testing.T) {
	topicRepo, _, _, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	topic := &domain.Topic{
		ID:        uuid.New().String(),
		Name:      "Literature",
		CreatedAt: time.Now().UTC(),
	}
	err := topicRepo.Create(ctx, topic)
	require.NoError(t, err)

	found, err := topicRepo.FindByID(ctx, topic.ID)
	require.NoError(t, err)
	assert.Equal(t, topic.Name, found.Name)
	assert.False(t, found.CreatedAt.IsZero())
}

func TestTopicRepository_FindByID_NotFound(t *testing.T) {
	topicRepo, _, _, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	_, err := topicRepo.FindByID(ctx, "nonexistent-id")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestConceptRepository_Create_Success(t *testing.T) {
	topicRepo, conceptRepo, _, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	topic := &domain.Topic{
		ID:        uuid.New().String(),
		Name:      "DDD",
		CreatedAt: time.Now().UTC(),
	}
	err := topicRepo.Create(ctx, topic)
	require.NoError(t, err)

	concept := &domain.Concept{
		ID:        uuid.New().String(),
		TopicID:   topic.ID,
		Title:     "Aggregates",
		FilePath:  "ddd/aggregates.md",
		CreatedAt: time.Now().UTC(),
	}

	err = conceptRepo.Create(ctx, concept)
	require.NoError(t, err)

	// Verify via ListByTopicID
	concepts, err := conceptRepo.ListByTopicID(ctx, topic.ID)
	require.NoError(t, err)
	assert.Len(t, concepts, 1)
	assert.Equal(t, "Aggregates", concepts[0].Title)
}

func TestConceptRepository_Create_DuplicateFilePath(t *testing.T) {
	topicRepo, conceptRepo, _, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	topic := &domain.Topic{
		ID:        uuid.New().String(),
		Name:      "Go",
		CreatedAt: time.Now().UTC(),
	}
	err := topicRepo.Create(ctx, topic)
	require.NoError(t, err)

	first := &domain.Concept{
		ID:        uuid.New().String(),
		TopicID:   topic.ID,
		Title:     "Concurrency",
		FilePath:  "go/concurrency.md",
		CreatedAt: time.Now().UTC(),
	}
	err = conceptRepo.Create(ctx, first)
	require.NoError(t, err)

	second := &domain.Concept{
		ID:        uuid.New().String(),
		TopicID:   topic.ID,
		Title:     "Concurrency",
		FilePath:  "go/concurrency.md",
		CreatedAt: time.Now().UTC(),
	}
	err = conceptRepo.Create(ctx, second)
	assert.ErrorIs(t, err, domain.ErrConflict)
}

func TestFlashcardRepository_UpsertByObsidianID_MixedInsertUpdate(t *testing.T) {
	// R6: mixed insert and update in same batch
	topicRepo, conceptRepo, flashcardRepo, cleanup := openTestDB(t)
	defer cleanup()
	ctx := context.Background()

	topic, err := topicRepo.UpsertByName(ctx, "Science")
	require.NoError(t, err)
	concept, err := conceptRepo.UpsertByPath(ctx, topic.ID, "Physics", "science/physics.md")
	require.NoError(t, err)

	// Create one card first
	_, err = flashcardRepo.UpsertByObsidianID(ctx, concept.ID, []domain.Flashcard{
		{
			ID:         uuid.New().String(),
			ConceptID:  concept.ID,
			Question:   "Old question",
			Answer:     "Old answer",
			ObsidianID: "obs-existing",
			CreatedAt:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Mixed batch: update existing + insert new
	count, err := flashcardRepo.UpsertByObsidianID(ctx, concept.ID, []domain.Flashcard{
		{
			ID:         uuid.New().String(),
			ConceptID:  concept.ID,
			Question:   "Updated question",
			Answer:     "Updated answer",
			ObsidianID: "obs-existing",
			CreatedAt:  time.Now(),
		},
		{
			ID:         uuid.New().String(),
			ConceptID:  concept.ID,
			Question:   "New question",
			Answer:     "New answer",
			ObsidianID: "obs-new",
			CreatedAt:  time.Now(),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count, "should process 2 cards (1 update + 1 insert)")
}
