package http

import (
	"encoding/json"
	"net/http"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// SyncFlashcardsHandler handles POST /api/sync/flashcards.
type SyncFlashcardsHandler struct {
	useCase SyncFlashcardsUseCase
}

// NewSyncFlashcardsHandler creates a SyncFlashcardsHandler.
func NewSyncFlashcardsHandler(uc SyncFlashcardsUseCase) *SyncFlashcardsHandler {
	return &SyncFlashcardsHandler{useCase: uc}
}

func (h *SyncFlashcardsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req SyncFlashcardsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ConceptID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "concept_id is required")
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
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, SyncFlashcardsResponse{Synced: count})
}

// DueCardsHandler handles GET /api/study/due.
type DueCardsHandler struct {
	useCase GetDueCardsUseCase
}

// NewDueCardsHandler creates a DueCardsHandler.
func NewDueCardsHandler(uc GetDueCardsUseCase) *DueCardsHandler {
	return &DueCardsHandler{useCase: uc}
}

func (h *DueCardsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cards, err := h.useCase.Execute(r.Context())
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	response := make([]DueCardResponse, 0, len(cards))
	for _, c := range cards {
		response = append(response, DueCardResponse{
			ID:           c.FlashcardID,
			Question:     c.Front,
			Answer:       c.Back,
			ConceptTitle: c.ConceptTitle,
			TopicName:    c.TopicName,
			NextReview:   instant(c.NextReview),
		})
	}

	httpx.WriteJSON(w, http.StatusOK, response)
}
