package app_test

// Accepting a gap into a card: the server names the card, writes it through the
// existing flashcards path, and claims the gap last (S6, S10).

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	studysqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// acceptGapOrder records the writes of one accept in the order they happened.
//
// The order is the rule and not an implementation detail: the gap's state is a claim
// that a card exists, so a claim written first could be true of nothing.
type acceptGapOrder struct {
	steps []string
}

func (o *acceptGapOrder) add(step string) {
	o.steps = append(o.steps, step)
}

// fakeAcceptGaps is the gap repository as the accept route uses it: one read by id,
// one state write.
type fakeAcceptGaps struct {
	order   *acceptGapOrder
	gaps    []*domain.RangeGap
	updates []domain.RangeGapState
	err     error
}

func (f *fakeAcceptGaps) FindByID(ctx context.Context, gapID string) (*domain.RangeGap, error) {
	f.order.add("gap")
	if f.err != nil {
		return nil, f.err
	}
	for _, gap := range f.gaps {
		if gap.ID == gapID {
			found := *gap
			return &found, nil
		}
	}
	return nil, apperror.ErrNotFound
}

func (f *fakeAcceptGaps) ListByExplanationID(
	ctx context.Context,
	explanationID string,
) ([]domain.RangeGap, error) {
	gaps := make([]domain.RangeGap, 0, len(f.gaps))
	for _, gap := range f.gaps {
		gaps = append(gaps, *gap)
	}
	return gaps, nil
}

func (f *fakeAcceptGaps) UpdateState(
	ctx context.Context,
	gapID string,
	state domain.RangeGapState,
) (*domain.RangeGap, error) {
	f.order.add("carded")
	if f.err != nil {
		return nil, f.err
	}
	for _, gap := range f.gaps {
		if gap.ID == gapID {
			gap.State = state
			f.updates = append(f.updates, state)
			stored := *gap
			return &stored, nil
		}
	}
	return nil, apperror.ErrNotFound
}

// fakeAcceptTopics is the topic upsert the naming rule needs, remembering the name
// it was given.
type fakeAcceptTopics struct {
	order *acceptGapOrder
	name  string
	topic *domain.Topic
	err   error
}

func (f *fakeAcceptTopics) UpsertByName(ctx context.Context, name string) (*domain.Topic, error) {
	f.order.add("topic")
	f.name = name
	if f.err != nil {
		return nil, f.err
	}
	if f.topic == nil {
		f.topic = &domain.Topic{ID: "topic-1", Name: name}
	}
	return f.topic, nil
}

func (f *fakeAcceptTopics) ListAll(ctx context.Context) ([]domain.Topic, error) {
	return nil, nil
}

func (f *fakeAcceptTopics) Create(ctx context.Context, topic *domain.Topic) error {
	return nil
}

func (f *fakeAcceptTopics) FindByID(ctx context.Context, id string) (*domain.Topic, error) {
	return nil, apperror.ErrNotFound
}

// fakeAcceptConcepts is the concept upsert, remembering the path that makes a second
// accept reuse the concept.
type fakeAcceptConcepts struct {
	order    *acceptGapOrder
	topicID  string
	title    string
	filePath string
	concept  *domain.Concept
	err      error
}

func (f *fakeAcceptConcepts) UpsertByPath(
	ctx context.Context,
	topicID, title, filePath string,
) (*domain.Concept, error) {
	f.order.add("concept")
	f.topicID, f.title, f.filePath = topicID, title, filePath
	if f.err != nil {
		return nil, f.err
	}
	if f.concept == nil {
		f.concept = &domain.Concept{
			ID: "concept-1", TopicID: topicID, Title: title, FilePath: filePath,
		}
	}
	return f.concept, nil
}

func (f *fakeAcceptConcepts) ListByTopicID(ctx context.Context, topicID string) ([]domain.Concept, error) {
	return nil, nil
}

func (f *fakeAcceptConcepts) Create(ctx context.Context, concept *domain.Concept) error {
	return nil
}

// fakeCardWriter stands in for the existing flashcards path. It records the write
// and can fail, which is how the test proves the gap is not claimed on a failure.
type fakeCardWriter struct {
	order *acceptGapOrder
	err   error
	calls []cardWrite
}

type cardWrite struct {
	conceptID string
	cards     []domain.Flashcard
}

