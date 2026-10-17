package app

import (
	"context"
	"fmt"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// EmbedReport summarises one document's embedding rebuild.
type EmbedReport struct {
	// DocumentID is the document that was embedded.
	DocumentID string

	// Model is the model that produced the vectors.
	Model string

	// Embeddings is how many vectors were written.
	Embeddings int

	// Dimensions is the model's dimension, zero when nothing was embedded.
	Dimensions int
}

// EmbedAllReport summarises a full embedding rebuild.
type EmbedAllReport struct {
	// Documents is how many documents were walked.
	Documents int

	// Model is the model that produced the vectors.
	Model string

	// Embeddings is how many vectors were written in total.
	Embeddings int

	// Dimensions is the model's dimension, zero when nothing was embedded.
	Dimensions int
}

// EmbedDocumentUseCase rebuilds a document's vectors from its stored chunks.
//
// The vectors are derived data, exactly like the lexical index: the chunk text is
// the source of truth, so recovering from a stale, partial, or differently
// modelled set is re-running this use case. It replaces all of the document's
// vectors inside one transaction, which is what makes it idempotent.
//
// This is the only step in the RAG context that costs money and reaches the
// network. It is deliberately separate from indexing, so searching a corpus
// requires neither.
type EmbedDocumentUseCase struct {
	documents  domain.DocumentRepository
	chunks     domain.ChunkRepository
	embeddings domain.EmbeddingRepository
	embedder   domain.Embedder

	// model is the model the vectors are stored under. It is supplied once, so a
	// document's rows cannot be written under two models in one run.
	model string

	// now is a seam for tests, not a configuration knob.
	now func() time.Time
}

// NewEmbedDocumentUseCase wires an embedding rebuild.
func NewEmbedDocumentUseCase(
	documents domain.DocumentRepository,
	chunks domain.ChunkRepository,
	embeddings domain.EmbeddingRepository,
	embedder domain.Embedder,
	model string,
) *EmbedDocumentUseCase {
	return &EmbedDocumentUseCase{
		documents:  documents,
		chunks:     chunks,
		embeddings: embeddings,
		embedder:   embedder,
		model:      model,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// Execute rebuilds one document's vectors. An unknown document is
// apperror.ErrNotFound.
//
// A document with no chunks has nothing to embed; its stored vectors, if any, are
// cleared so a chunkless document cannot keep stale rows.
func (uc *EmbedDocumentUseCase) Execute(ctx context.Context, documentID string) (EmbedReport, error) {
	report := EmbedReport{DocumentID: documentID, Model: uc.model}

	if _, err := uc.documents.FindByID(ctx, documentID); err != nil {
		return report, err
	}

	return uc.embed(ctx, documentID)
}

// EmbedAll rebuilds the vectors of every document that has recorded pages.
//
// One document at a time, so a failure names the document it happened on. A
// document nobody has converted has no chunk and nothing to embed; it is cleared
// and contributes zero vectors.
func (uc *EmbedDocumentUseCase) EmbedAll(ctx context.Context) (EmbedAllReport, error) {
	documents, err := uc.documents.ListIndexable(ctx)
	if err != nil {
		return EmbedAllReport{Model: uc.model}, fmt.Errorf("list documents with recorded pages: %w", err)
	}

	report := EmbedAllReport{Model: uc.model}
	for _, document := range documents {
		embedded, err := uc.embed(ctx, document.ID)
		if err != nil {
			return report, fmt.Errorf("embed document %s: %w", document.ID, err)
		}

		report.Documents++
		report.Embeddings += embedded.Embeddings
		if embedded.Dimensions > 0 {
			report.Dimensions = embedded.Dimensions
		}
	}

	return report, nil
}

// embed reads the document's stored chunks in index order, embeds their text, and
// replaces the document's vectors.
//
// The chunks are read from the index rather than re-chunked from the page
// outcomes: the text a vector is built from must be the same text the lexical
// stage holds, or the two rankings would describe different chunks.
func (uc *EmbedDocumentUseCase) embed(ctx context.Context, documentID string) (EmbedReport, error) {
	report := EmbedReport{DocumentID: documentID, Model: uc.model}

	chunks, err := uc.chunks.ListDocumentChunks(ctx, documentID)
	if err != nil {
		return report, fmt.Errorf("list the chunks of document %s: %w", documentID, err)
	}

	if len(chunks) == 0 {
		if err := uc.embeddings.ReplaceDocumentEmbeddings(ctx, documentID, uc.model, nil, uc.now()); err != nil {
			return report, fmt.Errorf("clear the vectors of document %s: %w", documentID, err)
		}
		return report, nil
	}

	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.Text
	}

	vectors, err := uc.embedder.Embed(ctx, texts)
	if err != nil {
		return report, fmt.Errorf("embed %d chunks of document %s: %w", len(texts), documentID, err)
	}
	if len(vectors) != len(texts) {
		return report, fmt.Errorf("the embedder returned %d vectors for %d chunks of document %s: %w",
			len(vectors), len(texts), documentID, apperror.ErrInvalidInput)
	}

	embeddings := make([]domain.ChunkEmbedding, len(chunks))
	for i := range chunks {
		if len(vectors[i]) == 0 {
			return report, fmt.Errorf("the embedder returned an empty vector for chunk %s: %w",
				chunks[i].ID, apperror.ErrInvalidInput)
		}
		embeddings[i] = domain.ChunkEmbedding{ChunkID: chunks[i].ID, Vector: vectors[i]}
	}

	if err := uc.embeddings.ReplaceDocumentEmbeddings(ctx, documentID, uc.model, embeddings, uc.now()); err != nil {
		return report, fmt.Errorf("replace the vectors of document %s: %w", documentID, err)
	}

	report.Embeddings = len(embeddings)
	report.Dimensions = len(vectors[0])

	return report, nil
}
