package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// RangeExplanationRepository implements domain.RangeExplanationRepository using
// SQLite.
type RangeExplanationRepository struct {
	db *sql.DB
}

// NewRangeExplanationRepository creates a RangeExplanationRepository backed by
// the given database.
func NewRangeExplanationRepository(db *sql.DB) *RangeExplanationRepository {
	return &RangeExplanationRepository{db: db}
}

// Create inserts the explanation and its gaps in one transaction.
//
// One transaction, and not an insert per table in sequence: the explanation row
// exists precisely to describe its gaps, so a half-written explanation would
// claim the reader explained a range that left nothing out. The same requirement
// is what lets the use case say "nothing is stored on a refusal": by the time
// this is called, the gaps are in hand, and either all of it lands or none of it
// does.
func (r *RangeExplanationRepository) Create(ctx context.Context, explanation *domain.RangeExplanation) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO range_explanations (id, document_id, from_page, to_page, text, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		explanation.ID,
		explanation.DocumentID,
		explanation.FromPage,
		explanation.ToPage,
		explanation.Text,
		explanation.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("create range_explanation: %w", err)
	}

	for _, gap := range explanation.Gaps {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO range_gaps (id, explanation_id, point, page, quote, state)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			gap.ID, explanation.ID, gap.Point, gap.Page, gap.Quote, string(gap.State),
		)
		if err != nil {
			return fmt.Errorf("create range_gap for explanation %q: %w", explanation.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// FindByID returns the explanation with its gaps, or apperror.ErrNotFound.
func (r *RangeExplanationRepository) FindByID(ctx context.Context, id string) (*domain.RangeExplanation, error) {
	var explanation domain.RangeExplanation
	var createdAt string

	err := r.db.QueryRowContext(ctx,
		`SELECT id, document_id, from_page, to_page, text, created_at
		 FROM range_explanations WHERE id = ?`,
		id,
	).Scan(
		&explanation.ID,
		&explanation.DocumentID,
		&explanation.FromPage,
		&explanation.ToPage,
		&explanation.Text,
		&createdAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("query range_explanation by id: %w", err)
	}

	explanation.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	gaps, err := listGaps(ctx, r.db, explanation.ID)
	if err != nil {
		return nil, err
	}
	explanation.Gaps = gaps

	return &explanation, nil
}
