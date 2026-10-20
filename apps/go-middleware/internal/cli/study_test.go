package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/composition"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	studydomain "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// seedStudy creates a migrated database under home and fills it with one topic,
// one concept and one flashcard, which starts due. It returns the flashcard id.
func seedStudy(t *testing.T, home string) string {
	t.Helper()

	cfg, err := config.LoadWith(config.Options{Home: home})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(home, "data"), 0o755))

	sqlDB, err := composition.OpenDatabase(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	study := composition.NewStudy(sqlDB)
	ctx := context.Background()

	topic, err := study.CreateTopic.Execute(ctx, "Algorithms")
	require.NoError(t, err)

	conceptID, err := study.SyncConcept.Execute(ctx, topic.Name, "Dijkstra", "algorithms/dijkstra.md")
	require.NoError(t, err)

	processed, err := study.SyncFlashcards.Execute(ctx, conceptID, []studydomain.Flashcard{
		{ObsidianID: "card-1", Question: "What does it compute?", Answer: "Shortest paths"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	due, err := study.GetDueCards.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, due, 1)

	return due[0].FlashcardID
}

func TestRun_ReviewWithoutAFlashcardIDIsAUsageError(t *testing.T) {
	code, _, stderr := runCLI(t, "review", "-grade", "3")

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "usage: study-system review")
}

func TestRun_ReviewWithoutAGradeIsAUsageError(t *testing.T) {
	code, _, stderr := runCLI(t, "review", "some-flashcard")

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "usage: study-system review")
}

func TestRun_DueListsTheSeededCard(t *testing.T) {
	home := t.TempDir()
	flashcardID := seedStudy(t, home)

	code, stdout, _ := runCLI(t, "-home", home, "due")

	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "ID")
	assert.Contains(t, stdout, flashcardID)
	assert.Contains(t, stdout, "Algorithms")
	assert.Contains(t, stdout, "Dijkstra")
	assert.Contains(t, stdout, "1 cards due")
}

func TestRun_TopicsAndConceptsReadTheSeededStudy(t *testing.T) {
	home := t.TempDir()
	seedStudy(t, home)

	code, stdout, _ := runCLI(t, "-home", home, "topics")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "Algorithms")
	assert.Contains(t, stdout, "1 topics")

	// A topic name selects its concepts; the name is matched case-insensitively.
	code, stdout, _ = runCLI(t, "-home", home, "concepts", "algorithms")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "Dijkstra")
	assert.Contains(t, stdout, "algorithms/dijkstra.md")
	assert.Contains(t, stdout, "1 concepts")
}

func TestRun_ConceptsRejectsATopicThatDoesNotExist(t *testing.T) {
	home := t.TempDir()
	seedStudy(t, home)

	// A typo must not look like an empty topic.
	code, _, stderr := runCLI(t, "-home", home, "concepts", "algoritmos")

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `no topic named "algoritmos"`)
}

func TestRun_ReviewSchedulesTheCardOutOfTheDueList(t *testing.T) {
	home := t.TempDir()
	flashcardID := seedStudy(t, home)

	code, stdout, stderr := runCLI(t, "-home", home, "review", flashcardID, "-grade", "3")
	require.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "reviewed "+flashcardID)
	assert.Contains(t, stdout, "next review")

	// A grade-3 review is scheduled days ahead, so the card leaves the due list.
	code, stdout, _ = runCLI(t, "-home", home, "due")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "0 cards due")
}

func TestRun_ReviewRejectsAGradeOutsideOneToFour(t *testing.T) {
	home := t.TempDir()
	flashcardID := seedStudy(t, home)

	code, _, stderr := runCLI(t, "-home", home, "review", flashcardID, "-grade", "5")

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "grade 5")
}

func TestRun_ConceptsOfATopicWithNoConceptsAnswersZero(t *testing.T) {
	home := t.TempDir()

	cfg, err := config.LoadWith(config.Options{Home: home})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(home, "data"), 0o755))

	sqlDB, err := composition.OpenDatabase(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	_, err = composition.NewStudy(sqlDB).CreateTopic.Execute(context.Background(), "Empty")
	require.NoError(t, err)

	// The topic exists, so this is not the "no such topic" case: it matched and it
	// answers zero, without a table header above the count.
	code, stdout, _ := runCLI(t, "-home", home, "concepts", "Empty")

	assert.Equal(t, 0, code)
	assert.NotContains(t, stdout, "ID")
	assert.Contains(t, stdout, "0 concepts")
}
