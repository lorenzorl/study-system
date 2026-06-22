package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// ConceptRepository implements domain.ConceptRepository using SQLite.
type ConceptRepository struct {
	db *sql.DB
}

// NewConceptRepository creates a new ConceptRepository backed by the given database.
func NewConceptRepository(db *sql.DB) *ConceptRepository {
	return &ConceptRepository{db: db}
}

// UpsertByPath creates or returns a concept by its unique FilePath.
func (r *ConceptRepository) UpsertByPath(ctx context.Context, topicID, title, filePath string) (*domain.Concept, error) {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO concepts (id, topic_id, title, file_path, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, topicID, title, filePath, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert concept: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return r.findByFilePath(ctx, filePath)
	}

	return &domain.Concept{
		ID:        id,
		TopicID:   topicID,
		Title:     title,
		FilePath:  filePath,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (r *ConceptRepository) findByFilePath(ctx context.Context, filePath string) (*domain.Concept, error) {
	var concept domain.Concept
	var createdAt string

	err := r.db.QueryRowContext(ctx,
		`SELECT id, topic_id, title, file_path, created_at FROM concepts WHERE file_path = ?`,
		filePath,
	).Scan(&concept.ID, &concept.TopicID, &concept.Title, &concept.FilePath, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("query concept by file path: %w", err)
	}

	concept.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &concept, nil
}

// Create inserts a new concept. Returns domain.ErrConflict on duplicate file_path.
func (r *ConceptRepository) Create(ctx context.Context, concept *domain.Concept) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO concepts (id, topic_id, title, file_path, created_at) VALUES (?, ?, ?, ?, ?)",
		concept.ID, concept.TopicID, concept.Title, concept.FilePath, concept.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return domain.ErrConflict
		}
		return fmt.Errorf("create concept: %w", err)
	}
	return nil
}

// ListByTopicID returns all concepts for a given topic, ordered by creation time.
func (r *ConceptRepository) ListByTopicID(ctx context.Context, topicID string) ([]domain.Concept, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, topic_id, title, file_path, created_at FROM concepts WHERE topic_id = ? ORDER BY created_at ASC`,
		topicID,
	)
	if err != nil {
		return nil, fmt.Errorf("list concepts: %w", err)
	}
	defer rows.Close()

	var concepts []domain.Concept
	for rows.Next() {
		var c domain.Concept
		var createdAt string
		if err := rows.Scan(&c.ID, &c.TopicID, &c.Title, &c.FilePath, &createdAt); err != nil {
			return nil, fmt.Errorf("scan concept: %w", err)
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		concepts = append(concepts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate concepts: %w", err)
	}

	if concepts == nil {
		concepts = []domain.Concept{}
	}
	return concepts, nil
}
