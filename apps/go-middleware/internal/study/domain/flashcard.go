package domain

import "time"

// Flashcard represents a question-answer pair linked to a concept via ObsidianID.
type Flashcard struct {
	ID         string
	ConceptID  string
	Question   string
	Answer     string
	ObsidianID string
	CreatedAt  time.Time
}
