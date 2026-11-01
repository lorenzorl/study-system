package app_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// capturedIDs runs the sync and returns the ids the repository was asked to write.
func capturedIDs(t *testing.T, cards []domain.Flashcard) ([]string, error) {
	t.Helper()

	ctx := context.Background()
	flashcardRepo := new(MockFlashcardRepository)
	cardStateRepo := new(MockCardStateRepository)
	uc := app.NewSyncFlashcardsUseCase(new(MockConceptRepository), flashcardRepo, cardStateRepo)

	var ids []string
	flashcardRepo.
		On("UpsertByObsidianID", ctx, "concept-uuid", mock.Anything).
		Run(func(args mock.Arguments) {
			for _, card := range args.Get(2).([]domain.Flashcard) {
				ids = append(ids, card.ObsidianID)
			}
		}).
		Return(len(cards), nil).
		Maybe()

	flashcardRepo.On("FindByObsidianID", ctx, mock.Anything).
		Return(&domain.Flashcard{ID: "f-1"}, nil).Maybe()
	cardStateRepo.On("FindByFlashcardID", ctx, "f-1").
		Return(&domain.CardState{ID: "cs-1"}, nil).Maybe()

	_, err := uc.Execute(ctx, "concept-uuid", cards)
	return ids, err
}

// A caller that cannot know the id (an agent proposing cards) omits it, and the
// server derives it. Without this the same proposal written twice would become
// two cards, because ids are the only thing the upsert matches on.
func TestSyncFlashcardsUseCase_DerivesTheIdentityWhenTheCallerOmitsIt(t *testing.T) {
	ids, err := capturedIDs(t, []domain.Flashcard{
		{Question: "¿Qué es un agregado?", Answer: "Una unidad de consistencia."},
	})
	require.NoError(t, err)

	require.Len(t, ids, 1)
	assert.Equal(t,
		domain.CardIdentity("concept-uuid", "¿Qué es un agregado?"),
		ids[0],
	)
	assert.NotEmpty(t, ids[0])
}

// The property the endpoint exists for: a second run of the same proposal writes
// the same id, so the upsert updates instead of appending a duplicate.
func TestSyncFlashcardsUseCase_TheSameProposalWrittenTwiceKeepsOneID(t *testing.T) {
	card := domain.Flashcard{
		Question: "¿Qué es un agregado?",
		Answer:   "Una unidad de consistencia.",
	}

	first, err := capturedIDs(t, []domain.Flashcard{card})
	require.NoError(t, err)
	second, err := capturedIDs(t, []domain.Flashcard{card})
	require.NoError(t, err)

	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.Equal(t, first[0], second[0])
}

// A card that arrives with an id keeps it: the Obsidian sync path is a consumer of
// this use case and must not have its identity rewritten by the new rule.
func TestSyncFlashcardsUseCase_AnExplicitIdentityIsLeftAlone(t *testing.T) {
	ids, err := capturedIDs(t, []domain.Flashcard{
		{ObsidianID: "ddd/aggregates#1", Question: "Q", Answer: "A"},
	})
	require.NoError(t, err)

	require.Len(t, ids, 1)
	assert.Equal(t, "ddd/aggregates#1", ids[0])
}

// A questionless card has no identity to derive and no content to study, so it is
// refused instead of being written under a meaningless hash.
func TestSyncFlashcardsUseCase_RefusesACardWithNoQuestion(t *testing.T) {
	ids, err := capturedIDs(t, []domain.Flashcard{
		{Question: "   ", Answer: "A"},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, apperror.ErrInvalidInput)
	assert.Empty(t, ids, "nothing may be written when the batch is refused")
}

// Same rule, other half: a proposal with no answer is not a card either, and
// storing one would put an unanswerable question into the review queue.
func TestSyncFlashcardsUseCase_RefusesACardWithNoAnswer(t *testing.T) {
	ids, err := capturedIDs(t, []domain.Flashcard{
		{Question: "¿Qué es un agregado?", Answer: "  "},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, apperror.ErrInvalidInput)
	assert.Empty(t, ids)
}

// A batch may mix both kinds of caller: a note syncing with its ids and the agent
// adding a card next to it.
func TestSyncFlashcardsUseCase_MixedBatchKeepsBothIdentities(t *testing.T) {
	ids, err := capturedIDs(t, []domain.Flashcard{
		{ObsidianID: "ddd/aggregates#1", Question: "Q1", Answer: "A1"},
		{Question: "¿Qué es un agregado?", Answer: "A2"},
	})
	require.NoError(t, err)

	require.Len(t, ids, 2)
	assert.Equal(t, "ddd/aggregates#1", ids[0])
	assert.Equal(t, domain.CardIdentity("concept-uuid", "¿Qué es un agregado?"), ids[1])
}
