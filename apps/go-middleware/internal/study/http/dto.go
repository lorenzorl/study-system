package http

import "time"

// instant encodes a timestamp for a response body.
//
// This is the only place response timestamps are formatted, and that is the
// point. A hand-written layout with a literal "Z" is a trap: in Go a bare Z is
// an ordinary character, not a zone token (that would be Z07:00), so it
// silently labels any non-UTC value as UTC. Four handlers each carried their own
// copy of that literal, and every value they did not convert first was sent to
// the client wrong by the machine's UTC offset.
//
// Converting to UTC here means the encoded string is honest regardless of the
// zone the value arrived in.
func instant(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// This file holds only the JSON shapes for the study context. The generic error
// envelope lives in platform/httpx, which every context shares.

// SyncConceptRequest is the JSON body for POST /api/sync/concept.
type SyncConceptRequest struct {
	TopicName    string `json:"topic_name"`
	ConceptTitle string `json:"concept_title"`
	FilePath     string `json:"file_path"`
}

// SyncConceptResponse is the JSON body returned by POST /api/sync/concept.
type SyncConceptResponse struct {
	ConceptID string `json:"concept_id"`
}

// CardRequest represents a single flashcard in the sync request.
type CardRequest struct {
	ObsidianID string `json:"obsidian_id"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
}

// SyncFlashcardsRequest is the JSON body for POST /api/sync/flashcards.
type SyncFlashcardsRequest struct {
	ConceptID string        `json:"concept_id"`
	Cards     []CardRequest `json:"cards"`
}

// SyncFlashcardsResponse is the JSON body returned by POST /api/sync/flashcards.
type SyncFlashcardsResponse struct {
	Synced int `json:"synced"`
}

// TopicResponse represents a topic with its nested concepts in the tree response.
type TopicResponse struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Concepts []ConceptDTO `json:"concepts"`
}

// ConceptDTO is a lightweight concept representation for the tree response.
type ConceptDTO struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	FilePath string `json:"file_path"`
}

// CreateTopicRequest is the JSON body for POST /api/topics.
type CreateTopicRequest struct {
	Name string `json:"name"`
}

// CreateTopicResponse is the JSON body returned by POST /api/topics.
type CreateTopicResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// CreateConceptRequest is the JSON body for POST /api/concepts.
type CreateConceptRequest struct {
	TopicID string `json:"topic_id"`
	Title   string `json:"title"`
}

// CreateConceptResponse is the JSON body returned by POST /api/concepts.
type CreateConceptResponse struct {
	ID        string `json:"id"`
	TopicID   string `json:"topic_id"`
	Title     string `json:"title"`
	FilePath  string `json:"file_path"`
	CreatedAt string `json:"created_at"`
}

// SyncResourceRequest is the JSON body for POST /api/sync/resource.
type SyncResourceRequest struct {
	TopicName     string `json:"topic_name"`
	ResourceTitle string `json:"resource_title"`
	Type          string `json:"type"`
	SourceURI     string `json:"source_uri"`
}

// SyncResourceResponse is the JSON body returned by POST /api/sync/resource.
type SyncResourceResponse struct {
	ResourceID string `json:"resource_id"`
}

// ResourceSummaryResponse is one item of the JSON array returned by GET /api/resources.
type ResourceSummaryResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	SourceURI string `json:"source_uri"`
	TopicName string `json:"topic_name"`
	CreatedAt string `json:"created_at"`
}

// DueCardResponse is the JSON body returned by GET /api/study/due.
type DueCardResponse struct {
	ID           string `json:"flashcard_id"`
	Question     string `json:"question"`
	Answer       string `json:"answer"`
	ConceptTitle string `json:"concept_title"`
	TopicName    string `json:"topic_name"`
	NextReview   string `json:"next_review"`
}

// ReviewRequest is the JSON body for POST /api/study/review.
type ReviewRequest struct {
	FlashcardID string `json:"flashcard_id"`
	Grade       int    `json:"grade"`
	DurationMs  int    `json:"duration_ms"`
}

// ReviewResponse is the JSON body returned by POST /api/study/review.
type ReviewResponse struct {
	NextReview string  `json:"next_review"`
	Stability  float64 `json:"stability"`
	Difficulty float64 `json:"difficulty"`
}
