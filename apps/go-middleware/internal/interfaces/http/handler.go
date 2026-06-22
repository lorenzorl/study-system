package http

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/application"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// SyncConceptUseCase defines the interface for syncing a concept.
type SyncConceptUseCase interface {
	Execute(ctx context.Context, topicName, conceptTitle, filePath string) (string, error)
}

// SyncFlashcardsUseCase defines the interface for syncing flashcards.
type SyncFlashcardsUseCase interface {
	Execute(ctx context.Context, conceptID string, cards []domain.Flashcard) (int, error)
}

// ListConceptsUseCase defines the interface for listing concepts.
type ListConceptsUseCase interface {
	Execute(ctx context.Context) ([]application.TopicWithConcepts, error)
}

// SyncConceptHandler handles POST /api/sync/concept requests.
type SyncConceptHandler struct {
	useCase SyncConceptUseCase
}

// NewSyncConceptHandler creates a new SyncConceptHandler.
func NewSyncConceptHandler(uc SyncConceptUseCase) *SyncConceptHandler {
	return &SyncConceptHandler{useCase: uc}
}

func (h *SyncConceptHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req SyncConceptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TopicName == "" || req.ConceptTitle == "" || req.FilePath == "" {
		writeError(w, http.StatusBadRequest, "topic_name, concept_title, and file_path are required")
		return
	}

	conceptID, err := h.useCase.Execute(r.Context(), req.TopicName, req.ConceptTitle, req.FilePath)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, SyncConceptResponse{ConceptID: conceptID})
}

// SyncFlashcardsHandler handles POST /api/sync/flashcards requests.
type SyncFlashcardsHandler struct {
	useCase SyncFlashcardsUseCase
}

// NewSyncFlashcardsHandler creates a new SyncFlashcardsHandler.
func NewSyncFlashcardsHandler(uc SyncFlashcardsUseCase) *SyncFlashcardsHandler {
	return &SyncFlashcardsHandler{useCase: uc}
}

func (h *SyncFlashcardsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req SyncFlashcardsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ConceptID == "" {
		writeError(w, http.StatusBadRequest, "concept_id is required")
		return
	}

	cards := make([]domain.Flashcard, len(req.Cards))
	for i, c := range req.Cards {
		cards[i] = domain.Flashcard{
			ObsidianID: c.ObsidianID,
			Question:   c.Question,
			Answer:     c.Answer,
		}
	}

	count, err := h.useCase.Execute(r.Context(), req.ConceptID, cards)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, SyncFlashcardsResponse{Synced: count})
}

// ListConceptsHandler handles GET /api/concepts requests.
type ListConceptsHandler struct {
	useCase ListConceptsUseCase
}

// NewListConceptsHandler creates a new ListConceptsHandler.
func NewListConceptsHandler(uc ListConceptsUseCase) *ListConceptsHandler {
	return &ListConceptsHandler{useCase: uc}
}

func (h *ListConceptsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	topics, err := h.useCase.Execute(r.Context())
	if err != nil {
		writeDomainError(w, err)
		return
	}

	response := make([]TopicResponse, 0, len(topics))
	for _, t := range topics {
		concepts := make([]ConceptDTO, 0, len(t.Concepts))
		for _, c := range t.Concepts {
			concepts = append(concepts, ConceptDTO{
				ID:       c.ID,
				Title:    c.Title,
				FilePath: c.FilePath,
			})
		}
		response = append(response, TopicResponse{
			ID:       t.ID,
			Name:     t.Name,
			Concepts: concepts,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("error encoding response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: message})
}

func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusConflict, "conflict")
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid input")
	default:
		log.Printf("internal error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