func (f *fakeCardWriter) Execute(
	ctx context.Context,
	conceptID string,
	cards []domain.Flashcard,
) (int, error) {
	f.order.add("card")
	f.calls = append(f.calls, cardWrite{conceptID: conceptID, cards: cards})
	if f.err != nil {
		return 0, f.err
	}
	return len(cards), nil
}

// acceptGapFixture is one accept wired over fakes, with the default material: a
// range of a book whose active plan recommends exactly that range.
type acceptGapFixture struct {
	useCase   *app.AcceptGapUseCase
	order     *acceptGapOrder
	gaps      *fakeAcceptGaps
	topics    *fakeAcceptTopics
	concepts  *fakeAcceptConcepts
	cards     *fakeCardWriter
	plans     *fakeStudyPlanRepository
	documents *fakeDocuments
}

func newAcceptGapFixture() *acceptGapFixture {
	order := &acceptGapOrder{}

	gaps := &fakeAcceptGaps{
		order: order,
		gaps: []*domain.RangeGap{{
			ID:            "g1",
			ExplanationID: "e1",
			Point:         "the prototype binds model and implementation",
			Page:          18,
			Quote:         "That crude prototype forged the essential link.",
			State:         domain.RangeGapStateOpen,
		}},
	}

	explanations := &fakeExplanationRepo{explanation: &domain.RangeExplanation{
		ID:         "e1",
		DocumentID: "doc-1",
		FromPage:   15,
		ToPage:     23,
		Text:       "mis palabras sobre el capítulo",
	}}

	plans := &fakeStudyPlanRepository{active: &domain.StudyPlan{
		ID:         "plan-1",
		DocumentID: "doc-1",
		Items: []domain.StudyPlanItem{
			{Position: 1, Title: "Chapter 2: Keeping the Model Crude", FromPage: 15, ToPage: 23},
		},
	}}

	topics := &fakeAcceptTopics{order: order}
	concepts := &fakeAcceptConcepts{order: order}
	cards := &fakeCardWriter{order: order}
	documents := &fakeDocuments{titles: map[string]string{"doc-1": "Domain-Driven Design"}}

	return &acceptGapFixture{
		useCase: app.NewAcceptGapUseCase(
			documents, gaps, explanations, plans, topics, concepts, cards,
		),
		order:     order,
		gaps:      gaps,
		topics:    topics,
		concepts:  concepts,
		cards:     cards,
		plans:     plans,
		documents: documents,
	}
}

// S6 and S10: one accept writes the topic, the concept and the card, and claims the
// gap only after the card exists.
func TestAcceptGap_WritesTheCardBeforeItMarksTheGapCarded(t *testing.T) {
	fixture := newAcceptGapFixture()

	result, err := fixture.useCase.Execute(t.Context(), app.AcceptGapInput{
		GapID:    "g1",
		Question: "  ¿Por qué el prototipo ata modelo e implementación?  ",
	})
	require.NoError(t, err)

	// S10: the server names both, and a client never re-derives the name.
	assert.Equal(t, "Domain-Driven Design", fixture.topics.name, "the topic is the document's title")
	assert.Equal(t, "topic-1", fixture.concepts.topicID)
	assert.Equal(t, "Chapter 2: Keeping the Model Crude", fixture.concepts.title)
	assert.Equal(t, "ranges/doc-1/15-23", fixture.concepts.filePath)

	// S6: the front is the reader's question, the back is the book's evidence.
	require.Len(t, fixture.cards.calls, 1, "one accept writes one card")
	assert.Equal(t, "concept-1", fixture.cards.calls[0].conceptID)
	require.Len(t, fixture.cards.calls[0].cards, 1)
	written := fixture.cards.calls[0].cards[0]
	assert.Equal(t, "¿Por qué el prototipo ata modelo e implementación?", written.Question)
	assert.Equal(t, "That crude prototype forged the essential link. (p.18)", written.Answer)
	assert.Empty(t, written.ObsidianID, "the cards path derives the id; the accept route must not invent one")

	assert.Equal(t, domain.RangeGapStateCarded, fixture.gaps.gaps[0].State)

	assert.Equal(t, "g1", result.GapID)
	assert.Equal(t, "¿Por qué el prototipo ata modelo e implementación?", result.Question)
	assert.Equal(t, "That crude prototype forged the essential link. (p.18)", result.Answer)
	assert.Equal(t, "concept-1", result.ConceptID)
	assert.Equal(t, "topic-1", result.TopicID)
	assert.Equal(t, domain.RangeGapStateCarded, result.State)

	assert.Equal(t, []string{"gap", "topic", "concept", "card", "carded"}, fixture.order.steps,
		"the claim is written last, after the card it claims exists")
}

