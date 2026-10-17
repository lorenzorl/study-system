package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// jobColumns is the select list every job read shares, so a column added here
// cannot be forgotten in one of them.
const jobColumns = `id, document_id, status, total_pages, started_at, updated_at, finished_at`

// JobRepository implements domain.JobRepository using SQLite.
type JobRepository struct {
	db *sql.DB
}

// NewJobRepository creates a JobRepository backed by the given database.
func NewJobRepository(db *sql.DB) *JobRepository {
	return &JobRepository{db: db}
}

// Create inserts a job. An unknown document is apperror.ErrNotFound.
func (r *JobRepository) Create(ctx context.Context, job *domain.IngestionJob) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO ingestion_jobs (id, document_id, status, total_pages, started_at, updated_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		job.ID,
		job.DocumentID,
		string(job.Status),
		job.TotalPages,
		formatInstant(job.StartedAt),
		formatInstant(job.UpdatedAt),
		nullableInstant(job.FinishedAt),
	)
	if err != nil {
		switch {
		case isUniqueConstraintError(err):
			return apperror.ErrConflict
		case isForeignKeyConstraintError(err):
			return apperror.ErrNotFound
		}
		return fmt.Errorf("insert job: %w", err)
	}
	return nil
}

// FindByID returns a job by its primary key, or apperror.ErrNotFound.
func (r *JobRepository) FindByID(ctx context.Context, id string) (*domain.IngestionJob, error) {
	return r.findOne(ctx, `SELECT `+jobColumns+` FROM ingestion_jobs WHERE id = ?`, id)
}

// FindLatestByDocumentID returns the document's most recent job, or
// apperror.ErrNotFound when the document has never been run.
//
// The sort is by started_at, then by rowid. The tiebreaker is not decoration:
// stored instants carry whole seconds, so two jobs created in the same second
// compare equal, and an ambiguous "latest" is a resume that silently continues
// the wrong run. rowid is insertion order, and SQLite assigns the next rowid as
// max(rowid)+1, so the largest surviving rowid is always the row inserted last —
// still true if the most recent job is deleted.
func (r *JobRepository) FindLatestByDocumentID(ctx context.Context, documentID string) (*domain.IngestionJob, error) {
	return r.findOne(ctx,
		`SELECT `+jobColumns+` FROM ingestion_jobs
		 WHERE document_id = ?
		 ORDER BY started_at DESC, rowid DESC
		 LIMIT 1`,
		documentID,
	)
}

// UpdateStatus persists a job's lifecycle fields after MarkRunning or Settle.
func (r *JobRepository) UpdateStatus(ctx context.Context, job *domain.IngestionJob) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE ingestion_jobs
		 SET status = ?, updated_at = ?, finished_at = ?
		 WHERE id = ?`,
		string(job.Status),
		formatInstant(job.UpdatedAt),
		nullableInstant(job.FinishedAt),
		job.ID,
	)
	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

// findOne runs a single-row job query and maps a miss to apperror.ErrNotFound.
func (r *JobRepository) findOne(ctx context.Context, query string, args ...any) (*domain.IngestionJob, error) {
	var (
		job        domain.IngestionJob
		status     string
		startedAt  string
		updatedAt  string
		finishedAt sql.NullString
	)

	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&job.ID,
		&job.DocumentID,
		&status,
		&job.TotalPages,
		&startedAt,
		&updatedAt,
		&finishedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("query job: %w", err)
	}

	job.Status = domain.JobStatus(status)

	if job.StartedAt, err = parseInstant(startedAt); err != nil {
		return nil, fmt.Errorf("job %s: %w", job.ID, err)
	}
	if job.UpdatedAt, err = parseInstant(updatedAt); err != nil {
		return nil, fmt.Errorf("job %s: %w", job.ID, err)
	}
	if job.FinishedAt, err = parseNullableInstant(finishedAt); err != nil {
		return nil, fmt.Errorf("job %s: %w", job.ID, err)
	}

	return &job, nil
}
