package http

import (
	"net/http"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
)

// SectionsHandler handles GET /api/rag/documents/{documentID}/sections.
//
// It is the endpoint that exposes the chapter table the structure stage already
// stores. Without it a client has to reassemble a book's structure from its
// printed table of contents, which is the text dot leaders mangle; with it, the
// chapters a plan cites are the ones the ingestion validated.
type SectionsHandler struct {
	useCase DocumentSectionsUseCase
}

// NewSectionsHandler creates a SectionsHandler.
func NewSectionsHandler(uc DocumentSectionsUseCase) *SectionsHandler {
	return &SectionsHandler{useCase: uc}
}

func (h *SectionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sections, err := h.useCase.Execute(r.Context(), r.PathValue("documentID"))
	if err != nil {
		// An unknown document is 404 and an unread one is 200 with [], and that
		// distinction is made by the use case: httpx maps the sentinel, so the
		// transport never has to tell the two silences apart itself.
		httpx.WriteErrorFrom(w, err)
		return
	}

	// make with len guarantees an empty structure encodes as [] and never as null:
	// the consumer ranges over this array, and "this book has no chapters" must not
	// be a case it has to special-case.
	response := make(DocumentSectionsResponse, 0, len(sections))
	for _, section := range sections {
		response = append(response, SectionResponse{
			ID:        section.ID,
			ParentID:  section.ParentID,
			Level:     section.Level,
			Title:     section.Title,
			StartPage: section.StartPage,
			EndPage:   section.EndPage,
			Source:    string(section.Source),
		})
	}

	httpx.WriteJSON(w, http.StatusOK, response)
}