// S6: a failure before the state write leaves the gap open. The card writer is
// reached, so the test knows the failure came after the naming and not before it.
func TestAcceptGap_AFailingCardWriterLeavesTheGapOpen(t *testing.T) {
	fixture := newAcceptGapFixture()
	fixture.cards.err = assert.AnError

	_, err := fixture.useCase.Execute(t.Context(), app.AcceptGapInput{
		GapID:    "g1",
		Question: "¿Por qué el prototipo ata modelo e implementación?",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
	require.Len(t, fixture.cards.calls, 1, "the card write is attempted")
	assert.Empty(t, fixture.gaps.updates, "a failed card write must not mark the gap carded")
	assert.Equal(t, domain.RangeGapStateOpen, fixture.gaps.gaps[0].State)
	assert.NotContains(t, fixture.order.steps, "carded")
}

// S6: something as small as a blank question must not create a topic, a concept or a
// card, and must not claim anything.
func TestAcceptGap_ABlankQuestionWritesNothing(t *testing.T) {
	for _, question := range []string{"", "   ", "\n\t "} {
		t.Run(question, func(t *testing.T) {
			fixture := newAcceptGapFixture()

			_, err := fixture.useCase.Execute(t.Context(), app.AcceptGapInput{
				GapID:    "g1",
				Question: question,
			})

			assert.ErrorIs(t, err, apperror.ErrInvalidInput)
			assert.Empty(t, fixture.topics.name)
			assert.Empty(t, fixture.concepts.filePath)
			assert.Empty(t, fixture.cards.calls)
			assert.Empty(t, fixture.gaps.updates)
			assert.Equal(t, domain.RangeGapStateOpen, fixture.gaps.gaps[0].State)
		})
	}
}

func TestAcceptGap_UnknownGapIsNotFound(t *testing.T) {
	fixture := newAcceptGapFixture()

	_, err := fixture.useCase.Execute(t.Context(), app.AcceptGapInput{
		GapID:    "no-such-gap",
		Question: "¿Qué es un agregado?",
	})

	assert.ErrorIs(t, err, apperror.ErrNotFound)
	assert.Empty(t, fixture.cards.calls, "an unknown gap writes no card")
}

// S6: a gap that already has a card is a conflict, and the refusal comes before any
// write, so accepting twice cannot duplicate a deck entry.
func TestAcceptGap_AnAlreadyCardedGapIsAConflict(t *testing.T) {
	fixture := newAcceptGapFixture()
	fixture.gaps.gaps[0].State = domain.RangeGapStateCarded

	_, err := fixture.useCase.Execute(t.Context(), app.AcceptGapInput{
		GapID:    "g1",
		Question: "¿Qué es un agregado?",
	})

	assert.ErrorIs(t, err, apperror.ErrConflict)
	assert.Empty(t, fixture.topics.name)
	assert.Empty(t, fixture.cards.calls)
	assert.Empty(t, fixture.gaps.updates)
}

// S10: when the explained range is exactly a plan item's range, the concept is the
// item's title — the name the reader already reads in the plan.
func TestAcceptGap_ConceptTitleIsThePlanItemTitleForTheExactRange(t *testing.T) {
	fixture := newAcceptGapFixture()

	_, err := fixture.useCase.Execute(t.Context(), app.AcceptGapInput{
		GapID:    "g1",
		Question: "¿Qué ata el prototipo?",
	})
	require.NoError(t, err)

	assert.Equal(t, "Chapter 2: Keeping the Model Crude", fixture.concepts.title)
}

// S10: without a matching item the concept is named by the pages, and the path stays
// the stable key so a second accept reuses it instead of failing the unique
// constraint.
func TestAcceptGap_ConceptTitleFallsBackToThePageRange(t *testing.T) {
	cases := []struct {
		name string
		plan *domain.StudyPlan
	}{
		{
			name: "an item with another range",
			plan: &domain.StudyPlan{
				ID: "plan-1", DocumentID: "doc-1",
				Items: []domain.StudyPlanItem{
					{Position: 1, Title: "Chapter 1: Crude prototypes", FromPage: 1, ToPage: 14},
				},
			},
		},
		{
			name: "a partially overlapping item is not the range",
			plan: &domain.StudyPlan{
				ID: "plan-1", DocumentID: "doc-1",
				Items: []domain.StudyPlanItem{
					{Position: 1, Title: "Chapter 2: Keeping the Model Crude", FromPage: 15, ToPage: 30},
				},
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newAcceptGapFixture()
			fixture.plans.active = testCase.plan

			_, err := fixture.useCase.Execute(t.Context(), app.AcceptGapInput{
				GapID:    "g1",
				Question: "¿Qué ata el prototipo?",
			})
			require.NoError(t, err)

			assert.Equal(t, "pp. 15-23", fixture.concepts.title)
			assert.Equal(t, "ranges/doc-1/15-23", fixture.concepts.filePath)
		})
	}

	t.Run("no active plan", func(t *testing.T) {
		fixture := newAcceptGapFixture()
		fixture.plans.active = nil

		_, err := fixture.useCase.Execute(t.Context(), app.AcceptGapInput{
			GapID:    "g1",
			Question: "¿Qué ata el prototipo?",
		})
		require.NoError(t, err)

		assert.Equal(t, "pp. 15-23", fixture.concepts.title)
	})
}

// S6 and S10: the second accept of one gap is refused, and the deck still holds one
// card. This runs over the real repositories and the real flashcards path, because
// the claim being tested — the identity is derived, so a retry converges — belongs to
// those two together.
func TestAcceptGap_A_SecondAcceptIsRefusedAndStoresNoSecondCard(t *testing.T) {
	ctx := t.Context()

	sqlDB, err := db.Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.Migrate(ctx, sqlDB, studysqlite.Migrations()))

	explanations := studysqlite.NewRangeExplanationRepository(sqlDB)
	gaps := studysqlite.NewRangeGapRepository(sqlDB)
	topics := studysqlite.NewTopicRepository(sqlDB)
	concepts := studysqlite.NewConceptRepository(sqlDB)
	flashcards := studysqlite.NewFlashcardRepository(sqlDB)
	cardStates := studysqlite.NewCardStateRepository(sqlDB)

	require.NoError(t, explanations.Create(ctx, &domain.RangeExplanation{
		ID:         "explanation-1",
		DocumentID: "doc-1",
		FromPage:   15,
		ToPage:     23,
		Text:       "mis palabras sobre el capítulo",
		CreatedAt:  domain.Now(),
		Gaps: []domain.RangeGap{{
			ID:    "gap-1",
			Point: "the prototype binds model and implementation",
			Page:  18,
			Quote: "That crude prototype forged the essential link.",
			State: domain.RangeGapStateOpen,
		}},
	}))

	useCase := app.NewAcceptGapUseCase(
		&fakeDocuments{titles: map[string]string{"doc-1": "Domain-Driven Design"}},
		gaps,
		explanations,
		&fakeStudyPlanRepository{},
		topics,
		concepts,
		app.NewSyncFlashcardsUseCase(concepts, flashcards, cardStates),
	)

	input := app.AcceptGapInput{
		GapID:    "gap-1",
		Question: "¿Por qué el prototipo ata modelo e implementación?",
	}

	first, err := useCase.Execute(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, domain.RangeGapStateCarded, first.State)

	due, err := flashcards.FindDueWithContext(ctx, domain.Now())
	require.NoError(t, err)
	require.Len(t, due, 1, "the first accept writes exactly one card")

	_, err = useCase.Execute(ctx, input)
	assert.ErrorIs(t, err, apperror.ErrConflict)

	due, err = flashcards.FindDueWithContext(ctx, domain.Now())
	require.NoError(t, err)
	assert.Len(t, due, 1, "a refused second accept never writes a second card")
}

// S7: a discarded gap is a judgment the reader made, so it is not turned into a card
// behind their back. The way back is the state route, which reopens it first.
func TestAcceptGap_RefusesADiscardedGap(t *testing.T) {
	fixture := newAcceptGapFixture()
	fixture.gaps.gaps[0].State = domain.RangeGapStateDiscarded

	result, err := fixture.useCase.Execute(t.Context(), app.AcceptGapInput{
		GapID:    "g1",
		Question: "¿Por qué el prototipo crudo fue importante?",
	})

	assert.ErrorIs(t, err, apperror.ErrConflict)
	var conflict *app.GapConflict
	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, "this gap was discarded", conflict.Error())
	assert.Empty(t, result.GapID)
	assert.Empty(t, fixture.cards.calls, "a refused accept must not write a card")
	assert.Equal(t, []string{"gap"}, fixture.order.steps,
		"a refusal reads the gap and writes nothing else")
}
