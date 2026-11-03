package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// SetReadingProgressHandler handles PUT /api/study/reading (coverage axis).
//
// It is where the reader's claim arrives, and it stays a claim: the handler
// validates the shape of the range and stores it. Nothing checks that the pages
// were read, and nothing should — a gate here would be the wrong instrument
// (backlog, W4).
type SetReadingProgressHandler struct {
	useCase SetReadingProgressUseCase
}

// NewSetReadingProgressHandler creates a SetReadingProgressHandler.
func NewSetReadingProgressHandler(uc SetReadingProgressUseCase) *SetReadingProgressHandler {
	return &SetReadingProgressHandler{useCase: uc}
}

func (h *SetReadingProgressHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req SetReadingProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Each refusal names the field to fix instead of the generic "invalid input"
	// the sentinel would produce. The checks are repeated in the use case on
	// purpose: the application layer is the one that decides what a valid range is,
	// and this layer's copy exists so the reader gets a sentence rather than
	// "invalid input".
	documentID := strings.TrimSpace(req.DocumentID)
	if documentID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "document_id is required")
		return
	}

	if req.FromPage < 1 {
		httpx.WriteError(w, http.StatusBadRequest, "from_page must be at least 1")
		return
	}

	if req.ToPage < req.FromPage {
		httpx.WriteError(w, http.StatusBadRequest, "to_page must not be before from_page")
		return
	}

	if _, known := domain.ParseReadingState(req.State); !known {
		httpx.WriteError(w, http.StatusBadRequest, "state must be read or unread")
		return
	}

	progress, err := h.useCase.Execute(r.Context(), app.SetReadingProgressInput{
		DocumentID: documentID,
		FromPage:   req.FromPage,
		ToPage:     req.ToPage,
		State:      req.State,
	})
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, readingProgressResponse(progress))
}

// ListReadingProgressHandler handles GET /api/study/reading.
type ListReadingProgressHandler struct {
	useCase ListReadingProgressUseCase
}

// NewListReadingProgressHandler creates a ListReadingProgressHandler.
func NewListReadingProgressHandler(uc ListReadingProgressUseCase) *ListReadingProgressHandler {
	return &ListReadingProgressHandler{useCase: uc}
}

func (h *ListReadingProgressHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	documentID := strings.TrimSpace(r.URL.Query().Get("document_id"))
	if documentID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "document_id is required")
		return
	}

	ranges, err := h.useCase.Execute(r.Context(), documentID)
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	// Built with its length so a document with nothing marked encodes as [] and
	// never as null: the client ranges over it.
	response := make([]ReadingProgressResponse, 0, len(ranges))
	for _, progress := range ranges {
		response = append(response, readingProgressResponse(progress))
	}

	httpx.WriteJSON(w, http.StatusOK, response)
}

func readingProgressResponse(progress domain.ReadingProgress) ReadingProgressResponse {
	return ReadingProgressResponse{
		DocumentID: progress.DocumentID,
		FromPage:   progress.FromPage,
		ToPage:     progress.ToPage,
		State:      string(progress.State),
		CreatedAt:  instant(progress.CreatedAt),
		UpdatedAt:  instant(progress.UpdatedAt),
	}
}
