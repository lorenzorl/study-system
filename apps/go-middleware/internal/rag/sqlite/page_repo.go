package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// PageOutcomeRepository implements domain.PageOutcomeRepository using SQLite.
type PageOutcomeRepository struct {
	db *sql.DB
}

// NewPageOutcomeRepository creates a PageOutcomeRepository backed by the given database.
func NewPageOutcomeRepository(db *sql.DB) *PageOutcomeRepository {
	return &PageOutcomeRepository{db: db}
}

// SaveOutcome records a page's outcome, replacing any earlier outcome for the
// same page. An unknown job is apperror.ErrNotFound.
//
// The upsert is the resume path: a page that failed on an earlier run is
// attempted again, and its row is updated in place. Appending instead would leave
// two outcomes for one page, and then ProgressOf's vote-counting would be the
// only thing standing between that and a page counted twice.
func (r *PageOutcomeRepository) SaveOutcome(ctx context.Context, outcome domain.PageOutcome) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO job_pages (job_id, page_number, status, method, error, text, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (job_id, page_number) DO UPDATE SET
			status     = excluded.status,
			method     = excluded.method,
			error      = excluded.error,
			text       = excluded.text,
			updated_at = excluded.updated_at`,
		outcome.JobID,
		outcome.PageNumber,
		string(outcome.Status),
		string(outcome.Method),
		outcome.Error,
		outcome.Text,
		formatInstant(outcome.UpdatedAt),
	)
	if err != nil {
		if isForeignKeyConstraintError(err) {
			return apperror.ErrNotFound
		}
		return fmt.Errorf("save page outcome %d: %w", outcome.PageNumber, err)
	}
	return nil
}

// ListOutcomes returns every recorded outcome of a job, by page number ascending.
func (r *PageOutcomeRepository) ListOutcomes(ctx context.Context, jobID string) ([]domain.PageOutcome, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT job_id, page_number, status, method, error, text, updated_at
		 FROM job_pages WHERE job_id = ? ORDER BY page_number ASC`,
		jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("list page outcomes: %w", err)
	}
	defer rows.Close()

	outcomes := []domain.PageOutcome{}
	for rows.Next() {
		var (
			outcome   domain.PageOutcome
			status    string
			method    string
			updatedAt string
		)

		if err := rows.Scan(
			&outcome.JobID,
			&outcome.PageNumber,
			&status,
			&method,
			&outcome.Error,
			&outcome.Text,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan page outcome: %w", err)
		}

		outcome.Status = domain.PageStatus(status)
		outcome.Method = domain.PageMethod(method)

		if outcome.UpdatedAt, err = parseInstant(updatedAt); err != nil {
			return nil, fmt.Errorf("page %d of job %s: %w", outcome.PageNumber, jobID, err)
		}

		outcomes = append(outcomes, outcome)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate page outcomes: %w", err)
	}

	return outcomes, nil
}
