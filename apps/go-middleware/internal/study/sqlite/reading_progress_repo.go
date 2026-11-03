package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// ReadingProgressRepository implements domain.ReadingProgressRepository over
// SQLite.
type ReadingProgressRepository struct {
	db *sql.DB
}

// NewReadingProgressRepository creates a ReadingProgressRepository backed by the
// given database.
func NewReadingProgressRepository(db *sql.DB) *ReadingProgressRepository {
	return &ReadingProgressRepository{db: db}
}

// Upsert writes the state of one range and returns the row as stored.
//
// The conflict target is the range itself, so marking the same chapter again is a
// state change and not a second row. Only `state` and `updated_at` are overwritten:
// `id` and `created_at` belong to the first marking, which is also why the row is
// read back inside the transaction instead of being echoed from the input.
func (r *ReadingProgressRepository) Upsert(
	ctx context.Context,
	progress *domain.ReadingProgress,
) (*domain.ReadingProgress, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO reading_progress (id, document_id, from_page, to_page, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (document_id, from_page, to_page) DO UPDATE SET
			state = excluded.state,
			updated_at = excluded.updated_at`,
		progress.ID,
		progress.DocumentID,
		progress.FromPage,
		progress.ToPage,
		string(progress.State),
		progress.CreatedAt.UTC().Format(time.RFC3339),
		progress.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("upsert reading_progress: %w", err)
	}

	stored, err := scanReadingProgress(tx.QueryRowContext(ctx,
		`SELECT id, document_id, from_page, to_page, state, created_at, updated_at
		 FROM reading_progress
		 WHERE document_id = ? AND from_page = ? AND to_page = ?`,
		progress.DocumentID, progress.FromPage, progress.ToPage,
	))
	if err != nil {
		return nil, fmt.Errorf("read back reading_progress: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return stored, nil
}

// ListByDocument returns one document's ranges in page order, as an empty slice
// when nothing was marked.
//
// Page order, and not insertion order: a client renders the ranges the way the
// book prints them, and the order the reader marked them in is not a fact about
// the book.
func (r *ReadingProgressRepository) ListByDocument(
	ctx context.Context,
	documentID string,
) ([]domain.ReadingProgress, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, document_id, from_page, to_page, state, created_at, updated_at
		 FROM reading_progress
		 WHERE document_id = ?
		 ORDER BY from_page, to_page`,
		documentID,
	)
	if err != nil {
		return nil, fmt.Errorf("query reading_progress by document: %w", err)
	}
	defer rows.Close()

	ranges := make([]domain.ReadingProgress, 0)
	for rows.Next() {
		progress, err := scanReadingProgress(rows)
		if err != nil {
			return nil, fmt.Errorf("scan reading_progress: %w", err)
		}
		ranges = append(ranges, *progress)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reading_progress: %w", err)
	}

	return ranges, nil
}

// rowScanner is what both *sql.Row and *sql.Rows offer.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanReadingProgress(row rowScanner) (*domain.ReadingProgress, error) {
	var progress domain.ReadingProgress
	var state, createdAt, updatedAt string

	if err := row.Scan(
		&progress.ID,
		&progress.DocumentID,
		&progress.FromPage,
		&progress.ToPage,
		&state,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}

	progress.State = domain.ReadingState(state)

	var err error
	if progress.CreatedAt, err = time.Parse(time.RFC3339, createdAt); err != nil {
		return nil, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if progress.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt); err != nil {
		return nil, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}

	return &progress, nil
}
