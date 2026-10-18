package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// searchSQL is the one ranked search.
//
// Some choices here are not free:
//
//   - snippet(chunks_fts, 1, ...) reads column 1 because chunks_fts declares
//     chunk_id first. The window is 12 tokens, and the markers are ASCII so they
//     survive any output channel. The result is folded onto one line because the
//     command prints one line per result, and a chunk whose snippet crossed a
//     paragraph boundary would otherwise break that line in half.
//   - The joins are what make a result citable: FTS5 knows the chunk id, and
//     chunks holds the page and documents the title.
//   - bm25() returns SMALLER-IS-BETTER scores (negative), so the order is
//     ascending. Ordering descending would present the worst match first while
//     looking, to anyone reading the SQL, exactly like the best one.
//   - The tiebreaker is the citation order, so the same index always answers the
//     same query the same way.
const searchSQL = `
SELECT c.id,
       c.document_id,
       d.title,
       c.page_number,
       replace(replace(replace(snippet(chunks_fts, 1, '[', ']', '…', 12), char(10), ' '), char(13), ' '), char(9), ' '),
       bm25(chunks_fts)
FROM chunks_fts
JOIN chunks c ON c.id = chunks_fts.chunk_id
JOIN documents d ON d.id = c.document_id
WHERE chunks_fts MATCH ?
ORDER BY bm25(chunks_fts) ASC, c.document_id ASC, c.page_number ASC, c.ordinal ASC
LIMIT ?`

// ChunkRepository implements domain.ChunkRepository using SQLite.
type ChunkRepository struct {
	db *sql.DB
}

// NewChunkRepository creates a ChunkRepository backed by the given database.
func NewChunkRepository(db *sql.DB) *ChunkRepository {
	return &ChunkRepository{db: db}
}

// ReplaceDocumentChunks deletes the document's chunks and writes the given ones
// in their place, inside one transaction.
//
// Both tables are written together in that transaction because chunks_fts holds
// a copy: a commit between them would leave either a chunk nobody can find or a
// term matching a chunk that does not exist, and neither has a repair path short
// of another reindex.
//
// Chunk ids are minted here. They are storage identity — the citation a result
// carries — and not a property of the text, so nothing above this adapter has to
// thread an identifier through the chunker.
func (r *ChunkRepository) ReplaceDocumentChunks(
	ctx context.Context,
	documentID, jobID string,
	chunks []domain.Chunk,
	now time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin chunk replace of document %s: %w", documentID, err)
	}
	// Rollback after a successful commit is a no-op, so the deferred call is
	// unconditional and every early return is covered.
	defer func() { _ = tx.Rollback() }()

	if err := deleteChunks(ctx, tx, documentID); err != nil {
		return err
	}

	insertChunk, err := tx.PrepareContext(ctx,
		`INSERT INTO chunks (id, document_id, job_id, page_number, ordinal, text, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare chunk insert: %w", err)
	}
	defer insertChunk.Close()

	insertIndex, err := tx.PrepareContext(ctx,
		`INSERT INTO chunks_fts (chunk_id, text) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare chunk index insert: %w", err)
	}
	defer insertIndex.Close()

	createdAt := formatInstant(now)
	for _, chunk := range chunks {
		chunkID := uuid.NewString()

		if _, err := insertChunk.ExecContext(ctx,
			chunkID, documentID, jobID, chunk.PageNumber, chunk.Ordinal, chunk.Text, createdAt,
		); err != nil {
			if isForeignKeyConstraintError(err) {
				return apperror.ErrNotFound
			}
			return fmt.Errorf("insert chunk %d of page %d: %w", chunk.Ordinal, chunk.PageNumber, err)
		}

		if _, err := insertIndex.ExecContext(ctx, chunkID, chunk.Text); err != nil {
			return fmt.Errorf("index chunk %d of page %d: %w", chunk.Ordinal, chunk.PageNumber, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit chunk replace of document %s: %w", documentID, err)
	}
	return nil
}

// deleteChunks removes a document's chunks from both tables, the FTS copy first.
//
// The order is the whole point: chunks_fts is keyed by chunk_id alone, so the ids
// have to be read while the chunk rows still exist. Deleting the chunks first
// would leave index rows that no join can reach and no delete can select.
func deleteChunks(ctx context.Context, tx *sql.Tx, documentID string) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM chunks_fts WHERE chunk_id IN (SELECT id FROM chunks WHERE document_id = ?)`,
		documentID,
	); err != nil {
		return fmt.Errorf("delete the index rows of document %s: %w", documentID, err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM chunks WHERE document_id = ?`,
		documentID,
	); err != nil {
		return fmt.Errorf("delete the chunks of document %s: %w", documentID, err)
	}

	return nil
}

