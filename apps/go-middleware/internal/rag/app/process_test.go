package app_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
)

// newPipeline wires the three stages over the ingestion harness, so a full run
// needs no poppler, no network, and no real PDF.
func newPipeline(t *testing.T, pageCount int, pdf []byte) (*harness, *app.ProcessDocumentUseCase, *fakeEmbedder) {
	t.Helper()

	h := newHarness(t, pageCount, pdf).withLongText(pageCount)
	embedder := spanishToEnglishEmbedder()
	chunks := ragsqlite.NewChunkRepository(h.sqlDB)
	embeddings := ragsqlite.NewEmbeddingRepository(h.sqlDB)
	indexer := app.NewIndexDocumentUseCase(h.docs, h.jobs, h.pages, chunks)
	embedderUC := app.NewEmbedDocumentUseCase(h.docs, chunks, embeddings, embedder, "test/model")
	process := app.NewProcessDocumentUseCase(h.docs, h.store, h.useCase, indexer, embedderUC)

	return h, process, embedder
}

// seedUploadedDocument reproduces what the upload endpoint leaves behind: the
// bytes in the store and the document registered by content hash.
func seedUploadedDocument(t *testing.T, h *harness, title string) *domain.Document {
	t.Helper()
	ctx := t.Context()

	artifact, err := h.store.Put(ctx, "book.pdf", bytes.NewReader(h.pdf))
	require.NoError(t, err)

	document := &domain.Document{
		ID:            uuid.NewString(),
		Title:         title,
		ArtifactID:    artifact.ID,
		SourceURI:     "upload",
		ContentSHA256: sha256Hex(h.pdf),
		CreatedAt:     time.Now().UTC(),
	}
	require.NoError(t, h.docs.Create(ctx, document))

	return document
}

func (h *harness) chunkCount(t *testing.T, documentID string) int {
	t.Helper()

	var count int
	require.NoError(t, h.sqlDB.QueryRow(
		`SELECT count(*) FROM chunks WHERE document_id = ?`, documentID).Scan(&count))
	return count
}

func (h *harness) embeddingCount(t *testing.T) int {
	t.Helper()

	var count int
	require.NoError(t, h.sqlDB.QueryRow(`SELECT count(*) FROM chunk_embeddings`).Scan(&count))
	return count
}

func TestProcessDocumentUseCase_RunsIngestionIndexAndEmbeddings(t *testing.T) {
	h, process, embedder := newPipeline(t, 3, []byte("text-only book"))
	document := seedUploadedDocument(t, h, "Test Book")

	report, err := process.Execute(t.Context(), document.ID)
	require.NoError(t, err)

	assert.Equal(t, document.ID, report.DocumentID)
	require.NotNil(t, report.Ingest)
	assert.True(t, report.Ingest.Settled)
	assert.Equal(t, 3, report.Ingest.Extracted, "the ingestion stage produced the page text")
	assert.Greater(t, report.Chunks, 0, "the index stage produced chunks")
	assert.Equal(t, report.Chunks, report.Embeddings, "one vector per chunk")

	assert.Equal(t, report.Chunks, h.chunkCount(t, document.ID))
	assert.Equal(t, report.Embeddings, h.embeddingCount(t))
	assert.NotEmpty(t, embedder.recorded(), "the chunk text reached the embedder")

	// S4: the Markdown lands inside the artifact directory, next to the PDF.
	dir, err := h.store.Directory(document.ArtifactID)
	require.NoError(t, err)
	markdown, err := os.ReadFile(filepath.Join(dir, "book.md"))
	require.NoError(t, err)
	assert.Contains(t, string(markdown), "Page 1")

	// The upload already stored the bytes and created the document, so the
	// pipeline must resume that document rather than store a second artifact.
	artifacts, err := h.store.List()
	require.NoError(t, err)
	assert.Len(t, artifacts, 1, "the pipeline resumes the uploaded artifact instead of re-storing it")
	assert.Equal(t, 1, h.countDocuments(t))
}

func TestProcessDocumentUseCase_UnknownDocumentIsNotFound(t *testing.T) {
	_, process, _ := newPipeline(t, 1, []byte("book"))

	_, err := process.Execute(t.Context(), uuid.NewString())
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

func TestProcessDocumentUseCase_AnIngestionFailureStopsBeforeIndexing(t *testing.T) {
	h, process, _ := newPipeline(t, 2, []byte("book"))
	document := seedUploadedDocument(t, h, "Test Book")
	h.figures.err = errors.New("pdfimages is not installed")

	report, err := process.Execute(t.Context(), document.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pdfimages")
	assert.Nil(t, report.Ingest)
	assert.Equal(t, 0, h.chunkCount(t, document.ID), "a failed ingestion must not index")
	assert.Equal(t, 0, h.embeddingCount(t), "a failed ingestion must not embed")
}
