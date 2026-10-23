package http

import (
	"encoding/json"
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
	// An announced length over the ceiling is refused before a byte is read: a
	// client pasting a book has nothing to gain from watching it upload first.
	if r.ContentLength > h.maxBodyBytes {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, h.tooLargeMessage())
		return
	}

	// MaxBytesReader is the streaming half of the same ceiling: a body that lies
	// about its length, or sends none, cannot make the decoder buffer past the
	// limit. The body is therefore never held in memory beyond it.
	body := http.MaxBytesReader(w, r.Body, h.maxBodyBytes)

	var request IngestTextRequest
	if err := json.NewDecoder(body).Decode(&request); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, h.tooLargeMessage())
			return
		}
		// The decoder's own reason, not a generic message: a client that sent a
		// trailing comma should be able to see where.
		httpx.WriteError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
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
	})
	if err != nil {
		// A duplicate is a refusal a client can explain to a user — the material is
		// already in the corpus — so it gets a message of its own instead of the
		// bare "conflict" the generic mapping prints.
		if errors.Is(err, apperror.ErrConflict) {
			httpx.WriteError(w, http.StatusConflict, "a document with the same text already exists")
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

// tooLargeMessage names the bound, so a caller knows what to trim rather than
// only that it was refused.
func (h *IngestTextHandler) tooLargeMessage() string {
	return fmt.Sprintf("the request body must be at most %d bytes", h.maxBodyBytes)
}