// Search returns the chunks matching query, best first.
//
// A limit below 1 is rejected rather than passed through: SQLite reads a negative
// LIMIT as "no limit", so the one value a caller is most likely to pass by
// accident would be the one that returns the whole index.
func (r *ChunkRepository) Search(ctx context.Context, query string, limit int) ([]domain.SearchResult, error) {
	results := []domain.SearchResult{}

	match := matchQuery(query)
	if match == "" {
		// Nothing to look for. Returning the empty list instead of handing MATCH
		// a phrase with no term is what keeps "..." a no-match rather than an
		// error in front of the user.
		return results, nil
	}
	if limit < 1 {
		return nil, fmt.Errorf("search limit must be positive, got %d: %w", limit, apperror.ErrInvalidInput)
	}

	rows, err := r.db.QueryContext(ctx, searchSQL, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var result domain.SearchResult
		if err := rows.Scan(
			&result.ChunkID,
			&result.DocumentID,
			&result.DocumentTitle,
			&result.PageNumber,
			&result.Snippet,
			&result.Score,
		); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}

	return results, nil
}

// ListDocumentChunks returns the document's chunks in index order, with their
// ids and text, or an empty slice when it has none.
//
// The order is page then ordinal, which is the order the chunks were written in,
// so a caller that pairs the result with vectors positionally pairs them with
// the right chunks.
func (r *ChunkRepository) ListDocumentChunks(ctx context.Context, documentID string) ([]domain.StoredChunk, error) {
	chunks := []domain.StoredChunk{}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, text FROM chunks WHERE document_id = ? ORDER BY page_number ASC, ordinal ASC`,
		documentID)
	if err != nil {
		return nil, fmt.Errorf("list the chunks of document %s: %w", documentID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var chunk domain.StoredChunk
		if err := rows.Scan(&chunk.ID, &chunk.Text); err != nil {
			return nil, fmt.Errorf("scan chunk of document %s: %w", documentID, err)
		}
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate the chunks of document %s: %w", documentID, err)
	}

	return chunks, nil
}

// CountByDocument returns how many chunks the document has, without reading any
// of their text.
//
// The count is answered by the chunks primary key, so it stays a single index
// scan however large the document is. An unknown document counts zero.
func (r *ChunkRepository) CountByDocument(ctx context.Context, documentID string) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx,
		`SELECT count(*) FROM chunks WHERE document_id = ?`,
		documentID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count the chunks of document %s: %w", documentID, err)
	}
	return count, nil
}

// matchQuery turns the user's text into an FTS5 MATCH expression.
//
// FTS5's MATCH grammar is a small query language — quotes, star, caret, NEAR,
// AND/OR/NOT, column filters — and a search box is not. Raw input reaching MATCH
// would let a stray quote change the query's meaning or raise a syntax error in
// front of the user, so each whitespace-separated token becomes one quoted
// literal term, embedded quotes are doubled, and the terms are joined with OR.
// A chunk that matches more terms then ranks better through bm25, while a chunk
// that matches one term is still returned.
//
// A token with no letter and no digit — "...", "???" — is dropped: quoting it
// would leave a phrase with nothing to look for. When every token is dropped the
// expression is empty, and the caller answers with no results instead of asking
// MATCH to match nothing.
func matchQuery(query string) string {
	terms := make([]string, 0)
	for _, token := range strings.Fields(query) {
		if !domain.HasIndexableText(token) {
			continue
		}
		terms = append(terms, `"`+strings.ReplaceAll(token, `"`, `""`)+`"`)
	}

	return strings.Join(terms, " OR ")
}
