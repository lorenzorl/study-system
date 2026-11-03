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

// ExplainRangeRequest is the JSON body for POST /api/study/explanation.
type ExplainRangeRequest struct {
	DocumentID string `json:"document_id"`
	FromPage   int    `json:"from_page"`
	ToPage     int    `json:"to_page"`
	Text       string `json:"text"`
}

// RangeGapResponse is one gap the explanation left out: the point, the page that
// states it, and the quote that evidences it. The three travel together because a
// gap missing any of them cannot become a card.
type RangeGapResponse struct {
	Point string `json:"point"`
	Page  int    `json:"page"`
	Quote string `json:"quote"`
}

// ExplainRangeResponse is the JSON body returned by POST /api/study/explanation.
//
// It carries no score: the explanation is the reader's retrieval material, and a
// number on an open response is noise that invites writing for the grader.
type ExplainRangeResponse struct {
	ExplanationID string             `json:"explanation_id"`
	DocumentID    string             `json:"document_id"`
	FromPage      int                `json:"from_page"`
	ToPage        int                `json:"to_page"`
	Gaps          []RangeGapResponse `json:"gaps"`
}

// SetReadingProgressRequest is the JSON body for PUT /api/study/reading.
type SetReadingProgressRequest struct {
	DocumentID string `json:"document_id"`
	FromPage   int    `json:"from_page"`
	ToPage     int    `json:"to_page"`
	State      string `json:"state"`
}

// ReadingProgressResponse is one page range and whether the reader read it.
//
// Both timestamps travel: created_at is when the range was first marked and
// updated_at when its state last changed, and the difference is what lets a client
// show "marked read two days ago" without a second request.
type ReadingProgressResponse struct {
	DocumentID string `json:"document_id"`
	FromPage   int    `json:"from_page"`
	ToPage     int    `json:"to_page"`
	State      string `json:"state"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// SavePlanRequest is the JSON body for POST /api/study/plan.
type SavePlanRequest struct {
	DocumentID  string            `json:"document_id"`
	ReadingMode string            `json:"reading_mode"`
	Objective   string            `json:"objective"`
	Source      string            `json:"source"`
	Items       []PlanItemRequest `json:"items"`
}

// PlanItemRequest is one recommended range in the plan the agent produced.
type PlanItemRequest struct {
	Title    string `json:"title"`
	FromPage int    `json:"from_page"`
	ToPage   int    `json:"to_page"`
	Note     string `json:"note"`
	State    string `json:"state"`
}

// StudyPlanResponse is a stored plan as a caller reads it.
//
// Read is derived, not stored: it says whether coverage marked this item's exact
// range, so the plan and the coverage axis cannot disagree.
type StudyPlanResponse struct {
	PlanID      string             `json:"plan_id"`
	DocumentID  string             `json:"document_id"`
	ReadingMode string             `json:"reading_mode"`
	Objective   string             `json:"objective"`
	Source      string             `json:"source"`
	CreatedAt   string             `json:"created_at"`
	Items       []PlanItemResponse `json:"items"`
}

// PlanItemResponse is one recommended range with its derived read flag.
type PlanItemResponse struct {
	Position int    `json:"position"`
	Title    string `json:"title"`
	FromPage int    `json:"from_page"`
	ToPage   int    `json:"to_page"`
	Note     string `json:"note"`
	State    string `json:"state"`
	Read     bool   `json:"read"`
}
