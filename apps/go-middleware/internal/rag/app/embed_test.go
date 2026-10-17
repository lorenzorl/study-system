package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
)

// marker is one deterministic vector the fake embedder returns when a text
// mentions token.
type marker struct {
	token  string
	vector []float32
}

// fakeEmbedder returns a deterministic vector per text, chosen by a marker the
// text mentions rather than by its words.
//
// That indirection is what makes the Spanish-against-English test hermetic and
// honest at the same time: the query and the passage share a meaning represented
// by the fake, and share no literal token, exactly as the real multilingual model
// is expected to behave. No test in this package reaches the network.
type fakeEmbedder struct {
	markers  []marker
	fallback []float32

	// err, when set, is what Embed returns, so a failing model is testable.
	err error

	// short makes Embed return one vector fewer than it was asked for, so the
	// pairing guard is testable.
	short bool

	calls  int
	inputs [][]string
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	f.calls++
	f.inputs = append(f.inputs, append([]string(nil), texts...))

	if f.err != nil {
		return nil, f.err
	}

	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vectors[i] = f.vectorFor(text)
	}
	if f.short && len(vectors) > 0 {
		vectors = vectors[:len(vectors)-1]
	}
	return vectors, nil
}

func (f *fakeEmbedder) vectorFor(text string) []float32 {
	lower := strings.ToLower(text)
	for _, m := range f.markers {
		if strings.Contains(lower, m.token) {
			return m.vector
		}
	}
	return f.fallback
}

// recorded reports every text the fake was asked to embed, flattened.
func (f *fakeEmbedder) recorded() []string {
	all := make([]string, 0)
	for _, batch := range f.inputs {
		all = append(all, batch...)
	}
	return all
}

// spanishToEnglishEmbedder is the fake used by the cross-language tests.
//
// "consistencia" and "consistency" are the same idea in two languages, so the fake
// places them almost on top of each other without sharing a token. The Spanish
// vector sits on the first and third axes, so a page about a repository is
// exactly orthogonal to it rather than merely weak.
func spanishToEnglishEmbedder() *fakeEmbedder {
	return &fakeEmbedder{
		markers: []marker{
			{token: "aggregate", vector: []float32{1, 0, 0}},
			{token: "consistency", vector: []float32{1, 0, 0}},
			{token: "consistencia", vector: []float32{0.98, 0, 0.19}},
			{token: "repository", vector: []float32{0, 1, 0}},
		},
		fallback: []float32{0, 0, 1},
	}
}

// embeddingStore returns the real vector repository over the harness database.
func embeddingStore(h *indexHarness) *ragsqlite.EmbeddingRepository {
	return ragsqlite.NewEmbeddingRepository(h.sqlDB)
}

// embeddingRows counts stored vectors directly, so a claim about rows is about
// the table.
func (h *indexHarness) embeddingRows(t *testing.T) int {
	t.Helper()

	var count int
	require.NoError(t, h.sqlDB.QueryRow(`SELECT count(*) FROM chunk_embeddings`).Scan(&count))
	return count
}

// embedderFor wires an embed use case with the given model.
func embedderFor(h *indexHarness, embedder domain.Embedder, model string) *app.EmbedDocumentUseCase {
	return app.NewEmbedDocumentUseCase(h.docs, h.chunks, embeddingStore(h), embedder, model)
}

func TestEmbedDocumentUseCase_StoresAVectorPerChunk(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	embedder := spanishToEnglishEmbedder()

	document := h.seedRecordedPages(t, "un libro", map[int]string{
		1: "el agregado es la raíz de la consistencia",
		2: "un repositorio guarda objetos",
	})
	_, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	report, err := embedderFor(h, embedder, "test/model").Execute(ctx, document.ID)
	require.NoError(t, err)

	assert.Equal(t, document.ID, report.DocumentID)
	assert.Equal(t, "test/model", report.Model)
	assert.Equal(t, 2, report.Embeddings, "one vector per chunk")
	assert.Equal(t, 3, report.Dimensions)

	stored, err := embeddingStore(h).ListByModel(ctx, "test/model")
	require.NoError(t, err)
	require.Len(t, stored, 2)

	assert.Equal(t, "un libro", stored[0].DocumentTitle, "a stored vector carries its citation")
	assert.Equal(t, 1, stored[0].PageNumber)
	assert.Equal(t, "el agregado es la raíz de la consistencia", stored[0].Text)
	assert.Len(t, stored[0].Vector, 3)
}

