package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// openMigratedDB opens an in-memory database with the study schema applied.
//
// In-memory SQLite is private to the connection that created it, which is why
// platform/db pins the pool to a single connection for memory DSNs: every
// repository here must see the same schema.
func openMigratedDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx := t.Context()
	sqlDB, err := db.Open(ctx, ":memory:")
	require.NoError(t, err, "failed to open in-memory test database")
	require.NoError(t, db.Migrate(ctx, sqlDB, sqlite.Migrations()), "failed to apply study migrations")

	return sqlDB
}

func openTestDB(t *testing.T) (domain.TopicRepository, domain.ConceptRepository, domain.FlashcardRepository, func()) {
	t.Helper()

	sqlDB := openMigratedDB(t)

	return sqlite.NewTopicRepository(sqlDB),
		sqlite.NewConceptRepository(sqlDB),
		sqlite.NewFlashcardRepository(sqlDB),
		func() { sqlDB.Close() }
}

// openTestDBFull returns all six repositories for testing the full RAG+FSRS infrastructure.
func openTestDBFull(t *testing.T) (
	domain.TopicRepository, domain.ConceptRepository, domain.FlashcardRepository,
	domain.ResourceRepository, domain.CardStateRepository, domain.ReviewLogRepository,
	func(),
) {
	t.Helper()

	sqlDB := openMigratedDB(t)

	return sqlite.NewTopicRepository(sqlDB),
		sqlite.NewConceptRepository(sqlDB),
		sqlite.NewFlashcardRepository(sqlDB),
		sqlite.NewResourceRepository(sqlDB),
		sqlite.NewCardStateRepository(sqlDB),
		sqlite.NewReviewLogRepository(sqlDB),
		func() { sqlDB.Close() }
}

// seedTopic inserts a topic with a unique name, for tests that need a real
// parent row to satisfy a foreign key.
func seedTopic(t *testing.T, ctx context.Context, topicRepo domain.TopicRepository) *domain.Topic {
	t.Helper()

	topic := &domain.Topic{
		ID:        uuid.New().String(),
		Name:      "seed-" + uuid.New().String(),
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, topicRepo.Create(ctx, topic))

	return topic
}

// seedFlashcard creates the topic -> concept -> flashcard chain a card_states or
// review_logs row must reference, and returns the flashcard ID.
//
// Foreign keys are enforced (see platform/db). A test can no longer insert a
// card_state or review_log pointing at an invented flashcard ID: the insert is
// rejected, which is the point.
func seedFlashcard(
	t *testing.T,
	ctx context.Context,
	topicRepo domain.TopicRepository,
	conceptRepo domain.ConceptRepository,
	flashcardRepo domain.FlashcardRepository,
) string {
	t.Helper()

	topic := seedTopic(t, ctx, topicRepo)

	concept := &domain.Concept{
		ID:        uuid.New().String(),
		TopicID:   topic.ID,
		Title:     "Seed Concept",
		FilePath:  "seed/" + uuid.New().String() + ".md",
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, conceptRepo.Create(ctx, concept))

	obsidianID := "seed-obs-" + uuid.New().String()
	_, err := flashcardRepo.UpsertByObsidianID(ctx, concept.ID, []domain.Flashcard{{
		ID:         uuid.New().String(),
		ConceptID:  concept.ID,
		Question:   "seed question",
		Answer:     "seed answer",
		ObsidianID: obsidianID,
		CreatedAt:  time.Now().UTC(),
	}})
	require.NoError(t, err)

	card, err := flashcardRepo.FindByObsidianID(ctx, obsidianID)
	require.NoError(t, err)

	return card.ID
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
	assert.ErrorIs(t, err, apperror.ErrConflict)
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
	assert.ErrorIs(t, err, apperror.ErrNotFound)
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
	assert.ErrorIs(t, err, apperror.ErrConflict)
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

// --- ResourceRepository tests ---

func TestResourceRepository_FindBySourceURI_NotFound(t *testing.T) {
	_, _, _, resourceRepo, _, _, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	res, err := resourceRepo.FindBySourceURI(ctx, "obsidian://nonexistent.md")
	require.NoError(t, err)
	assert.Nil(t, res, "FindBySourceURI should return nil,nil when not found")
}

func TestResourceRepository_FindBySourceURI_Found(t *testing.T) {
	topicRepo, _, _, resourceRepo, _, _, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	topic := &domain.Topic{
		ID:        uuid.New().String(),
		Name:      "DDD",
		CreatedAt: time.Now().UTC(),
	}
	err := topicRepo.Create(ctx, topic)
	require.NoError(t, err)

	now := time.Now().UTC()
	res := &domain.Resource{
		ID:        uuid.New().String(),
		TopicID:   topic.ID,
		Title:     "Clean Architecture",
		Type:      domain.ResourceTypeBook,
		SourceURI: "obsidian://clean-arch.md",
		CreatedAt: now,
	}
	err = resourceRepo.Create(ctx, res)
	require.NoError(t, err)

	found, err := resourceRepo.FindBySourceURI(ctx, "obsidian://clean-arch.md")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, res.ID, found.ID)
	assert.Equal(t, "Clean Architecture", found.Title)
	assert.Equal(t, domain.ResourceTypeBook, found.Type)
}

