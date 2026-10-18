package app_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// indexAndEmbed leaves the harness in the state a finished ingestion run plus an
// embed run leaves: chunks in the lexical index and a vector for each of them.
func indexAndEmbed(
	t *testing.T,
	h *indexHarness,
	embedder *fakeEmbedder,
	model, title string,
	pages map[int]string,
) *domain.Document {
	t.Helper()
	ctx := t.Context()

	document := h.seedRecordedPages(t, title, pages)

	_, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	_, err = embedderFor(h, embedder, model).Execute(ctx, document.ID)
	require.NoError(t, err)

	return document
}

// Acceptance criterion 2 of slice 5.2: a Spanish query finds an English passage
// it shares no literal token with, verified through the repository and the app,
// not only in a probe.
func TestSearchUseCase_SpanishQueryFindsAnEnglishPassageItSharesNoTokenWith(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	embedder := spanishToEnglishEmbedder()

	indexAndEmbed(t, h, embedder, "test/model", "An English paper", map[int]string{
		1: "The aggregate is the consistency boundary of the model.",
		2: "A repository gives the illusion of an in-memory collection.",
	})

	query := "¿cuál es el límite de consistencia del agregado?"

	// The lexical stage cannot cross the language boundary, so on its own it
	// finds nothing. That is the premise of the test, not a shortcoming of it.
	lexical, err := h.chunks.Search(ctx, query, app.DefaultSearchLimit)
	require.NoError(t, err)
	assert.Empty(t, lexical, "the Spanish query shares no indexable token with the English pages")

	hybrid := app.NewHybridSearchUseCase(h.chunks, embeddingStore(h), embedder, "test/model")
	results, err := hybrid.Search(ctx, query, app.DefaultSearchLimit)
	require.NoError(t, err)
	require.Len(t, results, 1, "the semantic stage must find exactly the English passage")

	assert.Equal(t, "An English paper", results[0].DocumentTitle)
	assert.Equal(t, 1, results[0].PageNumber, "a semantic result still cites its exact page")
	assert.Equal(t, domain.StageSemantic, results[0].Stage,
		"a result carries the stage that found it")
	assert.Greater(t, results[0].Similarity, 0.9)
	assert.Contains(t, results[0].Snippet, "consistency",
		"the snippet must show the English text that matched semantically")

	assert.Contains(t, embedder.recorded(), query, "the query itself is what was embedded")
}

// A semantic hit matched no term, so its snippet is a plain excerpt. The client
// that answers with the retrieved text needs the chunk itself, whether the stage
// that found it was lexical, semantic or both.
func TestSearchUseCase_SemanticResultCarriesTheWholeChunk(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	embedder := spanishToEnglishEmbedder()

	const chunk = "The aggregate is the consistency boundary of the model."

	indexAndEmbed(t, h, embedder, "test/model", "An English paper", map[int]string{1: chunk})

	hybrid := app.NewHybridSearchUseCase(h.chunks, embeddingStore(h), embedder, "test/model")
	results, err := hybrid.Search(ctx, "¿cuál es el límite de consistencia del agregado?", app.DefaultSearchLimit)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, domain.StageSemantic, results[0].Stage,
		"this test is about the semantic stage, so it must be the one that found it")
	assert.Equal(t, chunk, results[0].Text, "a semantic result must carry the whole chunk")
}

// Without stored vectors there is nothing to compare against, so search must stay
// a lexical search — and must not embed the query, which is what keeps it working
// with no API key and no network.
func TestSearchUseCase_FallsBackToLexicalWhenNoVectorsAreStored(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	embedder := spanishToEnglishEmbedder()

	document := h.seedRecordedPages(t, "a", map[int]string{1: "aggregate root"})
	_, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	hybrid := app.NewHybridSearchUseCase(h.chunks, embeddingStore(h), embedder, "test/model")
	results, err := hybrid.Search(ctx, "aggregate", app.DefaultSearchLimit)
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, domain.StageLexical, results[0].Stage)
	assert.Negative(t, results[0].Score)
	assert.Equal(t, 0, embedder.calls,
		"no stored vector means no query to embed and no network call")
}

