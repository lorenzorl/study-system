package http

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
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Concepts []ConceptDTO     `json:"concepts"`
}

// ConceptDTO is a lightweight concept representation for the tree response.
type ConceptDTO struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	FilePath string `json:"file_path"`
}

// ErrorResponse is the standard JSON error body.
type ErrorResponse struct {
	Error string `json:"error"`
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
	TopicName      string `json:"topic_name"`
	ResourceTitle  string `json:"resource_title"`
	Type           string `json:"type"`
	SourceURI      string `json:"source_uri"`
	DifyDocumentID string `json:"dify_document_id"`
}

// SyncResourceResponse is the JSON body returned by POST /api/sync/resource.
type SyncResourceResponse struct {
	ResourceID string `json:"resource_id"`
}

// DueCardResponse is the JSON body returned by GET /api/study/due.
type DueCardResponse struct {
	ID           string `json:"id"`
	Question     string `json:"question"`
	Answer       string `json:"answer"`
	ConceptID    string `json:"concept_id"`
	ConceptTitle string `json:"concept_title"`
	TopicName    string `json:"topic_name"`
	Stability    float64 `json:"stability"`
	Difficulty   float64 `json:"difficulty"`
	NextReview   string `json:"next_review"`
	LastReview   string `json:"last_review"`
}

// ReviewRequest is the JSON body for POST /api/study/review.
type ReviewRequest struct {
	FlashcardID string `json:"flashcard_id"`
	Grade       int    `json:"grade"`
	DurationMs  int    `json:"duration_ms"`
}

// ReviewResponse is the JSON body returned by POST /api/study/review.
type ReviewResponse struct {
	NextReview string `json:"next_review"`
}
