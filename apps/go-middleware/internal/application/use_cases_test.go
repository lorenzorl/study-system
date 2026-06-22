package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/application"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// MockTopicRepository mocks domain.TopicRepository.
type MockTopicRepository struct {
	mock.Mock
}

func (m *MockTopicRepository) UpsertByName(ctx context.Context, name string) (*domain.Topic, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Topic), args.Error(1)
}

func (m *MockTopicRepository) ListAll(ctx context.Context) ([]domain.Topic, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Topic), args.Error(1)
}

// MockConceptRepository mocks domain.ConceptRepository.
type MockConceptRepository struct {
	mock.Mock
}

func (m *MockConceptRepository) UpsertByPath(ctx context.Context, topicID, title, filePath string) (*domain.Concept, error) {
	args := m.Called(ctx, topicID, title, filePath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Concept), args.Error(1)
}

func (m *MockConceptRepository) ListByTopicID(ctx context.Context, topicID string) ([]domain.Concept, error) {
	args := m.Called(ctx, topicID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Concept), args.Error(1)
}

// MockFlashcardRepository mocks domain.FlashcardRepository.
type MockFlashcardRepository struct {
	mock.Mock
}

func (m *MockFlashcardRepository) UpsertByObsidianID(ctx context.Context, conceptID string, cards []domain.Flashcard) (int, error) {
	args := m.Called(ctx, conceptID, cards)
	return args.Int(0), args.Error(1)
}

func TestSyncConceptUseCase_AutoCreateTopic(t *testing.T) {
	// R5: SyncConcept auto-creates topic and concept
	topicRepo := new(MockTopicRepository)
	conceptRepo := new(MockConceptRepository)
	uc := application.NewSyncConceptUseCase(topicRepo, conceptRepo)

	ctx := context.Background()

	topic := &domain.Topic{ID: "topic-uuid", Name: "Science"}
	topicRepo.On("UpsertByName", ctx, "Science").Return(topic, nil)

	concept := &domain.Concept{ID: "concept-uuid", TopicID: "topic-uuid", Title: "Biology", FilePath: "science/biology.md"}
	conceptRepo.On("UpsertByPath", ctx, "topic-uuid", "Biology", "science/biology.md").Return(concept, nil)

	result, err := uc.Execute(ctx, "Science", "Biology", "science/biology.md")
	require.NoError(t, err)
	assert.Equal(t, "concept-uuid", result)
	topicRepo.AssertExpectations(t)
	conceptRepo.AssertExpectations(t)
}

func TestSyncConceptUseCase_ReuseExistingTopic(t *testing.T) {
	// R5: reuse existing topic
	topicRepo := new(MockTopicRepository)
	conceptRepo := new(MockConceptRepository)
	uc := application.NewSyncConceptUseCase(topicRepo, conceptRepo)

	ctx := context.Background()

	topic := &domain.Topic{ID: "existing-topic", Name: "Math"}
	topicRepo.On("UpsertByName", ctx, "Math").Return(topic, nil)

	concept := &domain.Concept{ID: "new-concept", TopicID: "existing-topic", Title: "Physics", FilePath: "math/physics.md"}
	conceptRepo.On("UpsertByPath", ctx, "existing-topic", "Physics", "math/physics.md").Return(concept, nil)

	result, err := uc.Execute(ctx, "Math", "Physics", "math/physics.md")
	require.NoError(t, err)
	assert.Equal(t, "new-concept", result)
}

func TestSyncConceptUseCase_TopicError(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	conceptRepo := new(MockConceptRepository)
	uc := application.NewSyncConceptUseCase(topicRepo, conceptRepo)

	ctx := context.Background()

	topicRepo.On("UpsertByName", ctx, "Science").Return(nil, errors.New("db error"))

	_, err := uc.Execute(ctx, "Science", "Biology", "science/biology.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upsert topic")
	topicRepo.AssertExpectations(t)
}

func TestSyncConceptUseCase_ConceptError(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	conceptRepo := new(MockConceptRepository)
	uc := application.NewSyncConceptUseCase(topicRepo, conceptRepo)

	ctx := context.Background()

	topic := &domain.Topic{ID: "topic-uuid", Name: "Science"}
	topicRepo.On("UpsertByName", ctx, "Science").Return(topic, nil)
	conceptRepo.On("UpsertByPath", ctx, "topic-uuid", "Biology", "science/biology.md").Return(nil, errors.New("db error"))

	_, err := uc.Execute(ctx, "Science", "Biology", "science/biology.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upsert concept")
}

