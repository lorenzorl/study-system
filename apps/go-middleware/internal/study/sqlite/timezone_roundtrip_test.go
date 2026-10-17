package sqlite_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// inNonUTCZone pins time.Local. Argentina is UTC-3, which is where an untagged
// local time sits three hours away from the instant it represents.
func inNonUTCZone(t *testing.T, offsetHours int) {
	t.Helper()

	original := time.Local
	time.Local = time.FixedZone("TEST", offsetHours*60*60)
	t.Cleanup(func() { time.Local = original })
}

// gradeAgain is "+0 days": the card becomes due the moment it was reviewed. It is
// the cheapest way to check that the scheduling decision, the stored value, and
// the due query all agree on one instant.
const gradeAgain = 1

// Drives the whole study loop in a non-UTC zone: create the material, review it,
// and read it back. The instant the caller is told about, the instant on disk,
// and the instant the due query compares against must be the same one.
func TestReviewRoundTrip_PreservesTheInstantInANonUTCZone(t *testing.T) {
	inNonUTCZone(t, -3)

	sqlDB := openMigratedDB(t)
	ctx := t.Context()

	topicRepo := sqlite.NewTopicRepository(sqlDB)
	conceptRepo := sqlite.NewConceptRepository(sqlDB)
	flashcardRepo := sqlite.NewFlashcardRepository(sqlDB)
	cardStateRepo := sqlite.NewCardStateRepository(sqlDB)
	reviewLogRepo := sqlite.NewReviewLogRepository(sqlDB)

	// Build the data through the application, not around it: a review depends on
	// the card state that SyncFlashcards creates, so creating it any other way
	// would test a state that cannot occur in production.
	syncConcept := app.NewSyncConceptUseCase(topicRepo, conceptRepo)
	conceptID, err := syncConcept.Execute(ctx, "DDD", "Aggregates", "DDD/Aggregates.md")
	require.NoError(t, err)

	const obsidianID = "timezone-obsidian-1"
	syncFlashcards := app.NewSyncFlashcardsUseCase(conceptRepo, flashcardRepo, cardStateRepo)
	_, err = syncFlashcards.Execute(ctx, conceptID, []domain.Flashcard{{
		ObsidianID: obsidianID,
		Question:   "what is an aggregate?",
		Answer:     "a consistency boundary",
	}})
	require.NoError(t, err)

	card, err := flashcardRepo.FindByObsidianID(ctx, obsidianID)
	require.NoError(t, err)

	submitReview := app.NewSubmitReviewUseCase(cardStateRepo, reviewLogRepo, domain.DummyFSRS{})
	reported, err := submitReview.Execute(ctx, card.ID, gradeAgain, 1500)
	require.NoError(t, err)

	stored, err := cardStateRepo.FindByFlashcardID(ctx, card.ID)
	require.NoError(t, err)

	assert.True(t, reported.NextReview.Equal(stored.NextReview),
		"the reported instant %s and the stored instant %s must be the same one",
		reported.NextReview, stored.NextReview)

	// A stored instant is only trustworthy if it is unambiguous on disk as well.
	var rawNextReview string
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		`SELECT next_review FROM card_states WHERE flashcard_id = ?`, card.ID,
	).Scan(&rawNextReview))

	assert.True(t, strings.HasSuffix(rawNextReview, "Z"),
		"a stored instant must be tagged UTC, got %q", rawNextReview)

	parsed, err := time.Parse(time.RFC3339, rawNextReview)
	require.NoError(t, err, "the stored value must be RFC3339, got %q", rawNextReview)

	assert.True(t, parsed.Equal(stored.NextReview),
		"the stored string %q resolves to %s, not to %s", rawNextReview, parsed, stored.NextReview)

	// And the due query must agree. This is the part that was wrongly claimed to
	// be broken: a card graded Again is due immediately.
	getDueCards := app.NewGetDueCardsUseCase(flashcardRepo)
	due, err := getDueCards.Execute(ctx)
	require.NoError(t, err)

	dueIDs := make([]string, 0, len(due))
	for _, dueCard := range due {
		dueIDs = append(dueIDs, dueCard.FlashcardID)
	}

	assert.Contains(t, dueIDs, card.ID,
		"a card graded Again must be due immediately; the due query returned %v", dueIDs)
}
