package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// fakeDueCards is the due query behind the projection's count. The two writes are
// only here because the port holds them; the projection reads nothing but the due
// list.
type fakeDueCards struct {
	cards []domain.DueCardResult
	err   error
}

func (f *fakeDueCards) UpsertByObsidianID(
	ctx context.Context,
	conceptID string,
	cards []domain.Flashcard,
) (int, error) {
	return 0, nil
}

func (f *fakeDueCards) FindByObsidianID(ctx context.Context, obsidianID string) (*domain.Flashcard, error) {
	return nil, apperror.ErrNotFound
}

func (f *fakeDueCards) FindDueWithContext(ctx context.Context, now time.Time) ([]domain.DueCardResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.cards, nil
}

// fakeTodayProgress is the coverage axis as the projection reads it.
type fakeTodayProgress struct {
	read []domain.ReadingProgress
	err  error
}

func (f *fakeTodayProgress) Upsert(
	ctx context.Context,
	progress *domain.ReadingProgress,
) (*domain.ReadingProgress, error) {
	return progress, nil
}

func (f *fakeTodayProgress) ListByDocument(
	ctx context.Context,
	documentID string,
) ([]domain.ReadingProgress, error) {
	return nil, nil
}

func (f *fakeTodayProgress) ListRead(ctx context.Context) ([]domain.ReadingProgress, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.read, nil
}

// fakeTodayExplanations answers which exact ranges are already distilled.
type fakeTodayExplanations struct {
	ranges []domain.ExplainedRange
	err    error
}

func (f *fakeTodayExplanations) Create(ctx context.Context, explanation *domain.RangeExplanation) error {
	return nil
}

func (f *fakeTodayExplanations) FindByID(ctx context.Context, id string) (*domain.RangeExplanation, error) {
	return nil, apperror.ErrNotFound
}

func (f *fakeTodayExplanations) ListExplainedRanges(ctx context.Context) ([]domain.ExplainedRange, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ranges, nil
}

// fakeTodayPlans answers the items of every active plan.
type fakeTodayPlans struct {
	items []domain.PlanItemWithDocument
	err   error
}

func (f *fakeTodayPlans) CreateActive(ctx context.Context, plan *domain.StudyPlan) (*domain.StudyPlan, error) {
	return plan, nil
}

func (f *fakeTodayPlans) FindActiveByDocument(ctx context.Context, documentID string) (*domain.StudyPlan, error) {
	return nil, apperror.ErrNotFound
}