// Acceptance criterion 4: embedding the same document twice leaves the same rows.
func TestEmbedDocumentUseCase_EmbeddingTwiceLeavesTheSameRows(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	document := h.seedRecordedPages(t, "un libro", map[int]string{
		1: "el agregado es la raíz",
		2: "el repositorio guarda objetos",
	})
	_, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	useCase := embedderFor(h, spanishToEnglishEmbedder(), "test/model")

	first, err := useCase.Execute(ctx, document.ID)
	require.NoError(t, err)
	before := h.embeddingRows(t)

	second, err := useCase.Execute(ctx, document.ID)
	require.NoError(t, err)

	assert.Equal(t, first.Embeddings, second.Embeddings)
	assert.Equal(t, before, h.embeddingRows(t), "a rebuild replaces vectors instead of appending them")
	assert.Equal(t, 2, before)
}

func TestEmbedDocumentUseCase_DocumentWithoutChunksStoresNothing(t *testing.T) {
	h := newIndexHarness(t)
	embedder := spanishToEnglishEmbedder()

	document := h.seedDocumentWithoutPages(t, "never run")

	report, err := embedderFor(h, embedder, "test/model").Execute(t.Context(), document.ID)
	require.NoError(t, err)

	assert.Equal(t, 0, report.Embeddings)
	assert.Equal(t, 0, h.embeddingRows(t))
	assert.Equal(t, 0, embedder.calls, "a document with no chunk has nothing to embed")
}

func TestEmbedDocumentUseCase_UnknownDocumentIsNotFound(t *testing.T) {
	h := newIndexHarness(t)

	_, err := embedderFor(h, spanishToEnglishEmbedder(), "test/model").Execute(t.Context(), uuid.NewString())
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

func TestEmbedDocumentUseCase_EmbedAllRebuildsEveryDocumentWithChunks(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	first := h.seedRecordedPages(t, "alpha", map[int]string{1: "alpha aggregate"})
	second := h.seedRecordedPages(t, "beta", map[int]string{1: "beta repository", 2: "beta invariants"})
	h.seedDocumentWithoutPages(t, "gamma")

	for _, document := range []*domain.Document{first, second} {
		_, err := h.indexer.Execute(ctx, document.ID)
		require.NoError(t, err)
	}

	useCase := embedderFor(h, spanishToEnglishEmbedder(), "test/model")
	report, err := useCase.EmbedAll(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, report.Documents, "a document with no chunk has nothing to embed")
	assert.Equal(t, 3, report.Embeddings)
	assert.Equal(t, "test/model", report.Model)
	assert.Equal(t, 3, report.Dimensions)

	again, err := useCase.EmbedAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, report.Embeddings, again.Embeddings, "embedding is idempotent")
	assert.Equal(t, 3, h.embeddingRows(t))
}

func TestEmbedDocumentUseCase_EmbedderFailureStoresNothing(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	document := h.seedRecordedPages(t, "un libro", map[int]string{1: "el agregado"})
	_, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	embedder := spanishToEnglishEmbedder()
	embedder.err = errors.New("the network is down")

	_, err = embedderFor(h, embedder, "test/model").Execute(ctx, document.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network is down")
	assert.Equal(t, 0, h.embeddingRows(t), "a failed embedding must not write a partial set")
}

func TestEmbedDocumentUseCase_RejectsAVectorCountThatDoesNotMatchTheChunks(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	document := h.seedRecordedPages(t, "un libro", map[int]string{1: "uno", 2: "dos"})
	_, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	embedder := spanishToEnglishEmbedder()
	embedder.short = true

	_, err = embedderFor(h, embedder, "test/model").Execute(ctx, document.ID)
	require.Error(t, err, "a vector must be paired with the chunk it was produced for")
	assert.Contains(t, err.Error(), "vector")
	assert.Equal(t, 0, h.embeddingRows(t))
}

// Acceptance criterion 5 at the use-case level: vectors belong to the model that
// produced them, so a model change replaces rather than mixes.
func TestEmbedDocumentUseCase_ChangingTheModelReplacesTheVectors(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	document := h.seedRecordedPages(t, "un libro", map[int]string{1: "el agregado"})
	_, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	_, err = embedderFor(h, spanishToEnglishEmbedder(), "model-a").Execute(ctx, document.ID)
	require.NoError(t, err)

	_, err = embedderFor(h, spanishToEnglishEmbedder(), "model-b").Execute(ctx, document.ID)
	require.NoError(t, err)

	old, err := embeddingStore(h).ListByModel(ctx, "model-a")
	require.NoError(t, err)
	assert.Empty(t, old, "the previous model's vectors must not survive a rebuild under another model")

	current, err := embeddingStore(h).ListByModel(ctx, "model-b")
	require.NoError(t, err)
	assert.Len(t, current, 1)
	assert.Equal(t, 1, h.embeddingRows(t))
}
