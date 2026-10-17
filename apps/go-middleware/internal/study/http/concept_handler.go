package http

import (
	"encoding/json"
	"net/http"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
)

// SyncConceptHandler handles POST /api/sync/concept.
type SyncConceptHandler struct {
	useCase SyncConceptUseCase
}

// NewSyncConceptHandler creates a SyncConceptHandler.
func NewSyncConceptHandler(uc SyncConceptUseCase) *SyncConceptHandler {
	return &SyncConceptHandler{useCase: uc}
}

func (h *SyncConceptHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req SyncConceptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TopicName == "" || req.ConceptTitle == "" || req.FilePath == "" {
		httpx.WriteError(w, http.StatusBadRequest, "topic_name, concept_title, and file_path are required")
		return
	}

	conceptID, err := h.useCase.Execute(r.Context(), req.TopicName, req.ConceptTitle, req.FilePath)
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, SyncConceptResponse{ConceptID: conceptID})
}

// ListConceptsHandler handles GET /api/concepts.
type ListConceptsHandler struct {
	useCase ListConceptsUseCase
}

// NewListConceptsHandler creates a ListConceptsHandler.
func NewListConceptsHandler(uc ListConceptsUseCase) *ListConceptsHandler {
	return &ListConceptsHandler{useCase: uc}
}

func (h *ListConceptsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	topics, err := h.useCase.Execute(r.Context())
	if err != nil {
		httpx.WriteErrorFrom(w, err)
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

	httpx.WriteJSON(w, http.StatusOK, response)
}

// CreateConceptHandler handles POST /api/concepts.
type CreateConceptHandler struct {
	useCase CreateConceptUseCase
}

// NewCreateConceptHandler creates a CreateConceptHandler.
func NewCreateConceptHandler(uc CreateConceptUseCase) *CreateConceptHandler {
	return &CreateConceptHandler{useCase: uc}
}

func (h *CreateConceptHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req CreateConceptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	concept, err := h.useCase.Execute(r.Context(), req.TopicID, req.Title)
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, CreateConceptResponse{
		ID:        concept.ID,
		TopicID:   concept.TopicID,
		Title:     concept.Title,
		FilePath:  concept.FilePath,
		CreatedAt: instant(concept.CreatedAt),
	})
}
