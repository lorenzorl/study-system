package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
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
		`SELECT id, topic_id, title, type, source_uri, COALESCE(dify_document_id, ''), created_at
		 FROM resources WHERE source_uri = ?`,
		sourceURI,
	).Scan(&res.ID, &res.TopicID, &res.Title, &res.Type, &res.SourceURI, &res.DifyDocumentID, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query resource by source_uri: %w", err)
	}

	res.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &res, nil
}

// Create inserts a new resource. Returns domain.ErrConflict on duplicate SourceURI.
func (r *ResourceRepository) Create(ctx context.Context, resource *domain.Resource) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO resources (id, topic_id, title, type, source_uri, dify_document_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		resource.ID, resource.TopicID, resource.Title, resource.Type,
		resource.SourceURI, resource.DifyDocumentID, resource.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return domain.ErrConflict
		}
		return fmt.Errorf("create resource: %w", err)
	}
	return nil
}

// FindAllDifyDocumentIDs returns all distinct non-empty DifyDocumentIDs from the resources table.
// Returns an empty slice when no rows match.
func (r *ResourceRepository) FindAllDifyDocumentIDs(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT dify_document_id FROM resources WHERE dify_document_id IS NOT NULL AND dify_document_id != ''`,
	)
	if err != nil {
		return nil, fmt.Errorf("query distinct dify_document_id: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan dify_document_id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dify_document_id rows: %w", err)
	}

	return ids, nil
}

// UpdateDifyDocumentID updates the DifyDocumentID field for an existing resource.
func (r *ResourceRepository) UpdateDifyDocumentID(ctx context.Context, id, difyDocID string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE resources SET dify_document_id = ? WHERE id = ?`,
		difyDocID, id,
	)
	if err != nil {
		return fmt.Errorf("update dify_document_id: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
