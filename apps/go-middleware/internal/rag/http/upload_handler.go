package http

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
)

// pdfMagic is the only header a PDF may start with. Checking it here keeps the
// store and the pipeline from accepting bytes that are not a PDF at all; the
// pipeline would otherwise fail minutes later, on the first page.
const pdfMagic = "%PDF-"

// uploadFieldName is the one form field the endpoint reads.
const uploadFieldName = "file"

// UploadDocumentHandler handles POST /api/rag/documents.
//
// The body is streamed: the handler buffers only the five header bytes, then
// hands the store a reader that replays those bytes and continues with the rest
// of the part, while a tee updates the SHA-256. Nothing ever holds the whole
// file in memory.
type UploadDocumentHandler struct {
	store     UploadDocumentStore
	documents UploadDocumentRepository
	processor DocumentProcessor
	hasAPIKey bool

	// now and newID are seams for tests, not configuration knobs.
	now   func() time.Time
	newID func() string
}

// NewUploadDocumentHandler creates an UploadDocumentHandler. hasAPIKey reports
// whether the server has a transcription credential; without one the endpoint
// refuses the upload, because the pipeline it would queue cannot run (S5).
func NewUploadDocumentHandler(
	store UploadDocumentStore,
	documents UploadDocumentRepository,
	processor DocumentProcessor,
	hasAPIKey bool,
) *UploadDocumentHandler {
	return &UploadDocumentHandler{
		store:     store,
		documents: documents,
		processor: processor,
		hasAPIKey: hasAPIKey,
		now:       func() time.Time { return time.Now().UTC() },
		newID:     uuid.NewString,
	}
}

func (h *UploadDocumentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.hasAPIKey {
		httpx.WriteError(w, http.StatusServiceUnavailable,
			"uploads require the "+config.EnvOpenRouterAPIKey+" environment variable")
		return
	}

	// MultipartReader streams the parts; ParseMultipartForm would buffer them to
	// disk. The reader is what keeps a large book off the heap.
	reader, err := r.MultipartReader()
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "request must be multipart/form-data")
		return
	}

	part, err := filePart(reader)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer part.Close()

	head, err := readPDFHeader(part)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// The header is replayed into the stream so the store receives the whole
	// payload, and the tee hashes every byte the store reads. A store that stops
	// early (the size ceiling) returns an error before the digest is used.
	digest := sha256.New()
	stream := io.TeeReader(io.MultiReader(bytes.NewReader(head), part), digest)

	artifact, err := h.store.Put(r.Context(), part.FileName(), stream)
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	// The document is found by content hash and created only when absent, so
	// re-uploading the same bytes resumes the same document. The rule is shared
	// with the ingestion run.
	contentHash := hex.EncodeToString(digest.Sum(nil))
	document, err := app.FindOrCreateDocument(
		r.Context(), h.documents, artifact, contentHash, artifact.OriginalName, "", h.now(), h.newID)
	if err != nil {
		httpx.WriteErrorFrom(w, err)
		return
	}

	// Submit never blocks, so the response returns immediately with the queued
	// document while the worker runs the pipeline (S2).
	h.processor.Submit(document.ID)

	httpx.WriteJSON(w, http.StatusAccepted, UploadDocumentResponse{
		DocumentID: document.ID,
		State:      string(app.StateQueued),
	})
}

// filePart returns the first part named "file", or an error naming what is
// wrong. Parts before it are discarded by NextPart, so a form carrying extra
// fields still works.
func filePart(reader *multipart.Reader) (*multipart.Part, error) {
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("file part is required")
		}
		if err != nil {
			return nil, errors.New("could not read the multipart body")
		}
		if part.FormName() == uploadFieldName {
			return part, nil
		}
	}
}

// readPDFHeader reads and validates the PDF magic. It returns the bytes it
// consumed so the store can receive the whole payload.
func readPDFHeader(part io.Reader) ([]byte, error) {
	head := make([]byte, len(pdfMagic))
	n, err := io.ReadFull(part, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, errors.New("could not read the file part")
	}
	if n == 0 {
		return nil, errors.New("the file part is empty")
	}
	if !bytes.HasPrefix(head[:n], []byte(pdfMagic)) {
		return nil, errors.New("the file part is not a PDF")
	}
	return head[:n], nil
}
