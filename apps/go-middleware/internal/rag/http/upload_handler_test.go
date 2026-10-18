package http_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/http"
)

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// fakeStore reads the whole stream the handler hands it and records the bytes,
// so a test can prove the payload reached the store unchanged and that the hash
// was computed over the same stream.
type fakeStore struct {
	mu    sync.Mutex
	calls int
	name  string
	read  []byte
	last  storage.Artifact
	err   error
}

func (f *fakeStore) Put(_ context.Context, originalName string, content io.Reader) (storage.Artifact, error) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.name = originalName
	f.mu.Unlock()

	if f.err != nil {
		return storage.Artifact{}, f.err
	}

	raw, err := io.ReadAll(content)
	if err != nil {
		return storage.Artifact{}, err
	}

	artifact := storage.Artifact{ID: fmt.Sprintf("artifact-%d", call), OriginalName: originalName}

	f.mu.Lock()
	f.read = raw
	f.last = artifact
	f.mu.Unlock()

	return artifact, nil
}

func (f *fakeStore) bytesRead() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.read...)
}

func (f *fakeStore) storedName() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.name
}

func (f *fakeStore) artifactID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last.ID
}

func (f *fakeStore) putCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeDocuments is an in-memory DocumentRegistrar keyed by content hash, the
// same identity the real repository enforces.
type fakeDocuments struct {
	mu      sync.Mutex
	byHash  map[string]*domain.Document
	creates int
}

func newFakeDocuments() *fakeDocuments {
	return &fakeDocuments{byHash: map[string]*domain.Document{}}
}

func (f *fakeDocuments) FindByContentHash(_ context.Context, contentSHA256 string) (*domain.Document, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	document, ok := f.byHash[contentSHA256]
	if !ok {
		return nil, apperror.ErrNotFound
	}
	return document, nil
}

func (f *fakeDocuments) Create(_ context.Context, document *domain.Document) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.byHash[document.ContentSHA256]; exists {
		return apperror.ErrConflict
	}
	f.byHash[document.ContentSHA256] = document
	f.creates++
	return nil
}

func (f *fakeDocuments) createCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.creates
}

// fakeProcessor records the document IDs the handler queued.
type fakeProcessor struct {
	mu        sync.Mutex
	submitted []string
}

func (f *fakeProcessor) Submit(documentID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submitted = append(f.submitted, documentID)
}

func (f *fakeProcessor) queued() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.submitted...)
}

// multipartRequest builds a multipart/form-data request. An empty fieldName
// builds a body with no file part.
func multipartRequest(t *testing.T, target, fieldName, filename string, content []byte) *http.Request {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if fieldName != "" {
		part, err := writer.CreateFormFile(fieldName, filename)
		require.NoError(t, err)
		_, err = part.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, target, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestUploadDocumentHandler_AcceptsAPDFAndQueuesIt(t *testing.T) {
	store := &fakeStore{}
	documents := newFakeDocuments()
	processor := &fakeProcessor{}
	handler := httppkg.NewUploadDocumentHandler(store, documents, processor, true)

	content := []byte("%PDF-1.7\nbody bytes")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, multipartRequest(t, "/api/rag/documents", "file", "DDD.pdf", content))

	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp httppkg.UploadDocumentResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.DocumentID)
	assert.Equal(t, "queued", resp.State)

	assert.Equal(t, content, store.bytesRead(), "the bytes reach the store unchanged")
	assert.Equal(t, "DDD.pdf", store.storedName())

	document, err := documents.FindByContentHash(t.Context(), sha256Hex(content))
	require.NoError(t, err)
	assert.Equal(t, resp.DocumentID, document.ID)
	assert.Equal(t, "DDD.pdf", document.Title)
	assert.Equal(t, store.artifactID(), document.ArtifactID)
	assert.Equal(t, []string{document.ID}, processor.queued())
}

func TestUploadDocumentHandler_ReuploadingTheSameBytesResumesTheSameDocument(t *testing.T) {
	store := &fakeStore{}
	documents := newFakeDocuments()
	processor := &fakeProcessor{}
	handler := httppkg.NewUploadDocumentHandler(store, documents, processor, true)

	content := []byte("%PDF-1.7\nsame bytes")

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, multipartRequest(t, "/api/rag/documents", "file", "book.pdf", content))
	require.Equal(t, http.StatusAccepted, first.Code)

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, multipartRequest(t, "/api/rag/documents", "file", "book.pdf", content))
	require.Equal(t, http.StatusAccepted, second.Code)

	var firstResp, secondResp httppkg.UploadDocumentResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstResp))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondResp))

	assert.Equal(t, firstResp.DocumentID, secondResp.DocumentID,
		"re-uploading the same bytes resumes the same document")
	assert.Equal(t, 1, documents.createCount(), "the document is created only when absent")
}

func TestUploadDocumentHandler_NoAPIKeyIsServiceUnavailable(t *testing.T) {
	store := &fakeStore{}
	documents := newFakeDocuments()
	handler := httppkg.NewUploadDocumentHandler(store, documents, &fakeProcessor{}, false)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, multipartRequest(t, "/api/rag/documents", "file", "book.pdf", []byte("%PDF-1.7")))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, 0, store.putCalls(), "a refused upload must not store the bytes")
}

func TestUploadDocumentHandler_RejectsABodyThatIsNotMultipart(t *testing.T) {
	store := &fakeStore{}
	handler := httppkg.NewUploadDocumentHandler(store, newFakeDocuments(), &fakeProcessor{}, true)

	req := httptest.NewRequest(http.MethodPost, "/api/rag/documents", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 0, store.putCalls())
}

func TestUploadDocumentHandler_RejectsAMissingFilePart(t *testing.T) {
	store := &fakeStore{}
	handler := httppkg.NewUploadDocumentHandler(store, newFakeDocuments(), &fakeProcessor{}, true)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, multipartRequest(t, "/api/rag/documents", "other", "", []byte("x")))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 0, store.putCalls())
}

func TestUploadDocumentHandler_RejectsAnEmptyFilePart(t *testing.T) {
	store := &fakeStore{}
	handler := httppkg.NewUploadDocumentHandler(store, newFakeDocuments(), &fakeProcessor{}, true)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, multipartRequest(t, "/api/rag/documents", "file", "empty.pdf", nil))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 0, store.putCalls())
}

func TestUploadDocumentHandler_RejectsBytesThatAreNotAPDF(t *testing.T) {
	store := &fakeStore{}
	handler := httppkg.NewUploadDocumentHandler(store, newFakeDocuments(), &fakeProcessor{}, true)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, multipartRequest(t, "/api/rag/documents", "file", "book.pdf", []byte("just text")))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 0, store.putCalls(), "a non-PDF must be refused before it is stored")
}

func TestUploadDocumentHandler_TooLargeIsRequestEntityTooLarge(t *testing.T) {
	store := &fakeStore{err: apperror.ErrTooLarge}
	handler := httppkg.NewUploadDocumentHandler(store, newFakeDocuments(), &fakeProcessor{}, true)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, multipartRequest(t, "/api/rag/documents", "file", "big.pdf", []byte("%PDF-1.7")))

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}
