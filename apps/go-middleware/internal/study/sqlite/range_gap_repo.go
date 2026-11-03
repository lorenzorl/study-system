package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// RangeGapRepository implements domain.RangeGapRepository using SQLite.
type RangeGapRepository struct {
	db *sql.DB
}

// NewRangeGapRepository creates a RangeGapRepository backed by the given database.
func NewRangeGapRepository(db *sql.DB) *RangeGapRepository {
	return &RangeGapRepository{db: db}
}

// ListByExplanationID returns the gaps of one explanation.
func (r *RangeGapRepository) ListByExplanationID(ctx context.Context, explanationID string) ([]domain.RangeGap, error) {
	return listGaps(ctx, r.db, explanationID)
}

// UpdateState writes one gap's state and returns the row as stored.
//
// The update is followed by a read inside the same transaction, because the state is
// the only column it may touch: returning the input would hide a write that also
// changed the evidence. A missing row is apperror.ErrNotFound, which is what makes a
// stale id in a browser a 404 instead of a silent success.
func (r *RangeGapRepository) UpdateState(
	ctx context.Context,
	gapID string,
	state domain.RangeGapState,
) (*domain.RangeGap, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx,
		`UPDATE range_gaps SET state = ? WHERE id = ?`,
		string(state), gapID,
	)
	if err != nil {
		return nil, fmt.Errorf("update range_gap state: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("count updated range_gaps: %w", err)
	}
	if affected == 0 {
		return nil, apperror.ErrNotFound
	}

	var gap domain.RangeGap
	var stored string
	err = tx.QueryRowContext(ctx,
		`SELECT id, explanation_id, point, page, quote, state FROM range_gaps WHERE id = ?`,
		gapID,
	).Scan(&gap.ID, &gap.ExplanationID, &gap.Point, &gap.Page, &gap.Quote, &stored)
	if err != nil {
		return nil, fmt.Errorf("read back range_gap: %w", err)
	}
	gap.State = domain.RangeGapState(stored)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &gap, nil
}

// listGaps reads the gaps of one explanation for both repositories, so the order
// and the mapping live in one place.
//
// ORDER BY rowid keeps the order the list was written in, which is the producer's
// ranking: the cap keeps the first MaxRangeGaps of it, so the order is part of what
// was decided, not an accident of storage.
func listGaps(ctx context.Context, db *sql.DB, explanationID string) ([]domain.RangeGap, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, explanation_id, point, page, quote, state
		 FROM range_gaps WHERE explanation_id = ?
		 ORDER BY rowid ASC`,
		explanationID,
	)
	if err != nil {
		return nil, fmt.Errorf("query range_gaps by explanation_id: %w", err)
	}
	defer rows.Close()

	// make with no length guarantees an empty explanation reads as an empty slice,
	// never as nil: a caller ranges over the gaps, and "nothing was left out" must
	// not be a case it has to special-case.
	gaps := make([]domain.RangeGap, 0, 4)
	for rows.Next() {
		var gap domain.RangeGap
		var state string
		if err := rows.Scan(&gap.ID, &gap.ExplanationID, &gap.Point, &gap.Page, &gap.Quote, &state); err != nil {
			return nil, fmt.Errorf("scan range_gap: %w", err)
		}
		gap.State = domain.RangeGapState(state)
		gaps = append(gaps, gap)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate range_gaps: %w", err)
	}

	return gaps, nil
}
