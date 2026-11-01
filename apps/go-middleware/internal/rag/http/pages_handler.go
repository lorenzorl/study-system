package http

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
)

// PagesHandler handles GET /api/rag/documents/{documentID}/pages.
//
// It is the endpoint a client cites from: a card proposed from a passage needs
// the passage, and (document_id, page_number) alone is a citation nobody can
// verify.
type PagesHandler struct {
	useCase DocumentPagesUseCase
}

// NewPagesHandler creates a PagesHandler.
func NewPagesHandler(uc DocumentPagesUseCase) *PagesHandler {
	return &PagesHandler{useCase: uc}
}

func (h *PagesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	from, err := parsePageParameter(query.Get("from"), "from")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	to, err := parsePageParameter(query.Get("to"), "to")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// The rules that need no document are applied before the read, so an
	// oversized request is refused without touching the database. The
	// document's last page is not known here, and 0 says so: the use case
	// applies that bound where the job's page count is available.
	if err := app.ValidatePageRange(from, to, 0); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	pages, err := h.useCase.Execute(r.Context(), r.PathValue("documentID"), from, to)
	if err != nil {
		// A refused range reads as a sentence naming the bound it missed, which
		// the generic mapping would flatten into "invalid input" — and the caller
		// then has no way to know what to ask for instead.
		var refusal *app.PageRangeError
		if errors.As(err, &refusal) {
			httpx.WriteError(w, http.StatusBadRequest, refusal.Error())
			return
		}

		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, documentPagesResponse(pages))
}

// parsePageParameter resolves one required page parameter.
//
// Absent, empty and non-numeric are separate messages because they are separate
// mistakes: a client that forgot the parameter is not the one that sent a word.
func parsePageParameter(raw, name string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("%s is required", name)
	}

	page, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return page, nil
}

// documentPagesResponse maps the read to its wire shape.
//
// The page list is built with the length of the outcomes, so an empty range
// encodes as [] and never as null: the caller ranges over it.
func documentPagesResponse(pages app.DocumentPages) DocumentPagesResponse {
	response := DocumentPagesResponse{
		DocumentID: pages.DocumentID,
		Title:      pages.Title,
		TotalPages: pages.TotalPages,
		Pages:      make([]PageTextResponse, 0, len(pages.Pages)),
	}

	for _, page := range pages.Pages {
		response.Pages = append(response.Pages, PageTextResponse{
			PageNumber: page.PageNumber,
			Status:     string(page.Status),
			Method:     string(page.Method),
			Text:       page.Text,
			Error:      page.Error,
		})
	}

	return response
}
