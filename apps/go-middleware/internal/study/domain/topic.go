package domain

import "time"

// Topic represents a study topic that groups related concepts together.
type Topic struct {
	ID        string
	Name      string
	CreatedAt time.Time
}
