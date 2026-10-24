package http

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
)

// MaxTextDocumentTitleRunes bounds a pasted material's title.
//
// The bound is the same kind the query layer applies to a request parameter: a
// constant next to the route, named in the error message. 200 runes is chosen
// because the upload path's title is a file's base name — the only title this
// corpus had before this route — and a filesystem bounds that at 255 bytes, so a
// pasted title stays within what an uploaded one could carry and still fits one
// list row. It is not a database limit; the column is unbounded TEXT.
const MaxTextDocumentTitleRunes = 200

// duplicateTextMessage is the refusal for material the corpus already holds.
//
// Both text-creating routes answer it, with these words: the refusal is decided
// by the content hash, which one use case owns, so a client that shows it to a
// user must not have to read two differently-worded versions of the same fact.
const duplicateTextMessage = "a document with the same text already exists"

// IngestTextHandler handles POST /api/rag/documents/text.
//
// It is the second way to create a document, so it shares the collection prefix
// with the PDF upload and answers with the same body shape: a client that shows
// "document X is in state Y" works against both. The text path is synchronous —
// every stage after the paste reads only the text — so it answers 201 where the
// streamed upload answers 202, and it reports the counts that say whether the
// document was indexed only or indexed and embedded.
type IngestTextHandler struct {
	useCase IngestTextUseCase

	// maxBodyBytes is the request ceiling, the same STUDY_MAX_UPLOAD_BYTES the
	// store enforces on an artifact. A JSON paste adds envelope bytes a file does
	// not, so the ceiling bounds the whole body rather than only the text.
	maxBodyBytes int64
}

// NewIngestTextHandler creates an IngestTextHandler.
func NewIngestTextHandler(useCase IngestTextUseCase, maxBodyBytes int64) *IngestTextHandler {
	return &IngestTextHandler{useCase: useCase, maxBodyBytes: maxBodyBytes}
}

func (h *IngestTextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var request IngestTextRequest
	if !readJSONBody(w, r, h.maxBodyBytes, &request) {
		return
	}

	title := strings.TrimSpace(request.Title)
	if title == "" {
		httpx.WriteError(w, http.StatusBadRequest, "title is required")
		return
	}
	if utf8.RuneCountInString(title) > MaxTextDocumentTitleRunes {
		httpx.WriteError(w, http.StatusBadRequest,
			fmt.Sprintf("title must be at most %d characters", MaxTextDocumentTitleRunes))
		return
	}
	if strings.TrimSpace(request.Text) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "text is required")
		return
	}

	report, err := h.useCase.Execute(r.Context(), app.IngestTextRequest{
		Title: title,
		Text:  request.Text,
		// A paste was read from nowhere, so it records no origin. The field is
		// passed explicitly rather than left out: the URL route writes the same
		// field, and a reader of this call should see that "" is the answer and
		// not an omission.
		SourceURI: "",
	})
	if err != nil {
		// A duplicate is a refusal a client can explain to a user — the material is
		// already in the corpus — so it gets a message of its own instead of the
		// bare "conflict" the generic mapping prints.
		if errors.Is(err, apperror.ErrConflict) {
			httpx.WriteError(w, http.StatusConflict, duplicateTextMessage)
			return
		}
		httpx.WriteErrorFrom(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, IngestTextResponse{
		DocumentID: report.DocumentID,
		State:      string(report.State),
		Chunks:     report.Chunks,
		Embedded:   report.Embedded,
	})
}