// PRESERVE: the lexical-only use case from slice 5.1 keeps working exactly as it
// did — same ranking, same marked snippet — and now names its stage.
func TestSearchUseCase_LexicalOnlyKeepsItsRankingAndSnippet(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	document := h.seedRecordedPages(t, "a", map[int]string{1: "aggregate root"})
	_, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	results, err := app.NewSearchUseCase(h.chunks).Search(ctx, "aggregate", app.DefaultSearchLimit)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, domain.StageLexical, results[0].Stage)
	assert.Contains(t, results[0].Snippet, "[aggregate]")
	assert.Negative(t, results[0].Score, "bm25 is still smaller-is-better")
	assert.Equal(t, 1, results[0].PageNumber)
}

// The fusion is observable: a chunk both stages find outranks one only a single
// stage finds, and it says so.
func TestSearchUseCase_ChunkFoundByBothStagesIsHybrid(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	embedder := spanishToEnglishEmbedder()

	indexAndEmbed(t, h, embedder, "test/model", "a", map[int]string{
		1: "the aggregate is the consistency boundary",
	})

	hybrid := app.NewHybridSearchUseCase(h.chunks, embeddingStore(h), embedder, "test/model")
	results, err := hybrid.Search(ctx, "aggregate", app.DefaultSearchLimit)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, domain.StageHybrid, results[0].Stage,
		"a chunk found by both stages must say so rather than claim one of them")
	assert.Greater(t, results[0].Similarity, 0.0)
	assert.Negative(t, results[0].Score)
	assert.Contains(t, results[0].Snippet, "[aggregate]",
		"a hybrid result keeps the lexical snippet, which marks the matched term")
}

// A query of only punctuation has no meaning to embed. Embedding it and returning
// the model's nearest neighbour would break the guarantee that such a query
// returns no results.
func TestSearchUseCase_PunctuationOnlyQueryIsEmptyAndEmbedsNothing(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	embedder := spanishToEnglishEmbedder()

	indexAndEmbed(t, h, embedder, "test/model", "a", map[int]string{
		1: "the aggregate is the consistency boundary",
	})
	embedder.calls = 0

	hybrid := app.NewHybridSearchUseCase(h.chunks, embeddingStore(h), embedder, "test/model")
	for _, query := range []string{"...", "??? !!! —", "", "   "} {
		t.Run(query, func(t *testing.T) {
			results, err := hybrid.Search(ctx, query, app.DefaultSearchLimit)
			require.NoError(t, err)
			assert.NotNil(t, results)
			assert.Empty(t, results)
		})
	}

	assert.Equal(t, 0, embedder.calls, "a query with no word in it has nothing to embed")
}

// The boundary of the semantic ranking: a vector with no shared direction is not a
// match, so an unrelated query does not return every chunk in the index.
func TestSearchUseCase_ASemanticallyUnrelatedQueryIsEmpty(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	embedder := spanishToEnglishEmbedder()

	indexAndEmbed(t, h, embedder, "test/model", "a", map[int]string{
		1: "the aggregate is the consistency boundary",
	})

	hybrid := app.NewHybridSearchUseCase(h.chunks, embeddingStore(h), embedder, "test/model")
	results, err := hybrid.Search(ctx, "zzzznomatchqq", app.DefaultSearchLimit)
	require.NoError(t, err)
	assert.NotNil(t, results)
	assert.Empty(t, results, "an orthogonal vector is no similarity, not a weak match")
}

func TestSearchUseCase_QueryEmbeddingFailureIsReported(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	document := h.seedRecordedPages(t, "a", map[int]string{1: "the aggregate boundary"})
	_, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)
	_, err = embedderFor(h, spanishToEnglishEmbedder(), "test/model").Execute(ctx, document.ID)
	require.NoError(t, err)

	failing := spanishToEnglishEmbedder()
	failing.err = errors.New("the network is down")

	hybrid := app.NewHybridSearchUseCase(h.chunks, embeddingStore(h), failing, "test/model")
	_, err = hybrid.Search(ctx, "agregado", app.DefaultSearchLimit)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network is down")
}

func TestSearchUseCase_FusionRespectsTheLimit(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()
	embedder := spanishToEnglishEmbedder()

	indexAndEmbed(t, h, embedder, "test/model", "a", map[int]string{
		1: "the aggregate is the consistency boundary",
		2: "a repository gives the illusion of a collection",
		3: "another page about a different topic",
	})

	hybrid := app.NewHybridSearchUseCase(h.chunks, embeddingStore(h), embedder, "test/model")
	results, err := hybrid.Search(ctx, "aggregate repository", 1)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}