func TestSyncFlashcardsUseCase_BatchCreate(t *testing.T) {
	// R6: batch create
	conceptRepo := new(MockConceptRepository)
	flashcardRepo := new(MockFlashcardRepository)
	uc := application.NewSyncFlashcardsUseCase(conceptRepo, flashcardRepo)

	ctx := context.Background()

	concept := &domain.Concept{ID: "concept-uuid"}
	conceptRepo.On("UpsertByPath", ctx, "concept-uuid", "", "").Maybe()

	conceptRepo.On("UpsertByPath", ctx, mock.Anything, mock.Anything, mock.Anything).Return(concept, nil)
	flashcardRepo.On("UpsertByObsidianID", ctx, "concept-uuid", mock.Anything).Return(2, nil)

	count, err := uc.Execute(ctx, "concept-uuid", []domain.Flashcard{
		{ObsidianID: "a", Question: "Q1", Answer: "A1"},
		{ObsidianID: "b", Question: "Q2", Answer: "A2"},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestSyncFlashcardsUseCase_MixedInsertUpdate(t *testing.T) {
	flashcardRepo := new(MockFlashcardRepository)
	conceptRepo := new(MockConceptRepository)
	uc := application.NewSyncFlashcardsUseCase(conceptRepo, flashcardRepo)

	ctx := context.Background()
	flashcardRepo.On("UpsertByObsidianID", ctx, "concept-uuid", mock.Anything).Return(2, nil)

	count, err := uc.Execute(ctx, "concept-uuid", []domain.Flashcard{
		{ObsidianID: "existing", Question: "new q", Answer: "new a"},
		{ObsidianID: "new-one", Question: "Q2", Answer: "A2"},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestListConceptsUseCase_ReturnsTree(t *testing.T) {
	// R7: topic→concept tree
	topicRepo := new(MockTopicRepository)
	conceptRepo := new(MockConceptRepository)
	uc := application.NewListConceptsUseCase(topicRepo, conceptRepo)

	ctx := context.Background()

	topics := []domain.Topic{
		{ID: "t1", Name: "Math"},
		{ID: "t2", Name: "Science"},
	}
	topicRepo.On("ListAll", ctx).Return(topics, nil)

	conceptRepo.On("ListByTopicID", ctx, "t1").Return([]domain.Concept{
		{ID: "c1", TopicID: "t1", Title: "Algebra", FilePath: "math/algebra.md"},
		{ID: "c2", TopicID: "t1", Title: "Geometry", FilePath: "math/geometry.md"},
	}, nil)
	conceptRepo.On("ListByTopicID", ctx, "t2").Return([]domain.Concept{
		{ID: "c3", TopicID: "t2", Title: "Biology", FilePath: "science/biology.md"},
	}, nil)

	result, err := uc.Execute(ctx)
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "Math", result[0].Name)
	assert.Len(t, result[0].Concepts, 2)
	assert.Equal(t, "Algebra", result[0].Concepts[0].Title)
	assert.Equal(t, "Geometry", result[0].Concepts[1].Title)
	assert.Equal(t, "Science", result[1].Name)
	assert.Len(t, result[1].Concepts, 1)
	assert.Equal(t, "Biology", result[1].Concepts[0].Title)
}

func TestListConceptsUseCase_Empty(t *testing.T) {
	// R7: empty database
	topicRepo := new(MockTopicRepository)
	conceptRepo := new(MockConceptRepository)
	uc := application.NewListConceptsUseCase(topicRepo, conceptRepo)

	ctx := context.Background()
	topicRepo.On("ListAll", ctx).Return([]domain.Topic{}, nil)

	result, err := uc.Execute(ctx)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestListConceptsUseCase_TopicsError(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	conceptRepo := new(MockConceptRepository)
	uc := application.NewListConceptsUseCase(topicRepo, conceptRepo)

	ctx := context.Background()
	topicRepo.On("ListAll", ctx).Return(nil, errors.New("db error"))

	_, err := uc.Execute(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list topics")
}
