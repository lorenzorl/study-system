package http

import (
	"net/http"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
)

// TodayHandler handles GET /api/study/today.
//
// It is a projection and only a projection: nothing is written, nothing is decided,
// and the whole answer is derived from rows the other study routes stored. That is
// why the handler takes no document and no range — the day is the question, and it
// covers every material.
type TodayHandler struct {
	useCase TodayUseCase
}

// NewTodayHandler creates a TodayHandler.
func NewTodayHandler(uc TodayUseCase) *TodayHandler {
	return &TodayHandler{useCase: uc}
}

func (h *TodayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	today, err := h.useCase.Execute(r.Context())
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, todayResponse(today))
}

// todayResponse maps the projection to its wire shape.
//
// The entries are built with their length so nothing to do encodes as [] rather
// than null: a client ranges over this list, and an empty day is the normal case.
func todayResponse(today app.Today) TodayResponse {
	response := TodayResponse{
		DueCount: today.DueCount,
		Entries:  make([]TodayEntryResponse, 0, len(today.Entries)),
	}

	for _, entry := range today.Entries {
		mapped := TodayEntryResponse{
			Kind:          entry.Kind,
			DocumentID:    entry.DocumentID,
			DocumentTitle: entry.DocumentTitle,
			Title:         entry.Title,
			FromPage:      entry.FromPage,
			ToPage:        entry.ToPage,
			Note:          entry.Note,
		}

		// Zero means "this kind has no such instant", not "the beginning of time":
		// encoding it would say a plan item was read in year 1.
		if !entry.ReadAt.IsZero() {
			mapped.ReadAt = instant(entry.ReadAt)
		}

		response.Entries = append(response.Entries, mapped)
	}

	return response
}
