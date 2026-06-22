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
