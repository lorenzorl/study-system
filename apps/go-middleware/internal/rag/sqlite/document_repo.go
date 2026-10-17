package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// DocumentRepository implements domain.DocumentRepository using SQLite.
type DocumentRepository struct {
	db *sql.DB
}

// NewDocumentRepository creates a DocumentRepository backed by the given database.
func NewDocumentRepository(db *sql.DB) *DocumentRepository {
	return &DocumentRepository{db: db}
}

// Create inserts a document. An already-known content hash is apperror.ErrConflict.
func (r *DocumentRepository) Create(ctx context.Context, document *domain.Document) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO documents (id, title, artifact_id, source_uri, content_sha256, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		document.ID,
		document.Title,
		document.ArtifactID,
		document.SourceURI,
		document.ContentSHA256,
		formatInstant(document.CreatedAt),
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("insert document: %w", err)
	}
	return nil
}

// FindByID returns a document by its primary key, or apperror.ErrNotFound.
func (r *DocumentRepository) FindByID(ctx context.Context, id string) (*domain.Document, error) {
	return r.findOne(ctx,
		`SELECT id, title, artifact_id, source_uri, content_sha256, created_at
		 FROM documents WHERE id = ?`,
		id,
	)
}

// FindByContentHash returns the document holding those bytes, or apperror.ErrNotFound.
func (r *DocumentRepository) FindByContentHash(ctx context.Context, contentSHA256 string) (*domain.Document, error) {
	return r.findOne(ctx,
		`SELECT id, title, artifact_id, source_uri, content_sha256, created_at
		 FROM documents WHERE content_sha256 = ?`,
		contentSHA256,
	)
}

// ListIndexable returns every document with at least one recorded page, ordered
// by title and then id.
//
// The EXISTS is answered by the job_pages primary key, and it is deliberately not
// restricted to the document's latest job: the question is whether there is text
// to index at all. A document with no recorded page is left out so a reindex does
// not report work it cannot do.
func (r *DocumentRepository) ListIndexable(ctx context.Context) ([]*domain.Document, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, title, artifact_id, source_uri, content_sha256, created_at
		 FROM documents
		 WHERE EXISTS (
			SELECT 1 FROM ingestion_jobs
			JOIN job_pages ON job_pages.job_id = ingestion_jobs.id
			WHERE ingestion_jobs.document_id = documents.id
		 )
		 ORDER BY title ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list indexable documents: %w", err)
	}
	defer rows.Close()

	documents := []*domain.Document{}
	for rows.Next() {
		var (
			document  domain.Document
			createdAt string
		)

		if err := rows.Scan(
			&document.ID,
			&document.Title,
			&document.ArtifactID,
			&document.SourceURI,
			&document.ContentSHA256,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan indexable document: %w", err)
		}

		if document.CreatedAt, err = parseInstant(createdAt); err != nil {
			return nil, fmt.Errorf("document %s: %w", document.ID, err)
		}

		documents = append(documents, &document)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexable documents: %w", err)
	}

	return documents, nil
}

// findOne runs a single-row document query and maps a miss to apperror.ErrNotFound.
func (r *DocumentRepository) findOne(ctx context.Context, query string, arg any) (*domain.Document, error) {
	var (
		document  domain.Document
		createdAt string
	)

	err := r.db.QueryRowContext(ctx, query, arg).Scan(
		&document.ID,
		&document.Title,
		&document.ArtifactID,
		&document.SourceURI,
		&document.ContentSHA256,
		&createdAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("query document: %w", err)
	}

	document.CreatedAt, err = parseInstant(createdAt)
	if err != nil {
		return nil, fmt.Errorf("document %s: %w", document.ID, err)
	}

	return &document, nil
}
