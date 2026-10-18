package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// T6/S3: a document_id narrows the lexical search to that document's chunks.
func TestSearchUseCase_DocumentIDNarrowsTheLexicalSearch(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	target := h.seedRecordedPages(t, "target", map[int]string{1: "the aggregate root"})
	other := h.seedRecordedPages(t, "other", map[int]string{1: "the aggregate boundary"})
	_, err := h.indexer.Execute(ctx, target.ID)
	require.NoError(t, err)
	_, err = h.indexer.Execute(ctx, other.ID)
	require.NoError(t, err)

	whole, err := h.searcher.Search(ctx, "aggregate", app.DefaultSearchLimit)
	require.NoError(t, err)
	require.Len(t, whole, 2, "without document_id both documents are in scope")

	scoped, err := h.searcher.SearchInDocument(ctx, "aggregate", target.ID, app.DefaultSearchLimit)
	require.NoError(t, err)
	require.Len(t, scoped, 1)
	assert.Equal(t, target.ID, scoped[0].DocumentID)
	assert.NotEqual(t, other.ID, scoped[0].DocumentID)
}

// T6: a document_id that matches nothing is not an error and not a special case;
// it is the same empty answer a query with no match produces.
func TestSearchUseCase_DocumentIDThatMatchesNothingIsEmpty(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	document := h.seedRecordedPages(t, "target", map[int]string{1: "the aggregate root"})
	_, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	results, err := h.searcher.SearchInDocument(ctx, "aggregate", "a-document-that-does-not-exist", app.DefaultSearchLimit)
	require.NoError(t, err)
	assert.NotNil(t, results, "an empty result is a slice, never nil")
	assert.Empty(t, results)
}

// PRESERVE: an absent (empty) document_id must be exactly the whole-corpus search
// the endpoint answered before the parameter existed.
func TestSearchUseCase_AbsentDocumentIDMatchesTheWholeCorpusSearch(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	first := h.seedRecordedPages(t, "first", map[int]string{1: "the aggregate root"})
	second := h.seedRecordedPages(t, "second", map[int]string{1: "the aggregate boundary"})
	_, err := h.indexer.Execute(ctx, first.ID)
	require.NoError(t, err)
	_, err = h.indexer.Execute(ctx, second.ID)
	require.NoError(t, err)

	whole, err := h.searcher.Search(ctx, "aggregate", 5)
	require.NoError(t, err)
	scoped, err := h.searcher.SearchInDocument(ctx, "aggregate", "", 5)
	require.NoError(t, err)

	assert.Equal(t, whole, scoped,
		"an empty document_id must return the same results as the unscoped search")
}

// T6: the semantic stage reads the matches with the same scope as the lexical
// one, so a filter that applied to only one of them would answer with another
// document's text.
func TestSearchUseCase_DocumentIDNarrowsTheHybridSearch(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	embedder := spanishToEnglishEmbedder()

	target := indexAndEmbed(t, h, embedder, "test/model", "An English paper", map[int]string{
		1: "The aggregate is the consistency boundary of the model.",
	})
	indexAndEmbed(t, h, embedder, "test/model", "Another paper", map[int]string{
		1: "The aggregate appears in another book.",
	})

	hybrid := app.NewHybridSearchUseCase(h.chunks, embeddingStore(h), embedder, "test/model")

	whole, err := hybrid.Search(ctx, "aggregate", app.DefaultSearchLimit)
	require.NoError(t, err)
	require.Len(t, whole, 2, "without document_id the hybrid search sees both documents")

	scoped, err := hybrid.SearchInDocument(ctx, "aggregate", target.ID, app.DefaultSearchLimit)
	require.NoError(t, err)
	require.NotEmpty(t, scoped)
	for _, result := range scoped {
		assert.Equal(t, target.ID, result.DocumentID,
			"the semantic stage must honour the document filter too")
	}
}

// The filter has to reach the index, before the limit. The noise document's
// chunks match the term more strongly, so a limit taken over the whole corpus
// would spend it on the noise document and then filtering it away would return
// nothing.
func TestSearchUseCase_LimitBoundsTheFilteredSet(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	target := h.seedRecordedPages(t, "target", map[int]string{1: "aggregate"})
	noise := h.seedRecordedPages(t, "noise", map[int]string{
		1: "aggregate aggregate aggregate aggregate",
		2: "aggregate aggregate aggregate aggregate",
		3: "aggregate aggregate aggregate aggregate",
	})
	_, err := h.indexer.Execute(ctx, target.ID)
	require.NoError(t, err)
	_, err = h.indexer.Execute(ctx, noise.ID)
	require.NoError(t, err)

	results, err := h.searcher.SearchInDocument(ctx, "aggregate", target.ID, 1)
	require.NoError(t, err)
	require.Len(t, results, 1, "the limit must bound the filtered set, not the unfiltered one")
	assert.Equal(t, target.ID, results[0].DocumentID)
}

// The lexical stage filters in SQL, so the test above is not enough: the semantic
// stage ranks in Go, and a filter applied after its truncation would answer a
// scoped search with nothing whenever the scoped document's best chunk ranked
// below a chunk of another document.
func TestSearchUseCase_SemanticFilterBoundsTheLimitOnTheFilteredSet(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	embedder := &fakeEmbedder{
		markers: []marker{
			{token: "consistencia", vector: []float32{1, 0, 0}},
			{token: "consistent", vector: []float32{1, 0, 0}},
			{token: "aggregate", vector: []float32{0.9, 0, 0.4358898943540674}},
		},
		fallback: []float32{0, 0, 1},
	}

	target := indexAndEmbed(t, h, embedder, "test/model", "Target",
		map[int]string{1: "the aggregate boundary"})
	noise := indexAndEmbed(t, h, embedder, "test/model", "Noise",
		map[int]string{1: "a consistent collection"})

	hybrid := app.NewHybridSearchUseCase(h.chunks, embeddingStore(h), embedder, "test/model")

	// The premise of the test, not a shortcoming of it: unscoped, this limit is
	// spent on the noise chunk, which outranks the scoped document's chunk.
	unscoped, err := hybrid.Search(ctx, "consistencia", 1)
	require.NoError(t, err)
	require.Len(t, unscoped, 1)
	require.Equal(t, noise.ID, unscoped[0].DocumentID)

	scoped, err := hybrid.SearchInDocument(ctx, "consistencia", target.ID, 1)
	require.NoError(t, err)
	require.Len(t, scoped, 1, "the limit must bound the filtered set, not the whole corpus")
	assert.Equal(t, target.ID, scoped[0].DocumentID)
	assert.Equal(t, domain.StageSemantic, scoped[0].Stage)
}
