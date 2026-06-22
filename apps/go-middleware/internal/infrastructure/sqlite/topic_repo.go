package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// TopicRepository implements domain.TopicRepository using SQLite.
type TopicRepository struct {
	db *sql.DB
}

// NewTopicRepository creates a new TopicRepository backed by the given database.
func NewTopicRepository(db *sql.DB) *TopicRepository {
	return &TopicRepository{db: db}
}

// UpsertByName creates a new topic if it does not exist, or returns the existing one.
func (r *TopicRepository) UpsertByName(ctx context.Context, name string) (*domain.Topic, error) {
	// Try INSERT first
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO topics (id, name, created_at) VALUES (?, ?, ?)`,
		id, name, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert topic: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("rows affected: %w", err)
	}

	if rowsAffected == 0 {
		// Topic already exists — fetch it
		return r.findByName(ctx, name)
	}

	return &domain.Topic{
		ID:        id,
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (r *TopicRepository) findByName(ctx context.Context, name string) (*domain.Topic, error) {
	var topic domain.Topic
	var createdAt string

	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, created_at FROM topics WHERE name = ?`, name,
	).Scan(&topic.ID, &topic.Name, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("query topic by name: %w", err)
	}

	topic.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &topic, nil
}

// Create inserts a new topic. Returns domain.ErrConflict on duplicate name.
func (r *TopicRepository) Create(ctx context.Context, topic *domain.Topic) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO topics (id, name, created_at) VALUES (?, ?, ?)",
		topic.ID, topic.Name, topic.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return domain.ErrConflict
		}
		return fmt.Errorf("create topic: %w", err)
	}
	return nil
}

// FindByID returns a topic by its primary key, or domain.ErrNotFound if missing.
func (r *TopicRepository) FindByID(ctx context.Context, id string) (*domain.Topic, error) {
	var topic domain.Topic
	var createdAt string

	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, created_at FROM topics WHERE id = ?`, id,
	).Scan(&topic.ID, &topic.Name, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("query topic by id: %w", err)
	}

	topic.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &topic, nil
}

// ListAll returns all topics ordered by creation time.
func (r *TopicRepository) ListAll(ctx context.Context) ([]domain.Topic, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, created_at FROM topics ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	defer rows.Close()

	var topics []domain.Topic
	for rows.Next() {
		var t domain.Topic
		var createdAt string
		if err := rows.Scan(&t.ID, &t.Name, &createdAt); err != nil {
			return nil, fmt.Errorf("scan topic: %w", err)
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		topics = append(topics, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topics: %w", err)
	}

	if topics == nil {
		topics = []domain.Topic{}
	}
	return topics, nil
}