func TestResourceRepository_Create_DuplicateSourceURI(t *testing.T) {
	topicRepo, _, _, resourceRepo, _, _, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	topic := &domain.Topic{
		ID:        uuid.New().String(),
		Name:      "DDD",
		CreatedAt: time.Now().UTC(),
	}
	err := topicRepo.Create(ctx, topic)
	require.NoError(t, err)

	first := &domain.Resource{
		ID:        uuid.New().String(),
		TopicID:   topic.ID,
		Title:     "Book A",
		Type:      domain.ResourceTypeBook,
		SourceURI: "obsidian://dup.md",
		CreatedAt: time.Now().UTC(),
	}
	err = resourceRepo.Create(ctx, first)
	require.NoError(t, err)

	second := &domain.Resource{
		ID:        uuid.New().String(),
		TopicID:   topic.ID,
		Title:     "Book B",
		Type:      domain.ResourceTypeBook,
		SourceURI: "obsidian://dup.md",
		CreatedAt: time.Now().UTC(),
	}
	err = resourceRepo.Create(ctx, second)
	assert.ErrorIs(t, err, apperror.ErrConflict)
}

func TestCardStateRepository_Create_FindByFlashcardID(t *testing.T) {
	topicRepo, conceptRepo, flashcardRepo, _, cardStateRepo, _, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().UTC()
	future := now.Add(24 * time.Hour)
	flashcardID := seedFlashcard(t, ctx, topicRepo, conceptRepo, flashcardRepo)

	cs := &domain.CardState{
		ID:          uuid.New().String(),
		FlashcardID: flashcardID,
		Stability:   2.5,
		Difficulty:  0.8,
		NextReview:  future,
		LastReview:  now,
	}
	err := cardStateRepo.Create(ctx, cs)
	require.NoError(t, err)

	found, err := cardStateRepo.FindByFlashcardID(ctx, flashcardID)
	require.NoError(t, err)
	assert.Equal(t, cs.ID, found.ID)
	assert.Equal(t, 2.5, found.Stability)
	assert.Equal(t, 0.8, found.Difficulty)
	assert.WithinDuration(t, future, found.NextReview, time.Second)
	assert.WithinDuration(t, now, found.LastReview, time.Second)
}

