package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// The identity a caller does not supply. Two runs of the same proposal must land
// on one card, and that can only be true if the same question yields the same id.
func TestCardIdentity_SameQuestionSameConceptIsStable(t *testing.T) {
	first := domain.CardIdentity("concept-1", "¿Qué es un agregado?")
	second := domain.CardIdentity("concept-1", "¿Qué es un agregado?")

	assert.Equal(t, first, second)
	assert.NotEmpty(t, first)
}

// The trap this pins: ids are UNIQUE globally, not per concept, so the concept
// has to be part of the derivation or two decks collide.
func TestCardIdentity_DifferentConceptIsADifferentCard(t *testing.T) {
	assert.NotEqual(t,
		domain.CardIdentity("concept-1", "¿Qué es un agregado?"),
		domain.CardIdentity("concept-2", "¿Qué es un agregado?"),
	)
}

// Re-running a proposal after a model reflow must not create a second card, so
// the same question in different spacing and case is the same card.
func TestCardIdentity_IgnoresSpacingAndCase(t *testing.T) {
	canonical := domain.CardIdentity("concept-1", "¿Qué es un agregado?")

	for _, variant := range []string{
		"¿Qué es un agregado?",
		"  ¿Qué es un agregado?  ",
		"¿Qué  es   un agregado?",
		"¿qué es un agregado?",
		"¿Qué es un agregado?\n",
	} {
		assert.Equal(t, canonical, domain.CardIdentity("concept-1", variant), "variant %q", variant)
	}
}

// The format is pinned, not the algorithm's mood: this id is stored in a UNIQUE
// column and the same reviewer reads it later. A silent change of prefix or width
// would orphan every card written before it.
func TestCardIdentity_HasOneFixedFormat(t *testing.T) {
	id := domain.CardIdentity("concept-1", "¿Qué es un agregado?")

	assert.True(t, strings.HasPrefix(id, "card-"), "got %q", id)
	assert.Len(t, id, len("card-")+32)
	assert.Regexp(t, `^card-[0-9a-f]{32}$`, id)
}

// The derived id must never be mistaken for one the Obsidian sync wrote.
func TestCardIdentity_IsNotAnObsidianID(t *testing.T) {
	id := domain.CardIdentity("concept-1", "q")
	require.NotEmpty(t, id)

	assert.NotContains(t, id, "/")
	assert.NotContains(t, id, "#")
}
