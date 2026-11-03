package sqlite

import (
	"context"
	"database/sql"
	"fmt"

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
