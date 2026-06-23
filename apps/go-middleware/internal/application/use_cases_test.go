package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

func (m *MockTopicRepository) Create(ctx context.Context, topic *domain.Topic) error {
	args := m.Called(ctx, topic)
	return args.Error(0)
}

func (m *MockTopicRepository) FindByID(ctx context.Context, id string) (*domain.Topic, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Topic), args.Error(1)
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

func (m *MockConceptRepository) Create(ctx context.Context, concept *domain.Concept) error {
	args := m.Called(ctx, concept)
	return args.Error(0)
}

// MockFlashcardRepository mocks domain.FlashcardRepository.
type MockFlashcardRepository struct {
	mock.Mock
}

func (m *MockFlashcardRepository) UpsertByObsidianID(ctx context.Context, conceptID string, cards []domain.Flashcard) (int, error) {
	args := m.Called(ctx, conceptID, cards)
	return args.Int(0), args.Error(1)
}

func (m *MockFlashcardRepository) FindByObsidianID(ctx context.Context, obsidianID string) (*domain.Flashcard, error) {
	args := m.Called(ctx, obsidianID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Flashcard), args.Error(1)
}

func (m *MockFlashcardRepository) FindDueWithContext(ctx context.Context, now time.Time) ([]domain.DueCardResult, error) {
	args := m.Called(ctx, now)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.DueCardResult), args.Error(1)
}

// MockResourceRepository mocks domain.ResourceRepository.
type MockResourceRepository struct {
	mock.Mock
}

func (m *MockResourceRepository) FindBySourceURI(ctx context.Context, sourceURI string) (*domain.Resource, error) {
	args := m.Called(ctx, sourceURI)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Resource), args.Error(1)
}

func (m *MockResourceRepository) Create(ctx context.Context, resource *domain.Resource) error {
	args := m.Called(ctx, resource)
	return args.Error(0)
}

func (m *MockResourceRepository) UpdateDifyDocumentID(ctx context.Context, id, difyDocumentID string) error {
	args := m.Called(ctx, id, difyDocumentID)
	return args.Error(0)
}

// MockCardStateRepository mocks domain.CardStateRepository.
type MockCardStateRepository struct {
	mock.Mock
}

func (m *MockCardStateRepository) Create(ctx context.Context, state *domain.CardState) error {
	args := m.Called(ctx, state)
	return args.Error(0)
}

func (m *MockCardStateRepository) FindByFlashcardID(ctx context.Context, flashcardID string) (*domain.CardState, error) {
	args := m.Called(ctx, flashcardID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.CardState), args.Error(1)
}

func (m *MockCardStateRepository) Update(ctx context.Context, state *domain.CardState) error {
	args := m.Called(ctx, state)
	return args.Error(0)
}

// MockReviewLogRepository mocks domain.ReviewLogRepository.
type MockReviewLogRepository struct {
	mock.Mock
}

func (m *MockReviewLogRepository) Create(ctx context.Context, log *domain.ReviewLog) error {
	args := m.Called(ctx, log)
	return args.Error(0)
}

func (m *MockReviewLogRepository) FindByFlashcardID(ctx context.Context, flashcardID string) ([]domain.ReviewLog, error) {
	args := m.Called(ctx, flashcardID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.ReviewLog), args.Error(1)
}

// MockFSRSAlgorithm mocks domain.FSRSAlgorithm.
type MockFSRSAlgorithm struct {
	mock.Mock
}