func (f *fakeTodayPlans) ListActiveItems(ctx context.Context) ([]domain.PlanItemWithDocument, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

// failingDocuments is a catalogue that cannot name anything, for the rule that a
// title nobody can resolve must not fail the projection.
type failingDocuments struct {
	err error
}

func (f *failingDocuments) Title(ctx context.Context, documentID string) (string, error) {
	return "", f.err
}

// todayFixture is the projection's five ports with empty answers, so each test
// names only the data its rule is about.
type todayFixture struct {
	due          *fakeDueCards
	plans        *fakeTodayPlans
	progress     *fakeTodayProgress
	explanations *fakeTodayExplanations
	documents    app.Documents
}

func newTodayFixture() *todayFixture {
	return &todayFixture{
		due:          &fakeDueCards{},
		plans:        &fakeTodayPlans{},
		progress:     &fakeTodayProgress{},
		explanations: &fakeTodayExplanations{},
		documents:    &fakeDocuments{titles: map[string]string{}},
	}
}

func (f *todayFixture) execute(t *testing.T) app.Today {
	t.Helper()

	today, err := app.NewTodayUseCase(
		f.due, f.plans, f.progress, f.explanations, f.documents,
	).Execute(t.Context())
	require.NoError(t, err)

	return today
}

// readRange is one range the reader marked read, with the instant the projection
// reports as read_at.
func readRange(documentID string, from, to int, at time.Time) domain.ReadingProgress {
	return domain.ReadingProgress{
		ID:         documentID + "-" + time.Duration(at.UnixNano()).String(),
		DocumentID: documentID,
		FromPage:   from,
		ToPage:     to,
		State:      domain.ReadingStateRead,
		UpdatedAt:  at,
	}
}

// planItem is one item of a document's active plan.
func planItem(documentID, title string, position, from, to int, state domain.PlanItemState) domain.PlanItemWithDocument {
	return domain.PlanItemWithDocument{
		DocumentID: documentID,
		Item: domain.StudyPlanItem{
			ID: documentID + "-i" + time.Duration(position).String(), Position: position,
			Title: title, FromPage: from, ToPage: to, State: state,
		},
	}
}

// S13 rule 1: the oldest read range of each document, and at most one per
// document. The rows arrive out of order on purpose: the projection decides which
// one is oldest, so a repository that changed its order cannot change the answer.
func TestToday_ReadNotDistilledIsTheOldestReadRangeOfEachDocument(t *testing.T) {
	fixture := newTodayFixture()
	fixture.documents = &fakeDocuments{titles: map[string]string{"doc-1": "DDD", "doc-2": "Refactoring"}}

	oldest := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	middle := oldest.Add(48 * time.Hour)
	newest := middle.Add(48 * time.Hour)
	fixture.progress.read = []domain.ReadingProgress{
		readRange("doc-1", 100, 110, middle),
		readRange("doc-1", 24, 36, oldest),
		readRange("doc-2", 1, 10, newest),
	}

	today := fixture.execute(t)

	require.Len(t, today.Entries, 2, "at most one entry per document")

	byDocument := make(map[string]app.TodayEntry, len(today.Entries))
	for _, entry := range today.Entries {
		byDocument[entry.DocumentID] = entry
	}

	oldestOfDoc1 := byDocument["doc-1"]
	assert.Equal(t, app.TodayKindReadNotDistilled, oldestOfDoc1.Kind)
	assert.Equal(t, 24, oldestOfDoc1.FromPage, "the oldest read range, not the most recent")
	assert.Equal(t, 36, oldestOfDoc1.ToPage)
	assert.Equal(t, oldest, oldestOfDoc1.ReadAt, "read_at is that row's updated_at")
	assert.Equal(t, "DDD", oldestOfDoc1.DocumentTitle)
	assert.True(t, oldestOfDoc1.Title == "" && oldestOfDoc1.Note == "", "a read entry names no plan item")

	assert.Equal(t, "Refactoring", byDocument["doc-2"].DocumentTitle)
	assert.Equal(t, newest, byDocument["doc-2"].ReadAt)
}

// S13 rule 1 and rule 5: an explanation distils the exact range it covers, so the
// next oldest reading is the one still owed an explanation.
func TestToday_AnExplainedRangeIsAlreadyDistilledAndTheNextOldestTakesItsPlace(t *testing.T) {
	fixture := newTodayFixture()
	fixture.documents = &fakeDocuments{titles: map[string]string{"doc-1": "DDD"}}

	first := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	fixture.progress.read = []domain.ReadingProgress{
		readRange("doc-1", 24, 36, first),
		readRange("doc-1", 61, 70, first.Add(24*time.Hour)),
	}
	fixture.explanations.ranges = []domain.ExplainedRange{
		{DocumentID: "doc-1", FromPage: 24, ToPage: 36},
	}

	today := fixture.execute(t)

	require.Len(t, today.Entries, 1)
	assert.Equal(t, app.TodayKindReadNotDistilled, today.Entries[0].Kind)
	assert.Equal(t, 61, today.Entries[0].FromPage)
	assert.Equal(t, 70, today.Entries[0].ToPage)
}

// S13 rule 2: the first pending item of the active plan whose exact range is not
// read. A skipped item is never the next thing to read, and a read one is done.
func TestToday_PlanNextIsTheFirstPendingItemNotMarkedRead(t *testing.T) {
	fixture := newTodayFixture()
	fixture.documents = &fakeDocuments{titles: map[string]string{"doc-1": "DDD"}}
	fixture.progress.read = []domain.ReadingProgress{
		readRange("doc-1", 24, 36, time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)),
	}

	// Out of position order on purpose: the projection picks the first position,
	// not the first row.
	fixture.plans.items = []domain.PlanItemWithDocument{
		planItem("doc-1", "Capítulo 6", 4, 120, 140, domain.PlanItemStatePending),
		planItem("doc-1", "Capítulo 1", 1, 1, 10, domain.PlanItemStateSkipped),
		planItem("doc-1", "Capítulo 2", 2, 24, 36, domain.PlanItemStatePending),
		planItem("doc-1", "Capítulo 5", 3, 100, 118, domain.PlanItemStatePending),
	}

	today := fixture.execute(t)

	require.Len(t, today.Entries, 2, "the read range and the next plan item")

	var next app.TodayEntry
	for _, entry := range today.Entries {
		if entry.Kind == app.TodayKindPlanNext {
			next = entry
		}
	}

	assert.Equal(t, "doc-1", next.DocumentID)
	assert.Equal(t, "DDD", next.DocumentTitle)
	assert.Equal(t, "Capítulo 5", next.Title, "position 1 was skipped and position 2 is read")
	assert.Equal(t, 100, next.FromPage)
	assert.Equal(t, 118, next.ToPage)
	assert.True(t, next.ReadAt.IsZero(), "a plan item has no read instant")
}

