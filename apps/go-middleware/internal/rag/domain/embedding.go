package domain

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
)

// Embedder turns texts into vectors.
//
// The port is one method and takes the batch as a slice, because embedding is a
// network call in this system and one call per chunk of a 359-page book would
// cost hundreds of round trips. The adapter owns batching below this port; a
// caller passes every text it wants vectors for.
//
// It is declared by this context rather than by the adapter: platform/llm
// satisfies it implicitly and knows nothing about chunks, which is what lets the
// embedding client live in platform without either package importing the other.
type Embedder interface {
	// Embed returns one vector per input text, in the same order as the input. An
	// empty input returns an empty result and performs no request.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// StoredChunk is a chunk as the embedding stage reads it back: the identity the
// vector is stored against, and the text that produced it.
type StoredChunk struct {
	// ID is the chunk's storage identity.
	ID string

	// Text is the chunk's text, verbatim.
	Text string
}

// ChunkEmbedding pairs a chunk with the vector produced for its text.
type ChunkEmbedding struct {
	// ChunkID is the chunk the vector belongs to.
	ChunkID string

	// Vector is the embedding. Its length is the model's dimension.
	Vector []float32
}

// StoredEmbedding is a stored vector together with everything a search result
// needs to cite and display its chunk, so the semantic stage needs no second
// lookup.
type StoredEmbedding struct {
	// ChunkID identifies the chunk.
	ChunkID string

	// DocumentID identifies the book the chunk came from.
	DocumentID string

	// DocumentTitle is the document's title.
	DocumentTitle string

	// PageNumber is the 1-based page to open.
	PageNumber int

	// Text is the chunk's text, verbatim.
	Text string

	// Vector is the stored embedding. Its length is the stored dimension.
	Vector []float32
}

// EmbeddingRepository manages persistence of chunk vectors.
type EmbeddingRepository interface {
	// ReplaceDocumentEmbeddings deletes the document's stored vectors and writes
	// the given ones in their place, inside one transaction.
	//
	// Replace is the only write, for the same reason the chunk index replaces:
	// embeddings are derived data with a rebuildable life cycle, so recovering
	// from a stale, partial, or differently-modelled set is re-running this call.
	// Every row is written with model and its dimension, so a vector from another
	// model is detectable rather than silently compared against the current one.
	//
	// An unknown document or chunk is apperror.ErrNotFound.
	ReplaceDocumentEmbeddings(
		ctx context.Context,
		documentID, model string,
		embeddings []ChunkEmbedding,
		now time.Time,
	) error

	// ListByModel returns every stored vector produced by model, each joined with
	// its chunk's citation and text, ordered by document, page, and ordinal.
	//
	// Filtering by model is what keeps a vector from another model out of a
	// search: comparing a 1024-dimension vector against a 3072-dimension query
	// would either fail or, worse, produce a similarity that means nothing.
	ListByModel(ctx context.Context, model string) ([]StoredEmbedding, error)

	// CountByDocument returns how many vectors the document has for model.
	//
	// It is an aggregate read, not a list: the processing-status endpoint needs
	// the count, and ListByModel would read every vector's blob to answer a
	// question about how many. The model filter is the same one ListByModel
	// applies, so a vector from another model is not counted. An unknown document
	// returns 0, not an error.
	CountByDocument(ctx context.Context, documentID, model string) (int, error)
}

// Cosine returns the cosine similarity of two vectors, in [-1, 1].
//
// It is accumulated in float64 because a 3072-dimension vector of order-one
// components overflows the precision a float32 accumulator keeps, and the loss
// lands exactly in the small terms that distinguish two near-neighbours.
//
// A zero vector has no direction, so its similarity is defined as 0 rather than
// an error: one degenerate stored vector must not make the whole search
// unrunnable. Vectors of different lengths, or an empty vector, are
// apperror.ErrInvalidInput — those are programming errors, not data.
func Cosine(a, b []float32) (float64, error) {
	if len(a) == 0 || len(b) == 0 {
		return 0, fmt.Errorf("cosine needs two non-empty vectors, got %d and %d: %w",
			len(a), len(b), apperror.ErrInvalidInput)
	}
	if len(a) != len(b) {
		return 0, fmt.Errorf("cosine needs vectors of equal length, got %d and %d: %w",
			len(a), len(b), apperror.ErrInvalidInput)
	}

	var dot, normA, normB float64
	for i := range a {
		x := float64(a[i])
		y := float64(b[i])
		dot += x * y
		normA += x * x
		normB += y * y
	}

	if normA == 0 || normB == 0 {
		return 0, nil
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB)), nil
}