func (m *MockFSRSAlgorithm) CalculateNextState(current domain.CardState, grade int) domain.CardState {
	args := m.Called(current, grade)
	return args.Get(0).(domain.CardState)
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
	cardStateRepo := new(MockCardStateRepository)
	uc := application.NewSyncFlashcardsUseCase(conceptRepo, flashcardRepo, cardStateRepo)

	ctx := context.Background()

	concept := &domain.Concept{ID: "concept-uuid"}
	conceptRepo.On("UpsertByPath", ctx, "concept-uuid", "", "").Maybe()

	conceptRepo.On("UpsertByPath", ctx, mock.Anything, mock.Anything, mock.Anything).Return(concept, nil)
	flashcardRepo.On("UpsertByObsidianID", ctx, "concept-uuid", mock.Anything).Return(2, nil)

	cardA := &domain.Flashcard{ID: "f-a", ObsidianID: "a"}
	cardB := &domain.Flashcard{ID: "f-b", ObsidianID: "b"}
	flashcardRepo.On("FindByObsidianID", ctx, "a").Return(cardA, nil)
	flashcardRepo.On("FindByObsidianID", ctx, "b").Return(cardB, nil)
	cardStateRepo.On("FindByFlashcardID", ctx, "f-a").Return(nil, domain.ErrNotFound)
	cardStateRepo.On("FindByFlashcardID", ctx, "f-b").Return(nil, domain.ErrNotFound)
	cardStateRepo.On("Create", ctx, mock.AnythingOfType("*domain.CardState")).Return(nil)
	cardStateRepo.On("Create", ctx, mock.AnythingOfType("*domain.CardState")).Return(nil)

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
	cardStateRepo := new(MockCardStateRepository)
	uc := application.NewSyncFlashcardsUseCase(conceptRepo, flashcardRepo, cardStateRepo)

	ctx := context.Background()
	flashcardRepo.On("UpsertByObsidianID", ctx, "concept-uuid", mock.Anything).Return(2, nil)

	existingCard := &domain.Flashcard{ID: "f-existing", ObsidianID: "existing"}
	newCard := &domain.Flashcard{ID: "f-new", ObsidianID: "new-one"}
	flashcardRepo.On("FindByObsidianID", ctx, "existing").Return(existingCard, nil)
	flashcardRepo.On("FindByObsidianID", ctx, "new-one").Return(newCard, nil)

	existingState := &domain.CardState{ID: "cs-existing", FlashcardID: "f-existing"}
	cardStateRepo.On("FindByFlashcardID", ctx, "f-existing").Return(existingState, nil)
	cardStateRepo.On("FindByFlashcardID", ctx, "f-new").Return(nil, domain.ErrNotFound)
	cardStateRepo.On("Create", ctx, mock.AnythingOfType("*domain.CardState")).Return(nil)

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

// --- CreateTopicUseCase tests ---

func TestCreateTopicUseCase_Success(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	uc := application.NewCreateTopicUseCase(topicRepo)

	ctx := context.Background()
	topicRepo.On("Create", ctx, mock.MatchedBy(func(t *domain.Topic) bool {
		return t.Name == "DDD" && t.ID != "" && !t.CreatedAt.IsZero()
	})).Return(nil)

	topic, err := uc.Execute(ctx, "DDD")
	require.NoError(t, err)
	assert.NotEmpty(t, topic.ID)
	assert.Equal(t, "DDD", topic.Name)
	assert.False(t, topic.CreatedAt.IsZero())
	topicRepo.AssertExpectations(t)
}

func TestCreateTopicUseCase_EmptyName(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	uc := application.NewCreateTopicUseCase(topicRepo)

	ctx := context.Background()
	_, err := uc.Execute(ctx, "")
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestCreateTopicUseCase_Duplicate(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	uc := application.NewCreateTopicUseCase(topicRepo)

	ctx := context.Background()
	topicRepo.On("Create", ctx, mock.AnythingOfType("*domain.Topic")).Return(domain.ErrConflict)

	_, err := uc.Execute(ctx, "Physics")
	assert.ErrorIs(t, err, domain.ErrConflict)
}

// --- CreateConceptUseCase tests ---

func TestCreateConceptUseCase_Success(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	conceptRepo := new(MockConceptRepository)
	uc := application.NewCreateConceptUseCase(topicRepo, conceptRepo)

	ctx := context.Background()

	topic := &domain.Topic{ID: "t1", Name: "DDD"}
	topicRepo.On("FindByID", ctx, "t1").Return(topic, nil)

	conceptRepo.On("Create", ctx, mock.MatchedBy(func(c *domain.Concept) bool {
		return c.TopicID == "t1" && c.Title == "Aggregates" &&
			c.FilePath == "manual/DDD/Aggregates.md" && c.ID != ""
	})).Return(nil)

	concept, err := uc.Execute(ctx, "t1", "Aggregates")
	require.NoError(t, err)
	assert.NotEmpty(t, concept.ID)
	assert.Equal(t, "t1", concept.TopicID)
	assert.Equal(t, "Aggregates", concept.Title)
	assert.Equal(t, "manual/DDD/Aggregates.md", concept.FilePath)
	assert.False(t, concept.CreatedAt.IsZero())
}

func TestCreateConceptUseCase_EmptyTitle(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	conceptRepo := new(MockConceptRepository)
	uc := application.NewCreateConceptUseCase(topicRepo, conceptRepo)

	ctx := context.Background()
	_, err := uc.Execute(ctx, "t1", "")
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestCreateConceptUseCase_TopicNotFound(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	conceptRepo := new(MockConceptRepository)
	uc := application.NewCreateConceptUseCase(topicRepo, conceptRepo)

	ctx := context.Background()
	topicRepo.On("FindByID", ctx, "t99").Return(nil, domain.ErrNotFound)

	_, err := uc.Execute(ctx, "t99", "Some Title")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCreateConceptUseCase_Duplicate(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	conceptRepo := new(MockConceptRepository)
	uc := application.NewCreateConceptUseCase(topicRepo, conceptRepo)

	ctx := context.Background()

	topic := &domain.Topic{ID: "t1", Name: "DDD"}
	topicRepo.On("FindByID", ctx, "t1").Return(topic, nil)
	conceptRepo.On("Create", ctx, mock.AnythingOfType("*domain.Concept")).Return(domain.ErrConflict)

	_, err := uc.Execute(ctx, "t1", "Aggregates")
	assert.ErrorIs(t, err, domain.ErrConflict)
}

// --- SyncResourceUseCase tests ---

func TestSyncResourceUseCase_NewTopicAndResource(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	resourceRepo := new(MockResourceRepository)
	uc := application.NewSyncResourceUseCase(topicRepo, resourceRepo)

	ctx := context.Background()

	topic := &domain.Topic{ID: "t-ddd", Name: "DDD"}
	topicRepo.On("UpsertByName", ctx, "DDD").Return(topic, nil)

	resourceRepo.On("FindBySourceURI", ctx, "obsidian://ddd.md").Return(nil, nil)
	resourceRepo.On("Create", ctx, mock.MatchedBy(func(r *domain.Resource) bool {
		return r.TopicID == "t-ddd" && r.Title == "DDD Book" &&
			r.Type == domain.ResourceTypeBook && r.SourceURI == "obsidian://ddd.md"
	})).Return(nil)

	id, err := uc.Execute(ctx, "DDD", "DDD Book", "book", "obsidian://ddd.md", "doc-123")
	require.NoError(t, err)
	assert.NotEmpty(t, id)
}

func TestSyncResourceUseCase_ExistingTopic(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	resourceRepo := new(MockResourceRepository)
	uc := application.NewSyncResourceUseCase(topicRepo, resourceRepo)

	ctx := context.Background()

	topic := &domain.Topic{ID: "t-math", Name: "Math"}
	topicRepo.On("UpsertByName", ctx, "Math").Return(topic, nil)

	resourceRepo.On("FindBySourceURI", ctx, "obsidian://math.md").Return(nil, nil)
	resourceRepo.On("Create", ctx, mock.MatchedBy(func(r *domain.Resource) bool {
		return r.TopicID == "t-math"
	})).Return(nil)

	id, err := uc.Execute(ctx, "Math", "Math Notes", "note", "obsidian://math.md", "")
	require.NoError(t, err)
	assert.NotEmpty(t, id)
}

func TestSyncResourceUseCase_UpdateExistingResource(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	resourceRepo := new(MockResourceRepository)
	uc := application.NewSyncResourceUseCase(topicRepo, resourceRepo)

	ctx := context.Background()

	topic := &domain.Topic{ID: "t-ddd", Name: "DDD"}
	topicRepo.On("UpsertByName", ctx, "DDD").Return(topic, nil)

	existing := &domain.Resource{ID: "r-existing", TopicID: "t-ddd", SourceURI: "obsidian://ddd.md"}
	resourceRepo.On("FindBySourceURI", ctx, "obsidian://ddd.md").Return(existing, nil)
	resourceRepo.On("UpdateDifyDocumentID", ctx, "r-existing", "doc-new").Return(nil)

	id, err := uc.Execute(ctx, "DDD", "DDD Book", "book", "obsidian://ddd.md", "doc-new")
	require.NoError(t, err)
	assert.Equal(t, "r-existing", id)
}

func TestSyncResourceUseCase_InvalidType(t *testing.T) {
	topicRepo := new(MockTopicRepository)
	resourceRepo := new(MockResourceRepository)
	uc := application.NewSyncResourceUseCase(topicRepo, resourceRepo)

	ctx := context.Background()

	_, err := uc.Execute(ctx, "DDD", "Title", "podcast", "obsidian://x.md", "")
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

// --- GetDueCardsUseCase tests ---

func TestGetDueCardsUseCase_HasDueCards(t *testing.T) {
	flashcardRepo := new(MockFlashcardRepository)
	uc := application.NewGetDueCardsUseCase(flashcardRepo)

	ctx := context.Background()
	now := time.Now().UTC()

	results := []domain.DueCardResult{
		{
			FlashcardID:  "f1",
			Front:        "What is DDD?",
			Back:         "Domain-Driven Design",
			ConceptTitle: "DDD Intro",
			TopicName:    "Architecture",
			NextReview:   now,
		},
	}
	flashcardRepo.On("FindDueWithContext", ctx, mock.AnythingOfType("time.Time")).Return(results, nil)

	cards, err := uc.Execute(ctx)
	require.NoError(t, err)
	assert.Len(t, cards, 1)
	assert.Equal(t, "f1", cards[0].FlashcardID)
	assert.Equal(t, "What is DDD?", cards[0].Front)
	assert.Equal(t, "Domain-Driven Design", cards[0].Back)
	assert.Equal(t, "DDD Intro", cards[0].ConceptTitle)
	assert.Equal(t, "Architecture", cards[0].TopicName)
}

func TestGetDueCardsUseCase_NoDueCards(t *testing.T) {
	flashcardRepo := new(MockFlashcardRepository)
	uc := application.NewGetDueCardsUseCase(flashcardRepo)

	ctx := context.Background()

	flashcardRepo.On("FindDueWithContext", ctx, mock.AnythingOfType("time.Time")).Return([]domain.DueCardResult{}, nil)

	cards, err := uc.Execute(ctx)
	require.NoError(t, err)
	assert.Empty(t, cards)
}

// --- SubmitReviewUseCase tests ---

func TestSubmitReviewUseCase_ValidReview(t *testing.T) {
	cardStateRepo := new(MockCardStateRepository)
	reviewLogRepo := new(MockReviewLogRepository)
	fsrsMock := new(MockFSRSAlgorithm)
	uc := application.NewSubmitReviewUseCase(cardStateRepo, reviewLogRepo, fsrsMock)

	ctx := context.Background()
	now := time.Now().UTC()
	nextReview := now.AddDate(0, 0, 3)

	current := &domain.CardState{
		ID:          "cs-1",
		FlashcardID: "f1",
		Stability:   1.0,
		Difficulty:  0.5,
		NextReview:  now,
		LastReview:  now.AddDate(0, 0, -1),
	}
	cardStateRepo.On("FindByFlashcardID", ctx, "f1").Return(current, nil)

	nextState := domain.CardState{
		ID:          "cs-1",
		FlashcardID: "f1",
		Stability:   1.1,
		Difficulty:  0.5,
		NextReview:  nextReview,
		LastReview:  now,
	}
	fsrsMock.On("CalculateNextState", *current, 3).Return(nextState)

	cardStateRepo.On("Update", ctx, mock.MatchedBy(func(cs *domain.CardState) bool {
		return cs.ID == "cs-1" && cs.Stability == 1.1 && !cs.NextReview.IsZero()
	})).Return(nil)

	reviewLogRepo.On("Create", ctx, mock.MatchedBy(func(rl *domain.ReviewLog) bool {
		return rl.FlashcardID == "f1" && rl.Grade == 3 && rl.DurationMs == 5000
	})).Return(nil)

	result, err := uc.Execute(ctx, "f1", 3, 5000)
	require.NoError(t, err)
	assert.Equal(t, 1.1, result.Stability)
	assert.Equal(t, 0.5, result.Difficulty)
	assert.False(t, result.NextReview.IsZero())
}

func TestSubmitReviewUseCase_NoCardState(t *testing.T) {
	cardStateRepo := new(MockCardStateRepository)
	reviewLogRepo := new(MockReviewLogRepository)
	fsrsMock := new(MockFSRSAlgorithm)
	uc := application.NewSubmitReviewUseCase(cardStateRepo, reviewLogRepo, fsrsMock)

	ctx := context.Background()

	cardStateRepo.On("FindByFlashcardID", ctx, "f-missing").Return(nil, domain.ErrNotFound)

	_, err := uc.Execute(ctx, "f-missing", 3, 2000)
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestSubmitReviewUseCase_InvalidGrade(t *testing.T) {
	cardStateRepo := new(MockCardStateRepository)
	reviewLogRepo := new(MockReviewLogRepository)
	fsrsMock := new(MockFSRSAlgorithm)
	uc := application.NewSubmitReviewUseCase(cardStateRepo, reviewLogRepo, fsrsMock)

	ctx := context.Background()

	_, err := uc.Execute(ctx, "f1", 0, 1000)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestSubmitReviewUseCase_GradeTooHigh(t *testing.T) {
	cardStateRepo := new(MockCardStateRepository)
	reviewLogRepo := new(MockReviewLogRepository)
	fsrsMock := new(MockFSRSAlgorithm)
	uc := application.NewSubmitReviewUseCase(cardStateRepo, reviewLogRepo, fsrsMock)

	ctx := context.Background()

	_, err := uc.Execute(ctx, "f1", 5, 1000)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}
