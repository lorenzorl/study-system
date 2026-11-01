package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// CardIdentity derives a card's identity from what makes it that card: the concept
// it belongs to and its question.
//
// The reason this exists on the server rather than in the caller: identities are
// what the upsert matches on, so a client that invents one per run turns the same
// proposal into a new card every time. Deriving it here makes "write this card
// twice" mean "update it" for every caller, including the one that cannot know an
// id at all.
//
// The concept is part of the input because ObsidianID is UNIQUE across the whole
// table, not per concept: without it, the same question in two concepts would be
// one row. The prefix keeps a derived id distinguishable from the ids the note sync
// writes, which are paths.
func CardIdentity(conceptID, question string) string {
	sum := sha256.Sum256([]byte(conceptID + "\x00" + normalizeQuestion(question)))

	// Half of the digest, in hex: 32 characters. The full width buys nothing here
	// and the shorter id is what a human reads in a database row.
	return "card-" + hex.EncodeToString(sum[:16])
}

// normalizeQuestion is the part of a question that identifies it, so a model that
// reflows or re-cases the same question between runs still lands on one card.
//
// Only spacing and case are folded. Words are never dropped and punctuation is
// never removed: "¿Qué es un agregado?" and "¿Qué no es un agregado?" differ by one
// word, and a rule that ignored negation would merge them.
func normalizeQuestion(question string) string {
	return strings.ToLower(strings.Join(strings.Fields(question), " "))
}
