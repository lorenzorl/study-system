package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// SavePlanHandler handles POST /api/study/plan.
//
// It is where the plan the skill produced arrives. Nothing here classifies a book or
// reorders a chapter: the judgment lives in the skill, and this endpoint only decides
// whether the plan is well-formed enough to store.
type SavePlanHandler struct {
	useCase SavePlanUseCase
}

// NewSavePlanHandler creates a SavePlanHandler.
func NewSavePlanHandler(uc SavePlanUseCase) *SavePlanHandler {
	return &SavePlanHandler{useCase: uc}
}

func (h *SavePlanHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req SavePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Every refusal names the item and the field. The same rules are re-checked in
	// the use case, which is the layer that owns them; this copy exists so the agent
	// that posted the plan gets a sentence it can act on instead of "invalid input".
	if refusal := planRefusal(req); refusal != "" {
		httpx.WriteError(w, http.StatusBadRequest, refusal)
		return
	}

	items := make([]app.PlanItemInput, 0, len(req.Items))
	for _, item := range req.Items {
		items = append(items, app.PlanItemInput{
			Title:    item.Title,
			FromPage: item.FromPage,
			ToPage:   item.ToPage,
			Note:     item.Note,
			State:    item.State,
		})
	}

	view, err := h.useCase.Execute(r.Context(), app.SavePlanInput{
		DocumentID:  req.DocumentID,
		ReadingMode: req.ReadingMode,
		Objective:   req.Objective,
		Source:      req.Source,
		Items:       items,
	})
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, studyPlanResponse(view))
}

// GetPlanHandler handles GET /api/study/plan.
type GetPlanHandler struct {
	useCase GetPlanUseCase
}

// NewGetPlanHandler creates a GetPlanHandler.
func NewGetPlanHandler(uc GetPlanUseCase) *GetPlanHandler {
	return &GetPlanHandler{useCase: uc}
}

func (h *GetPlanHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	documentID := strings.TrimSpace(r.URL.Query().Get("document_id"))
	if documentID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "document_id is required")
		return
	}

	view, err := h.useCase.Execute(r.Context(), documentID)
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, studyPlanResponse(view))
}

// planRefusal returns the sentence for the first thing wrong with the plan, or an
// empty string when the shape is one the use case can judge.
func planRefusal(req SavePlanRequest) string {
	if strings.TrimSpace(req.DocumentID) == "" {
		return "document_id is required"
	}

	if _, known := domain.ParseReadingMode(strings.TrimSpace(req.ReadingMode)); !known {
		return "reading_mode must be linear, project, survey or consult"
	}

	if len(req.Items) == 0 {
		return "items is required"
	}

	seen := make(map[string]bool, len(req.Items))
	for index, item := range req.Items {
		position := index + 1

		if strings.TrimSpace(item.Title) == "" {
			return fmt.Sprintf("item %d: title is required", position)
		}

		if item.FromPage < 1 {
			return fmt.Sprintf("item %d: from_page must be at least 1", position)
		}

		if item.ToPage < item.FromPage {
			return fmt.Sprintf("item %d: to_page must not be before from_page", position)
		}

		// Empty means pending: an item the plan proposes is pending until the reader
		// says otherwise, and only the two states that exist are accepted.
		if raw := strings.TrimSpace(item.State); raw != "" {
			if _, known := domain.ParsePlanItemState(raw); !known {
				return fmt.Sprintf("item %d: state must be pending or skipped", position)
			}
		}

		key := fmt.Sprintf("%d-%d", item.FromPage, item.ToPage)
		if seen[key] {
			return fmt.Sprintf("duplicate range %s", key)
		}
		seen[key] = true
	}

	return ""
}

// studyPlanResponse maps a stored plan to its wire shape.
//
// The item list is built with its length, so a plan with no items would encode as []
// rather than null; it cannot happen, because a plan without items is refused, and
// the client should still never range over null.
func studyPlanResponse(view app.StudyPlanView) StudyPlanResponse {
	response := StudyPlanResponse{
		PlanID:      view.ID,
		DocumentID:  view.DocumentID,
		ReadingMode: string(view.ReadingMode),
		Objective:   view.Objective,
		Source:      view.Source,
		CreatedAt:   instant(view.CreatedAt),
		Items:       make([]PlanItemResponse, 0, len(view.Items)),
	}

	for _, item := range view.Items {
		response.Items = append(response.Items, PlanItemResponse{
			Position: item.Position,
			Title:    item.Title,
			FromPage: item.FromPage,
			ToPage:   item.ToPage,
			Note:     item.Note,
			State:    string(item.State),
			Read:     item.Read,
		})
	}

	return response
}