// S13 rule 2: at most one plan entry per document, however many pending items its
// plan has.
func TestToday_OnePlanNextEntryPerDocument(t *testing.T) {
	fixture := newTodayFixture()
	fixture.documents = &fakeDocuments{titles: map[string]string{"doc-1": "DDD", "doc-2": "Refactoring"}}
	fixture.plans.items = []domain.PlanItemWithDocument{
		planItem("doc-1", "Capítulo 1", 1, 1, 10, domain.PlanItemStatePending),
		planItem("doc-1", "Capítulo 2", 2, 24, 36, domain.PlanItemStatePending),
		planItem("doc-2", "Capítulo 1", 1, 1, 10, domain.PlanItemStatePending),
	}

	today := fixture.execute(t)

	require.Len(t, today.Entries, 2)
	assert.Equal(t, "doc-1", today.Entries[0].DocumentID)
	assert.Equal(t, "Capítulo 1", today.Entries[0].Title)
	assert.Equal(t, "doc-2", today.Entries[1].DocumentID)
}

// S13 rule 3: every read_not_distilled first, oldest read_at first, and then every
// plan_next by document title.
func TestToday_OrdersReadEntriesBeforePlanEntries(t *testing.T) {
	fixture := newTodayFixture()
	fixture.documents = &fakeDocuments{titles: map[string]string{
		"doc-a": "Alpha", "doc-b": "Beta", "doc-c": "Gamma", "doc-d": "Delta",
	}}

	older := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	fixture.progress.read = []domain.ReadingProgress{
		readRange("doc-a", 24, 36, older.Add(24*time.Hour)),
		readRange("doc-b", 61, 70, older),
	}
	fixture.plans.items = []domain.PlanItemWithDocument{
		planItem("doc-c", "Capítulo 1", 1, 1, 10, domain.PlanItemStatePending),
		planItem("doc-d", "Capítulo 1", 1, 1, 10, domain.PlanItemStatePending),
	}

	today := fixture.execute(t)

	require.Len(t, today.Entries, 4)

	documents := make([]string, 0, len(today.Entries))
	for _, entry := range today.Entries {
		documents = append(documents, entry.DocumentID)
	}

	assert.Equal(t, []string{"doc-b", "doc-a", "doc-d", "doc-c"}, documents,
		"readings by read_at, then plan items by document title")
	assert.Equal(t, app.TodayKindReadNotDistilled, today.Entries[0].Kind)
	assert.Equal(t, app.TodayKindReadNotDistilled, today.Entries[1].Kind)
	assert.Equal(t, app.TodayKindPlanNext, today.Entries[2].Kind)
	assert.Equal(t, app.TodayKindPlanNext, today.Entries[3].Kind)
}

// S13 rule 3: two readings of the same instant are still ordered, by title and
// then by document id, so the same request twice answers the same thing.
func TestToday_BreaksReadAtTiesByTitleThenDocumentID(t *testing.T) {
	fixture := newTodayFixture()
	fixture.documents = &fakeDocuments{titles: map[string]string{
		"doc-z": "Zeta", "doc-m": "Alfa", "doc-a": "Alfa",
	}}

	at := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	fixture.progress.read = []domain.ReadingProgress{
		readRange("doc-z", 1, 10, at),
		readRange("doc-m", 1, 10, at),
		readRange("doc-a", 1, 10, at),
	}

	today := fixture.execute(t)

	documents := make([]string, 0, len(today.Entries))
	for _, entry := range today.Entries {
		documents = append(documents, entry.DocumentID)
	}

	assert.Equal(t, []string{"doc-a", "doc-m", "doc-z"}, documents)
}

