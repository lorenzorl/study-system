package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// ReviewLogRepository implements domain.ReviewLogRepository using SQLite.
type ReviewLogRepository struct {
	db *sql.DB
}

// NewReviewLogRepository creates a new ReviewLogRepository backed by the given database.
func NewReviewLogRepository(db *sql.DB) *ReviewLogRepository {
	return &ReviewLogRepository{db: db}
}

// Create inserts a new ReviewLog.
func (r *ReviewLogRepository) Create(ctx context.Context, log *domain.ReviewLog) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO review_logs (id, flashcard_id, grade, duration_ms, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		log.ID, log.FlashcardID, log.Grade, log.DurationMs,
		log.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("create review_log: %w", err)
	}
	return nil
}

// FindByFlashcardID returns all review logs for a given flashcard,
// ordered by creation time ascending.
func (r *ReviewLogRepository) FindByFlashcardID(ctx context.Context, flashcardID string) ([]domain.ReviewLog, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, flashcard_id, grade, duration_ms, created_at
		 FROM review_logs WHERE flashcard_id = ?
		 ORDER BY created_at ASC`,
		flashcardID,
	)
	if err != nil {
		return nil, fmt.Errorf("query review_logs by flashcard_id: %w", err)
	}
	defer rows.Close()

	var logs []domain.ReviewLog
	for rows.Next() {
		var rl domain.ReviewLog
		var createdAtStr string
		if err := rows.Scan(&rl.ID, &rl.FlashcardID, &rl.Grade, &rl.DurationMs, &createdAtStr); err != nil {
			return nil, fmt.Errorf("scan review_log: %w", err)
		}
		rl.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		logs = append(logs, rl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate review_logs: %w", err)
	}

	if logs == nil {
		logs = []domain.ReviewLog{}
	}
	return logs, nil
}
