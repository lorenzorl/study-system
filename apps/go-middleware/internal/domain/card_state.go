package domain

import "time"

// CardState represents the FSRS scheduling state for a single flashcard.
// Each flashcard has exactly one CardState (enforced by UNIQUE on FlashcardID).
type CardState struct {
	ID          string
	FlashcardID string
	Stability   float64
	Difficulty  float64
	NextReview  time.Time
	LastReview  time.Time
}