// S13 rule 4: a title that cannot be resolved is an empty title, never a missing
// entry and never a failed projection. The document id is what the reader acts on.
func TestToday_KeepsAnEntryWhoseTitleCannotBeResolved(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "the document is gone", err: apperror.ErrNotFound},
		{name: "the catalogue cannot answer", err: errors.New("documents is unavailable")},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newTodayFixture()
			fixture.documents = &failingDocuments{err: testCase.err}
			fixture.progress.read = []domain.ReadingProgress{
				readRange("doc-1", 24, 36, time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)),
			}
			fixture.plans.items = []domain.PlanItemWithDocument{
				planItem("doc-1", "Capítulo 5", 1, 100, 118, domain.PlanItemStatePending),
			}

			today := fixture.execute(t)

			require.Len(t, today.Entries, 2)
			for _, entry := range today.Entries {
				assert.Empty(t, entry.DocumentTitle)
				assert.Equal(t, "doc-1", entry.DocumentID)
			}
		})
	}
}

// S13 rule 5: matching is the exact range, never overlap. Half a chapter read is
// not that chapter read.
func TestToday_RangeMatchingIsExactNotOverlapping(t *testing.T) {
	oldest := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)

	t.Run("a narrower read does not mark a wider plan item", func(t *testing.T) {
		fixture := newTodayFixture()
		fixture.documents = &fakeDocuments{titles: map[string]string{"doc-1": "DDD"}}
		fixture.progress.read = []domain.ReadingProgress{readRange("doc-1", 24, 35, oldest)}
		fixture.plans.items = []domain.PlanItemWithDocument{
			planItem("doc-1", "Capítulo 2", 1, 24, 36, domain.PlanItemStatePending),
		}

		today := fixture.execute(t)

		require.Len(t, today.Entries, 2)
		assert.Equal(t, app.TodayKindReadNotDistilled, today.Entries[0].Kind)
		assert.Equal(t, 24, today.Entries[0].FromPage)
		assert.Equal(t, 35, today.Entries[0].ToPage)
		assert.Equal(t, app.TodayKindPlanNext, today.Entries[1].Kind)
		assert.Equal(t, 36, today.Entries[1].ToPage)
	})

	t.Run("an explanation of a narrower range does not distil a wider reading", func(t *testing.T) {
		fixture := newTodayFixture()
		fixture.documents = &fakeDocuments{titles: map[string]string{"doc-1": "DDD"}}
		fixture.progress.read = []domain.ReadingProgress{readRange("doc-1", 24, 36, oldest)}
		fixture.explanations.ranges = []domain.ExplainedRange{
			{DocumentID: "doc-1", FromPage: 24, ToPage: 35},
		}

		today := fixture.execute(t)

		require.Len(t, today.Entries, 1)
		assert.Equal(t, app.TodayKindReadNotDistilled, today.Entries[0].Kind)
		assert.Equal(t, 24, today.Entries[0].FromPage)
		assert.Equal(t, 36, today.Entries[0].ToPage)
	})
}

// S13: nothing read and nothing planned is nothing to do, and the corpus holding a
// document is not a reason to invent an entry for it.
func TestToday_ADocumentWithNothingReadAndNoPlanContributesNoEntry(t *testing.T) {
	fixture := newTodayFixture()
	fixture.documents = &fakeDocuments{titles: map[string]string{"doc-1": "DDD"}}

	today := fixture.execute(t)

	assert.Empty(t, today.Entries)
	assert.Zero(t, today.DueCount)
}

// S13: due reviews are a count and not an entry, because there is no range to name.
func TestToday_CountsDueCardsInsteadOfListingThem(t *testing.T) {
	fixture := newTodayFixture()
	fixture.due.cards = []domain.DueCardResult{
		{FlashcardID: "f1", Front: "What is an aggregate?", Back: "A consistency boundary"},
		{FlashcardID: "f2", Front: "What is a repository?", Back: "A collection facade"},
		{FlashcardID: "f3", Front: "What is a value object?", Back: "An immutable value"},
	}

	today := fixture.execute(t)

	assert.Equal(t, 3, today.DueCount)
	assert.Empty(t, today.Entries)
}

// S13: entries is built with its length, so a client never ranges over null.
func TestToday_NeverReturnsANilSlice(t *testing.T) {
	assert.NotNil(t, newTodayFixture().execute(t).Entries)
}
