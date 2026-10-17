package sqlite_test

import (
	"context"
	"database/sql"
	"encoding/binary"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
)

// embeddingRows counts the stored vectors directly, so "the same rows" is a
// statement about the table and not about what a query chose to return.
func embeddingRows(t *testing.T, sqlDB *sql.DB) int {
	t.Helper()

	var count int
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM chunk_embeddings`).Scan(&count))
	return count
}

// embeddingsFor pairs every chunk of the document, in index order, with the
// given vector.
func embeddingsFor(t *testing.T, ctx context.Context, repos ragRepos, documentID string, vector []float32) []domain.ChunkEmbedding {
	t.Helper()

	chunks, err := repos.chunks.ListDocumentChunks(ctx, documentID)
	require.NoError(t, err)

	embeddings := make([]domain.ChunkEmbedding, 0, len(chunks))
	for _, chunk := range chunks {
		embeddings = append(embeddings, domain.ChunkEmbedding{ChunkID: chunk.ID, Vector: vector})
	}
	return embeddings
}

func TestChunkRepository_ListDocumentChunksReturnsTheIndexOrder(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()

	document := indexedDocument(t, ctx, repos, "un libro", map[int][]string{
		1: {"primera página, primer trozo", "primera página, segundo trozo"},
		2: {"segunda página"},
	})

	chunks, err := repos.chunks.ListDocumentChunks(ctx, document.ID)
	require.NoError(t, err)
	require.Len(t, chunks, 3)

	assert.Equal(t, "primera página, primer trozo", chunks[0].Text)
	assert.Equal(t, "primera página, segundo trozo", chunks[1].Text)
	assert.Equal(t, "segunda página", chunks[2].Text)

	for _, chunk := range chunks {
		assert.NotEmpty(t, chunk.ID)
	}
}

func TestChunkRepository_ListDocumentChunksOfAnUnknownDocumentIsEmpty(t *testing.T) {
	_, repos := openRepos(t)

	chunks, err := repos.chunks.ListDocumentChunks(t.Context(), uuid.NewString())
	require.NoError(t, err)
	assert.NotNil(t, chunks, "an empty result must be an empty slice, not nil")
	assert.Empty(t, chunks)
}

func TestEmbeddingRepository_RoundTripsVectorsWithTheCitation(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	document := indexedDocument(t, ctx, repos, "Domain-Driven Design", map[int][]string{
		3: {"the aggregate is the consistency boundary"},
		4: {"a repository gives the illusion of an in-memory collection"},
	})
	store := ragsqlite.NewEmbeddingRepository(sqlDB)

	embeddings := embeddingsFor(t, ctx, repos, document.ID, []float32{0.25, -0.5, 1})
	require.NoError(t, store.ReplaceDocumentEmbeddings(ctx, document.ID, "model-a", embeddings, instant))

	stored, err := store.ListByModel(ctx, "model-a")
	require.NoError(t, err)
	require.Len(t, stored, 2, "every chunk's vector must be readable")

	assert.Equal(t, document.ID, stored[0].DocumentID)
	assert.Equal(t, "Domain-Driven Design", stored[0].DocumentTitle,
		"the read carries the citation, so a semantic result needs no second lookup")
	assert.Equal(t, 3, stored[0].PageNumber)
	assert.Equal(t, "the aggregate is the consistency boundary", stored[0].Text)
	assert.Equal(t, []float32{0.25, -0.5, 1}, stored[0].Vector)
	assert.Equal(t, 4, stored[1].PageNumber)
}

// Acceptance criterion 4: indexing the same document's embeddings twice leaves
// the same rows.
func TestEmbeddingRepository_ReplaceLeavesTheSameRowsWhenRunTwice(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	document := indexedDocument(t, ctx, repos, "un libro", map[int][]string{
		1: {"el agregado es la raíz"},
		2: {"el repositorio guarda objetos"},
	})
	store := ragsqlite.NewEmbeddingRepository(sqlDB)

	embeddings := embeddingsFor(t, ctx, repos, document.ID, []float32{1, 2})
	require.NoError(t, store.ReplaceDocumentEmbeddings(ctx, document.ID, "model-a", embeddings, instant))
	before := embeddingRows(t, sqlDB)

	require.NoError(t, store.ReplaceDocumentEmbeddings(ctx, document.ID, "model-a", embeddings, instant))

	assert.Equal(t, before, embeddingRows(t, sqlDB), "a replace must not append a second set")
	assert.Equal(t, 2, before)
}

func TestEmbeddingRepository_ReplaceWithNothingRemovesTheRows(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	document := indexedDocument(t, ctx, repos, "un libro", map[int][]string{
		1: {"el agregado es la raíz"},
	})
	store := ragsqlite.NewEmbeddingRepository(sqlDB)

	require.NoError(t, store.ReplaceDocumentEmbeddings(ctx, document.ID, "model-a",
		embeddingsFor(t, ctx, repos, document.ID, []float32{1}), instant))
	require.Equal(t, 1, embeddingRows(t, sqlDB))

	require.NoError(t, store.ReplaceDocumentEmbeddings(ctx, document.ID, "model-a", nil, instant))

	assert.Equal(t, 0, embeddingRows(t, sqlDB))
	stored, err := store.ListByModel(ctx, "model-a")
	require.NoError(t, err)
	assert.NotNil(t, stored)
	assert.Empty(t, stored)
}

// Acceptance criterion 5: a vector from another model is not mixed into a search.
func TestEmbeddingRepository_ListByModelIsScopedToOneModel(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	first := indexedDocument(t, ctx, repos, "a", map[int][]string{1: {"alpha"}})
	second := indexedDocument(t, ctx, repos, "b", map[int][]string{1: {"beta"}})
	store := ragsqlite.NewEmbeddingRepository(sqlDB)

	require.NoError(t, store.ReplaceDocumentEmbeddings(ctx, first.ID, "model-a",
		embeddingsFor(t, ctx, repos, first.ID, []float32{1}), instant))
	require.NoError(t, store.ReplaceDocumentEmbeddings(ctx, second.ID, "model-b",
		embeddingsFor(t, ctx, repos, second.ID, []float32{2}), instant))

	stored, err := store.ListByModel(ctx, "model-a")
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, first.ID, stored[0].DocumentID)

	other, err := store.ListByModel(ctx, "model-b")
	require.NoError(t, err)
	require.Len(t, other, 1)
	assert.Equal(t, second.ID, other[0].DocumentID)
}

// A model change must not leave the old model's vectors behind to be compared
// against the new query vector.
func TestEmbeddingRepository_ReplacingWithAnotherModelDropsTheOldVectors(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	document := indexedDocument(t, ctx, repos, "un libro", map[int][]string{
		1: {"el agregado es la raíz"},
	})
	store := ragsqlite.NewEmbeddingRepository(sqlDB)

	embeddings := embeddingsFor(t, ctx, repos, document.ID, []float32{1, 2})
	require.NoError(t, store.ReplaceDocumentEmbeddings(ctx, document.ID, "model-a", embeddings, instant))
	require.NoError(t, store.ReplaceDocumentEmbeddings(ctx, document.ID, "model-b", embeddings, instant))

	old, err := store.ListByModel(ctx, "model-a")
	require.NoError(t, err)
	assert.Empty(t, old, "the previous model's vectors must be gone")

	current, err := store.ListByModel(ctx, "model-b")
	require.NoError(t, err)
	assert.Len(t, current, 1)
	assert.Equal(t, 1, embeddingRows(t, sqlDB), "the rows are replaced, not added to")
}

// The encoding is the contract: float32, little-endian, dimensions*4 bytes.
func TestEmbeddingRepository_StoresFloat32LittleEndianBlobs(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	document := indexedDocument(t, ctx, repos, "un libro", map[int][]string{1: {"el agregado"}})
	store := ragsqlite.NewEmbeddingRepository(sqlDB)

	vector := []float32{1.5, -2.25, 0.125}
	require.NoError(t, store.ReplaceDocumentEmbeddings(ctx, document.ID, "model-a",
		embeddingsFor(t, ctx, repos, document.ID, vector), instant))

	var blob []byte
	var dimensions int
	require.NoError(t, sqlDB.QueryRow(
		`SELECT vector, dimensions FROM chunk_embeddings`).Scan(&blob, &dimensions))

	assert.Equal(t, len(vector), dimensions)
	require.Len(t, blob, dimensions*4, "the blob is exactly dimensions * 4 bytes")
	for i, want := range vector {
		got := math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
		assert.Equal(t, want, got, "component %d must decode from little-endian float32", i)
	}
}

func TestEmbeddingRepository_ABlobOfTheWrongLengthIsAnError(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	document := indexedDocument(t, ctx, repos, "un libro", map[int][]string{1: {"el agregado"}})
	store := ragsqlite.NewEmbeddingRepository(sqlDB)

	chunks, err := repos.chunks.ListDocumentChunks(ctx, document.ID)
	require.NoError(t, err)
	require.Len(t, chunks, 1)

	// dimensions says 2, the blob holds one float: a silent decode would read
	// half a vector and compare it against a real one.
	_, err = sqlDB.Exec(
		`INSERT INTO chunk_embeddings (chunk_id, model, dimensions, vector, created_at)
		 VALUES (?, 'model-a', 2, ?, '2026-10-17T12:00:00Z')`,
		chunks[0].ID, []byte{0, 0, 0, 0})
	require.NoError(t, err)

	_, err = store.ListByModel(ctx, "model-a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dimensions")
}

func TestEmbeddingRepository_UnknownChunkIsNotFound(t *testing.T) {
	sqlDB, _ := openRepos(t)
	store := ragsqlite.NewEmbeddingRepository(sqlDB)

	err := store.ReplaceDocumentEmbeddings(t.Context(), uuid.NewString(), "model-a",
		[]domain.ChunkEmbedding{{ChunkID: uuid.NewString(), Vector: []float32{1}}}, instant)
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

// The foreign key is ON DELETE CASCADE, so reindexing a document — which replaces
// its chunks and therefore their ids — cannot leave a vector pointing at a chunk
// that no longer exists.
func TestEmbeddingRepository_DeletingTheChunksDeletesTheirVectors(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()

	document := indexedDocument(t, ctx, repos, "un libro", map[int][]string{1: {"el agregado"}})
	store := ragsqlite.NewEmbeddingRepository(sqlDB)

	require.NoError(t, store.ReplaceDocumentEmbeddings(ctx, document.ID, "model-a",
		embeddingsFor(t, ctx, repos, document.ID, []float32{1}), instant))
	require.Equal(t, 1, embeddingRows(t, sqlDB))

	job, err := repos.jobs.FindLatestByDocumentID(ctx, document.ID)
	require.NoError(t, err)
	require.NoError(t, repos.chunks.ReplaceDocumentChunks(ctx, document.ID, job.ID, nil, instant))

	assert.Equal(t, 0, embeddingRows(t, sqlDB),
		"a vector must never outlive the chunk it describes")
}
