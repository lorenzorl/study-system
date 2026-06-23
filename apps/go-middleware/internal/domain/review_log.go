package domain

import "time"

// ReviewLog records a single review attempt of a flashcard.
// Grade must be 1 (Again), 2 (Hard), 3 (Good), or 4 (Easy).
type ReviewLog struct {
	ID          string
	FlashcardID string
	Grade       int
	DurationMs  int
	CreatedAt   time.Time
}
