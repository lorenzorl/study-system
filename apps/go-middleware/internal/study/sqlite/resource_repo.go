package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// ResourceRepository implements domain.ResourceRepository using SQLite.
type ResourceRepository struct {
	db *sql.DB
}

// NewResourceRepository creates a new ResourceRepository backed by the given database.
func NewResourceRepository(db *sql.DB) *ResourceRepository {
	return &ResourceRepository{db: db}
}

// FindBySourceURI returns a resource by its unique SourceURI.
// Returns nil, nil when no resource is found.
func (r *ResourceRepository) FindBySourceURI(ctx context.Context, sourceURI string) (*domain.Resource, error) {
	var res domain.Resource
	var createdAt string

	err := r.db.QueryRowContext(ctx,
		`SELECT id, topic_id, title, type, source_uri, created_at
		 FROM resources WHERE source_uri = ?`,
		sourceURI,
	).Scan(&res.ID, &res.TopicID, &res.Title, &res.Type, &res.SourceURI, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query resource by source_uri: %w", err)
	}

	res.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &res, nil
}

// Create inserts a new resource. Returns apperror.ErrConflict on duplicate SourceURI.
func (r *ResourceRepository) Create(ctx context.Context, resource *domain.Resource) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO resources (id, topic_id, title, type, source_uri, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		resource.ID, resource.TopicID, resource.Title, resource.Type,
		resource.SourceURI, resource.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("create resource: %w", err)
	}
	return nil
}
