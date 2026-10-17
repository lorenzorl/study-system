package domain

import "time"

// Concept represents an atomic unit of study material within a topic.
type Concept struct {
	ID        string
	TopicID   string
	Title     string
	FilePath  string
	CreatedAt time.Time
}
