package app_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// textModel is the model the text harness stores vectors under. It is a
// constant so a test never pins an instant or a model name it cannot predict.
const textModel = "test/model"

// textHarness wires the pasted-text path over the real repositories, a real
// store rooted in the test's temporary directory, and the real read paths, so
// every assertion goes through a public use case or the store's own API.
type textHarness struct {
	*indexHarness

	store  *storage.Store
	ingest *app.IngestTextUseCase
	list   *app.ListDocumentsUseCase
	status *app.DocumentStatusUseCase
}

func newTextHarness(t *testing.T) *textHarness {
	t.Helper()

	base := newIndexHarness(t)
	store, err := storage.New(t.TempDir(), 1<<20)
	require.NoError(t, err)

	return &textHarness{
		indexHarness: base,
		store:        store,
		ingest:       app.NewIngestTextUseCase(base.docs, base.jobs, base.pages, store, base.indexer),
		list:         app.NewListDocumentsUseCase(base.docs, base.pages, base.chunks),
		status: app.NewDocumentStatusUseCase(
			base.docs, base.jobs, base.pages, base.chunks, embeddingStore(base), nil, textModel),
	}
}

// withEmbedder adds the embedding stage the composition root wires when a client
// exists.
func (h *textHarness) withEmbedder(embedder domain.Embedder) *textHarness {
	h.ingest = h.ingest.WithEmbed(embedderFor(h.indexHarness, embedder, textModel))
	return h
}

// T3/S4: a pasted text becomes a corpus document: it is stored, listed, and a
// search for a word it carries finds it — all through the public read paths.
func TestIngestTextUseCase_StoresAListedSearchableDocument(t *testing.T) {
	h := newTextHarness(t)
	ctx := t.Context()

	report, err := h.ingest.Execute(ctx, app.IngestTextRequest{
		Title: "  Lecture notes  ",
		Text:  "\n  El agregado es la raíz de la consistencia del modelo.  \n",
	})
	require.NoError(t, err)
	require.NotEmpty(t, report.DocumentID)
	assert.Positive(t, report.Chunks, "a pasted text produces at least one chunk")
	assert.Zero(t, report.Embedded, "without an embedding client no vector is written")
	assert.Equal(t, app.StateIndexing, report.State,
		"indexed with no vectors is the state the status read derives for this corpus")

	document, err := h.docs.FindByID(ctx, report.DocumentID)
	require.NoError(t, err)
	assert.Equal(t, "Lecture notes", document.Title, "the title is trimmed before it is recorded")

	payload, err := h.store.Open(ctx, document.ArtifactID)
	require.NoError(t, err)
	t.Cleanup(func() { payload.Close() })
	stored, err := io.ReadAll(payload)
	require.NoError(t, err)
	assert.Equal(t, "El agregado es la raíz de la consistencia del modelo.", string(stored),
		"the pasted text is what the artifact store holds, trimmed at the edges only")

	summaries, err := h.list.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, report.DocumentID, summaries[0].ID)
	assert.Equal(t, "Lecture notes", summaries[0].Title)
	assert.Equal(t, 1, summaries[0].Pages, "a pasted text is one page")
	assert.Equal(t, report.Chunks, summaries[0].Chunks)

	results, err := h.searcher.Search(ctx, "consistencia", 10)
	require.NoError(t, err)
	require.NotEmpty(t, results, "a word the pasted text carries finds its chunk")
	assert.Equal(t, report.DocumentID, results[0].DocumentID)
	assert.Equal(t, 1, results[0].PageNumber)

	status, err := h.status.Execute(ctx, report.DocumentID)
	require.NoError(t, err)
	assert.Equal(t, 1, status.Pages.Total)
	assert.Equal(t, 1, status.Pages.Extracted, "the text is already available")
	assert.Zero(t, status.Pages.Transcribed, "a pasted text needs no model to be available")
	assert.Equal(t, report.Chunks, status.Chunks)
}

// T3/S4: content_sha256 is UNIQUE, so the same material twice is a conflict and
// the corpus keeps one document.
func TestIngestTextUseCase_TheSameTextTwiceIsAConflict(t *testing.T) {
	h := newTextHarness(t)
	ctx := t.Context()

	text := "el repositorio da la ilusión de una colección de objetos"
	first, err := h.ingest.Execute(ctx, app.IngestTextRequest{Title: "Notes", Text: text})
	require.NoError(t, err)

	// The edges differ, so only trimming makes these the same material.
	second, err := h.ingest.Execute(ctx, app.IngestTextRequest{
		Title: "Notes again",
		Text:  "\n\n  " + text + "  \n",
	})
	require.ErrorIs(t, err, apperror.ErrConflict)
	assert.Empty(t, second.DocumentID)

	summaries, err := h.list.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, summaries, 1, "the corpus holds one document, not two")
	assert.Equal(t, first.DocumentID, summaries[0].ID)
	assert.Equal(t, first.Chunks, summaries[0].Chunks, "the refused paste adds no chunk")
}

// T3/S4: a blank paste is refused by the use case too, so no transport is the
// only thing standing between the corpus and an empty document.
func TestIngestTextUseCase_RejectsBlankInput(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request app.IngestTextRequest
	}{
		{"blank title", app.IngestTextRequest{Title: "   ", Text: "some material"}},
		{"blank text", app.IngestTextRequest{Title: "Notes", Text: " \n\t "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newTextHarness(t)
			ctx := t.Context()

			_, err := h.ingest.Execute(ctx, tc.request)
			require.ErrorIs(t, err, apperror.ErrInvalidInput)

			summaries, err := h.list.Execute(ctx)
			require.NoError(t, err)
			assert.Empty(t, summaries, "a refused paste creates no document")

			artifacts, err := h.store.List()
			require.NoError(t, err)
			assert.Empty(t, artifacts, "a refused paste stores no bytes")
		})
	}
}

// T3/S4: with an embedding client the same paste is indexed and embedded, which
// is the other answer the response has to be able to report.
func TestIngestTextUseCase_EmbedsWhenAClientIsWired(t *testing.T) {
	h := newTextHarness(t).withEmbedder(spanishToEnglishEmbedder())
	ctx := t.Context()

	report, err := h.ingest.Execute(ctx, app.IngestTextRequest{
		Title: "Notes",
		Text:  "el agregado es la raíz de la consistencia",
	})
	require.NoError(t, err)

	assert.Positive(t, report.Chunks)
	assert.Equal(t, report.Chunks, report.Embedded, "every chunk gets a vector")
	assert.Equal(t, app.StateCompleted, report.State)

	results, err := h.searcher.Search(ctx, "consistencia", 10)
	require.NoError(t, err)
	require.NotEmpty(t, results)
	assert.Equal(t, report.DocumentID, results[0].DocumentID)
}
