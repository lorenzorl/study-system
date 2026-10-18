package app_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// T7/S3: the corpus list carries the id, the title, the creation instant and the
// two counts a caller needs to choose a document to attach.
func TestListDocumentsUseCase_CountsPagesAndChunks(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	document := h.seedRecordedPages(t, "Domain-Driven Design", map[int]string{
		1: "the aggregate is the consistency boundary",
		2: "a repository gives the illusion of a collection",
	})
	_, err := h.indexer.Execute(ctx, document.ID)
	require.NoError(t, err)

	useCase := app.NewListDocumentsUseCase(h.docs, h.pages, h.chunks)
	summaries, err := useCase.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, summaries, 1)

	assert.Equal(t, document.ID, summaries[0].ID)
	assert.Equal(t, "Domain-Driven Design", summaries[0].Title)
	assert.True(t, indexInstant.Equal(summaries[0].CreatedAt))
	assert.Equal(t, 2, summaries[0].Pages, "pages is the document's recorded page count")
	assert.Equal(t, 2, summaries[0].Chunks, "chunks is the document's chunk count")
}

// T7: an empty corpus is an empty slice, never nil — the consumer ranges over it.
func TestListDocumentsUseCase_EmptyCorpusIsAnEmptySlice(t *testing.T) {
	h := newIndexHarness(t)

	summaries, err := app.NewListDocumentsUseCase(h.docs, h.pages, h.chunks).Execute(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, summaries, "an empty corpus is a slice, never nil")
	assert.Empty(t, summaries)
}

// T7: the order is derived from the data alone — created_at descending, then id
// ascending — and never from insertion order.
func TestListDocumentsUseCase_OrdersByCreatedAtDescendingThenID(t *testing.T) {
	h := newIndexHarness(t)
	ctx := t.Context()

	base := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

	create := func(id string, createdAt time.Time) {
		t.Helper()
		require.NoError(t, h.docs.Create(ctx, &domain.Document{
			ID:            id,
			Title:         id,
			ArtifactID:    "artifact-" + id,
			SourceURI:     "/books/" + id,
			ContentSHA256: sha256Hex([]byte(id)),
			CreatedAt:     createdAt,
		}))
	}

	// Inserted out of the expected order on purpose: "older" first, then the two
	// documents that share an instant, so insertion order cannot be what the
	// assertion passes on.
	create("older", base.Add(-time.Hour))
	create("b", base)
	create("a", base)

	summaries, err := app.NewListDocumentsUseCase(h.docs, h.pages, h.chunks).Execute(ctx)
	require.NoError(t, err)
	require.Len(t, summaries, 3)

	ids := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		ids = append(ids, summary.ID)
	}
	assert.Equal(t, []string{"a", "b", "older"}, ids,
		"created_at descending, then id ascending to break the tie")
}