func TestCardStateRepository_FindByFlashcardID_NotFound(t *testing.T) {
	_, _, _, _, cardStateRepo, _, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	_, err := cardStateRepo.FindByFlashcardID(ctx, "nonexistent")
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

func TestCardStateRepository_Create_DuplicateFlashcardID(t *testing.T) {
	topicRepo, conceptRepo, flashcardRepo, _, cardStateRepo, _, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().UTC()
	flashcardID := seedFlashcard(t, ctx, topicRepo, conceptRepo, flashcardRepo)

	first := &domain.CardState{
		ID:          uuid.New().String(),
		FlashcardID: flashcardID,
		NextReview:  now,
		LastReview:  now,
	}
	err := cardStateRepo.Create(ctx, first)
	require.NoError(t, err)

	second := &domain.CardState{
		ID:          uuid.New().String(),
		FlashcardID: flashcardID,
		NextReview:  now,
		LastReview:  now,
	}
	err = cardStateRepo.Create(ctx, second)
	assert.ErrorIs(t, err, apperror.ErrConflict)
}

func TestCardStateRepository_Update(t *testing.T) {
	topicRepo, conceptRepo, flashcardRepo, _, cardStateRepo, _, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	flashcardID := seedFlashcard(t, ctx, topicRepo, conceptRepo, flashcardRepo)

	cs := &domain.CardState{
		ID:          uuid.New().String(),
		FlashcardID: flashcardID,
		Stability:   1.0,
		Difficulty:  0.5,
		NextReview:  time.Now().UTC(),
		LastReview:  time.Now().UTC(),
	}
	err := cardStateRepo.Create(ctx, cs)
	require.NoError(t, err)

	newNext := time.Now().UTC().Add(72 * time.Hour)
	cs.Stability = 2.0
	cs.Difficulty = 0.3
	cs.NextReview = newNext
	cs.LastReview = time.Now().UTC()
	err = cardStateRepo.Update(ctx, cs)
	require.NoError(t, err)

	found, err := cardStateRepo.FindByFlashcardID(ctx, flashcardID)
	require.NoError(t, err)
	assert.Equal(t, 2.0, found.Stability)
	assert.Equal(t, 0.3, found.Difficulty)
	assert.WithinDuration(t, newNext, found.NextReview, time.Second)
}

func TestCardStateRepository_Update_NotFound(t *testing.T) {
	_, _, _, _, cardStateRepo, _, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	cs := &domain.CardState{
		ID:          "nonexistent-id",
		FlashcardID: "no-such-flashcard",
		NextReview:  time.Now(),
		LastReview:  time.Now(),
	}
	err := cardStateRepo.Update(ctx, cs)
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

// --- ReviewLogRepository tests ---

func TestReviewLogRepository_Create_FindByFlashcardID(t *testing.T) {
	topicRepo, conceptRepo, flashcardRepo, _, _, reviewLogRepo, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().UTC()
	flashcardID := seedFlashcard(t, ctx, topicRepo, conceptRepo, flashcardRepo)

	rl := &domain.ReviewLog{
		ID:          uuid.New().String(),
		FlashcardID: flashcardID,
		Grade:       3,
		DurationMs:  5000,
		CreatedAt:   now,
	}
	err := reviewLogRepo.Create(ctx, rl)
	require.NoError(t, err)

	logs, err := reviewLogRepo.FindByFlashcardID(ctx, flashcardID)
	require.NoError(t, err)
	assert.Len(t, logs, 1)
	assert.Equal(t, rl.ID, logs[0].ID)
	assert.Equal(t, 3, logs[0].Grade)
	assert.Equal(t, 5000, logs[0].DurationMs)
	assert.WithinDuration(t, now, logs[0].CreatedAt, time.Second)
}

func TestReviewLogRepository_FindByFlashcardID_Multiple(t *testing.T) {
	topicRepo, conceptRepo, flashcardRepo, _, _, reviewLogRepo, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	flashcardID := seedFlashcard(t, ctx, topicRepo, conceptRepo, flashcardRepo)

	for i := 1; i <= 3; i++ {
		rl := &domain.ReviewLog{
			ID:          uuid.New().String(),
			FlashcardID: flashcardID,
			Grade:       i,
			DurationMs:  i * 1000,
			CreatedAt:   time.Now().UTC().Add(time.Duration(i) * time.Second),
		}
		err := reviewLogRepo.Create(ctx, rl)
		require.NoError(t, err)
	}

	logs, err := reviewLogRepo.FindByFlashcardID(ctx, flashcardID)
	require.NoError(t, err)
	assert.Len(t, logs, 3)
	// Should be ordered by created_at ASC
	assert.Equal(t, 1, logs[0].Grade)
	assert.Equal(t, 2, logs[1].Grade)
	assert.Equal(t, 3, logs[2].Grade)
}

func TestReviewLogRepository_FindByFlashcardID_Empty(t *testing.T) {
	_, _, _, _, _, reviewLogRepo, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	logs, err := reviewLogRepo.FindByFlashcardID(ctx, "no-logs")
	require.NoError(t, err)
	assert.Empty(t, logs)
}

// --- FlashcardRepository FindDueWithContext tests ---

func TestFlashcardRepository_FindDueWithContext_HasDueCards(t *testing.T) {
	topicRepo, conceptRepo, flashcardRepo, _, cardStateRepo, _, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	// Setup topic + concept + flashcard
	topic := &domain.Topic{ID: uuid.New().String(), Name: "Math", CreatedAt: time.Now().UTC()}
	err := topicRepo.Create(ctx, topic)
	require.NoError(t, err)

	concept := &domain.Concept{ID: uuid.New().String(), TopicID: topic.ID, Title: "Algebra", FilePath: "math/algebra.md", CreatedAt: time.Now().UTC()}
	err = conceptRepo.Create(ctx, concept)
	require.NoError(t, err)

	// Upsert two flashcards — one due, one not
	count, err := flashcardRepo.UpsertByObsidianID(ctx, concept.ID, []domain.Flashcard{
		{ConceptID: concept.ID, Question: "2+2?", Answer: "4", ObsidianID: "obs-due", CreatedAt: time.Now()},
		{ConceptID: concept.ID, Question: "3+3?", Answer: "6", ObsidianID: "obs-future", CreatedAt: time.Now()},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Get real IDs after upsert (UpsertByObsidianID generates new UUIDs)
	dueCard, err := flashcardRepo.FindByObsidianID(ctx, "obs-due")
	require.NoError(t, err)
	futureCard, err := flashcardRepo.FindByObsidianID(ctx, "obs-future")
	require.NoError(t, err)

	now := time.Now().UTC()
	// dueCard: NextReview = 1 hour ago (due)
	err = cardStateRepo.Create(ctx, &domain.CardState{
		ID: uuid.New().String(), FlashcardID: dueCard.ID,
		Stability: 1.0, Difficulty: 0.5,
		NextReview: now.Add(-1 * time.Hour), LastReview: now.Add(-24 * time.Hour),
	})
	require.NoError(t, err)

	// futureCard: NextReview = 24 hours from now (not due)
	err = cardStateRepo.Create(ctx, &domain.CardState{
		ID: uuid.New().String(), FlashcardID: futureCard.ID,
		Stability: 1.0, Difficulty: 0.5,
		NextReview: now.Add(24 * time.Hour), LastReview: now,
	})
	require.NoError(t, err)

	results, err := flashcardRepo.FindDueWithContext(ctx, now)
	require.NoError(t, err)
	assert.Len(t, results, 1, "only one card should be due")
	assert.Equal(t, dueCard.ID, results[0].FlashcardID)
	assert.Equal(t, "2+2?", results[0].Front)
	assert.Equal(t, "4", results[0].Back)
	assert.Equal(t, "Algebra", results[0].ConceptTitle)
	assert.Equal(t, "Math", results[0].TopicName)
}

func TestFlashcardRepository_FindDueWithContext_NoDueCards(t *testing.T) {
	_, _, flashcardRepo, _, _, _, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	results, err := flashcardRepo.FindDueWithContext(ctx, time.Now().UTC())
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestFlashcardRepository_FindByObsidianID_Found(t *testing.T) {
	topicRepo, conceptRepo, flashcardRepo, _, _, _, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	topic := seedTopic(t, ctx, topicRepo)
	concept := &domain.Concept{
		ID: uuid.New().String(), TopicID: topic.ID,
		Title: "Test", FilePath: "test/test.md", CreatedAt: time.Now().UTC(),
	}
	err := conceptRepo.Create(ctx, concept)
	require.NoError(t, err)

	_, err = flashcardRepo.UpsertByObsidianID(ctx, concept.ID, []domain.Flashcard{
		{
			ID: uuid.New().String(), ConceptID: concept.ID,
			Question: "Q", Answer: "A", ObsidianID: "obs-find",
			CreatedAt: time.Now(),
		},
	})
	require.NoError(t, err)

	card, err := flashcardRepo.FindByObsidianID(ctx, "obs-find")
	require.NoError(t, err)
	assert.Equal(t, "Q", card.Question)
	assert.Equal(t, "A", card.Answer)
}

func TestFlashcardRepository_FindByObsidianID_NotFound(t *testing.T) {
	_, _, flashcardRepo, _, _, _, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	_, err := flashcardRepo.FindByObsidianID(ctx, "obs-nonexistent")
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

func TestResourceRepository_List_Empty(t *testing.T) {
	_, _, _, resourceRepo, _, _, cleanup := openTestDBFull(t)
	defer cleanup()

	resources, err := resourceRepo.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, resources)
}

func TestResourceRepository_List_CarriesTheTopicNameAndTheNewestFirst(t *testing.T) {
	topicRepo, _, _, resourceRepo, _, _, cleanup := openTestDBFull(t)
	defer cleanup()
	ctx := context.Background()

	ddd := &domain.Topic{ID: uuid.New().String(), Name: "DDD", CreatedAt: time.Now().UTC()}
	psych := &domain.Topic{ID: uuid.New().String(), Name: "Psicología", CreatedAt: time.Now().UTC()}
	require.NoError(t, topicRepo.Create(ctx, ddd))
	require.NoError(t, topicRepo.Create(ctx, psych))

	// The tie-break case is deliberate: two rows share an instant, so the id is
	// the only thing that can keep the order stable between two reads.
	newest := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	middle := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	older := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	rows := []*domain.Resource{
		{ID: "r-older", TopicID: ddd.ID, Title: "Clean Architecture", Type: domain.ResourceTypeBook, SourceURI: "obsidian://ca.md", CreatedAt: older},
		{ID: "r-newest", TopicID: psych.ID, Title: "Atención", Type: domain.ResourceTypeNote, SourceURI: "obsidian://atencion.md", CreatedAt: newest},
		{ID: "r-middle-a", TopicID: ddd.ID, Title: "Paper A", Type: domain.ResourceTypeArticle, SourceURI: "obsidian://a.md", CreatedAt: middle},
		{ID: "r-middle-b", TopicID: ddd.ID, Title: "Video B", Type: domain.ResourceTypeVideo, SourceURI: "obsidian://b.md", CreatedAt: middle},
	}
	for _, row := range rows {
		require.NoError(t, resourceRepo.Create(ctx, row))
	}

	found, err := resourceRepo.List(ctx)
	require.NoError(t, err)
	require.Len(t, found, 4)

	assert.Equal(t, []string{"r-newest", "r-middle-a", "r-middle-b", "r-older"},
		[]string{found[0].ID, found[1].ID, found[2].ID, found[3].ID})
	assert.Equal(t, "Psicología", found[0].TopicName)
	assert.Equal(t, domain.ResourceTypeNote, found[0].Type)
	assert.Equal(t, "obsidian://atencion.md", found[0].SourceURI)
	assert.WithinDuration(t, newest, found[0].CreatedAt, time.Second)
}
