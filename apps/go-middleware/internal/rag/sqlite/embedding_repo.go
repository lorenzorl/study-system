package sqlite

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// storedEmbeddingsSQL reads every vector of one model with its chunk's citation
// and text.
//
// Filtering by model is the whole point of storing the model with the vector: a
// vector produced by another model is not comparable with the current query
// vector, and comparing it anyway would return a similarity that means nothing.
// The order is deterministic so a brute-force cosine over equal similarities
// always breaks the tie the same way.
const storedEmbeddingsSQL = `
SELECT c.id,
       c.document_id,
       d.title,
       c.page_number,
       c.text,
       e.dimensions,
       e.vector
FROM chunk_embeddings e
JOIN chunks c ON c.id = e.chunk_id
JOIN documents d ON d.id = c.document_id
WHERE e.model = ?
ORDER BY c.document_id ASC, c.page_number ASC, c.ordinal ASC`

// EmbeddingRepository implements domain.EmbeddingRepository using SQLite.
type EmbeddingRepository struct {
	db *sql.DB
}

// NewEmbeddingRepository creates an EmbeddingRepository backed by the given
// database.
func NewEmbeddingRepository(db *sql.DB) *EmbeddingRepository {
	return &EmbeddingRepository{db: db}
}

// ReplaceDocumentEmbeddings deletes the document's stored vectors and writes the
// given ones in their place, inside one transaction.
//
// The delete is scoped to the document and not to the model, so changing the
// model replaces the document's vectors instead of leaving the previous model's
// rows to accumulate. The alternative — keeping them — would let a search with
// the current model still return the old model's citations from a table nobody
// prunes.
func (r *EmbeddingRepository) ReplaceDocumentEmbeddings(
	ctx context.Context,
	documentID, model string,
	embeddings []domain.ChunkEmbedding,
	now time.Time,
) error {
	if model == "" {
		return fmt.Errorf("embedding model must not be empty: %w", apperror.ErrInvalidInput)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin embedding replace of document %s: %w", documentID, err)
	}
	// Rollback after a successful commit is a no-op, so the deferred call is
	// unconditional and every early return is covered.
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM chunk_embeddings WHERE chunk_id IN (SELECT id FROM chunks WHERE document_id = ?)`,
		documentID,
	); err != nil {
		return fmt.Errorf("delete the embeddings of document %s: %w", documentID, err)
	}

	insert, err := tx.PrepareContext(ctx,
		`INSERT INTO chunk_embeddings (chunk_id, model, dimensions, vector, created_at)
		 VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare embedding insert: %w", err)
	}
	defer insert.Close()

	createdAt := formatInstant(now)
	for _, embedding := range embeddings {
		if len(embedding.Vector) == 0 {
			return fmt.Errorf("the embedding of chunk %s is empty: %w",
				embedding.ChunkID, apperror.ErrInvalidInput)
		}

		if _, err := insert.ExecContext(ctx,
			embedding.ChunkID, model, len(embedding.Vector), encodeVector(embedding.Vector), createdAt,
		); err != nil {
			if isForeignKeyConstraintError(err) {
				return apperror.ErrNotFound
			}
			return fmt.Errorf("insert the embedding of chunk %s: %w", embedding.ChunkID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit embedding replace of document %s: %w", documentID, err)
	}
	return nil
}

// ListByModel returns every stored vector of model with its citation and text.
//
// A blob whose length is not dimensions*4 is an error rather than a partial
// decode: half a vector compared against a real one is a similarity that looks
// valid and is not.
func (r *EmbeddingRepository) ListByModel(ctx context.Context, model string) ([]domain.StoredEmbedding, error) {
	stored := []domain.StoredEmbedding{}

	rows, err := r.db.QueryContext(ctx, storedEmbeddingsSQL, model)
	if err != nil {
		return nil, fmt.Errorf("list stored embeddings for model %s: %w", model, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			embedding  domain.StoredEmbedding
			dimensions int
			blob       []byte
		)

		if err := rows.Scan(
			&embedding.ChunkID,
			&embedding.DocumentID,
			&embedding.DocumentTitle,
			&embedding.PageNumber,
			&embedding.Text,
			&dimensions,
			&blob,
		); err != nil {
			return nil, fmt.Errorf("scan stored embedding: %w", err)
		}

		vector, err := decodeVector(blob, dimensions)
		if err != nil {
			return nil, fmt.Errorf("read the embedding of chunk %s: %w", embedding.ChunkID, err)
		}
		embedding.Vector = vector

		stored = append(stored, embedding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stored embeddings: %w", err)
	}

	return stored, nil
}

// encodeVector writes a vector as float32, little-endian, dimensions*4 bytes.
func encodeVector(vector []float32) []byte {
	blob := make([]byte, len(vector)*4)
	for i, value := range vector {
		binary.LittleEndian.PutUint32(blob[i*4:], math.Float32bits(value))
	}
	return blob
}

// decodeVector reads a vector written by encodeVector, rejecting a blob whose
// length disagrees with the stored dimension.
func decodeVector(blob []byte, dimensions int) ([]float32, error) {
	if dimensions < 1 {
		return nil, fmt.Errorf("stored dimensions must be positive, got %d: %w",
			dimensions, apperror.ErrInvalidInput)
	}
	if len(blob) != dimensions*4 {
		return nil, fmt.Errorf("stored vector is %d bytes but %d dimensions need %d: %w",
			len(blob), dimensions, dimensions*4, apperror.ErrInvalidInput)
	}

	vector := make([]float32, dimensions)
	for i := range vector {
		vector[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return vector, nil
}
