package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// sectionColumns is the projection every read shares, so the two queries that
// scan a section cannot drift in column order.
const sectionColumns = `
SELECT id, document_id, COALESCE(parent_id, ''), level, title, start_page, end_page, source, created_at
FROM document_sections`

// SectionRepository implements domain.SectionRepository using SQLite.
type SectionRepository struct {
	db *sql.DB
}

// NewSectionRepository creates a SectionRepository backed by the given database.
func NewSectionRepository(db *sql.DB) *SectionRepository {
	return &SectionRepository{db: db}
}

// ReplaceDocumentSections deletes the document's sections and writes the given
// ones in their place, inside one transaction.
//
// Two ordering facts are handled here rather than asked of the caller:
//
//   - Parent before child. parent_id references this same table, so a child
//     written first is a foreign-key violation. Ordering by level puts every
//     parent first, because a parent is always at a lower level than its child.
//   - The document id is the parameter, not each section's own field. One call
//     writes one document; taking the identity from the call is what makes a
//     mismatched section impossible to store instead of merely discouraged.
//
// The clock is not a parameter: a section carries its own instant, stamped by
// domain.NewSection, which is also where its range and provenance were validated.
func (r *SectionRepository) ReplaceDocumentSections(ctx context.Context, documentID string, sections []*domain.Section) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin section replace of document %s: %w", documentID, err)
	}
	// Rollback after a successful commit is a no-op, so the deferred call is
	// unconditional and every early return is covered.
	defer func() { _ = tx.Rollback() }()

	// A document that does not exist is ErrNotFound on both paths. An empty
	// replace only deletes rows, and a zero-row DELETE cannot trip the foreign key
	// that an INSERT would, so the check cannot be left to the constraint.
	if err := documentExists(ctx, tx, documentID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM document_sections WHERE document_id = ?`, documentID,
	); err != nil {
		return fmt.Errorf("delete the sections of document %s: %w", documentID, err)
	}

	ordered := append([]*domain.Section(nil), sections...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Level != ordered[j].Level {
			return ordered[i].Level < ordered[j].Level
		}
		return ordered[i].StartPage < ordered[j].StartPage
	})

	insert, err := tx.PrepareContext(ctx,
		`INSERT INTO document_sections
		 (id, document_id, parent_id, level, title, start_page, end_page, source, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare section insert: %w", err)
	}
	defer insert.Close()

	for _, section := range ordered {
		// A top-level section has no parent, and an empty string is not a row id:
		// storing it would violate the foreign key instead of meaning "none".
		var parent any
		if section.ParentID != "" {
			parent = section.ParentID
		}

		if _, err := insert.ExecContext(ctx,
			section.ID, documentID, parent, section.Level, section.Title,
			section.StartPage, section.EndPage, string(section.Source),
			formatInstant(section.CreatedAt),
		); err != nil {
			if isForeignKeyConstraintError(err) {
				return apperror.ErrNotFound
			}
			if isUniqueConstraintError(err) {
				return apperror.ErrConflict
			}
			return fmt.Errorf("insert section %s of document %s: %w", section.ID, documentID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit section replace of document %s: %w", documentID, err)
	}
	return nil
}

// documentExists reports apperror.ErrNotFound when the document is unknown.
func documentExists(ctx context.Context, tx *sql.Tx, documentID string) error {
	var found int
	err := tx.QueryRowContext(ctx,
		`SELECT 1 FROM documents WHERE id = ?`, documentID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return apperror.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("look up document %s: %w", documentID, err)
	}
	return nil
}

// ListByDocument returns the document's sections in reading order.
//
// The order is the page a section starts on, then its level: a part and the
// chapter that opens on the same page read part first. The id is the last
// tiebreak so two sections on one page always come back in the same order.
func (r *SectionRepository) ListByDocument(ctx context.Context, documentID string) ([]*domain.Section, error) {
	sections := []*domain.Section{}

	rows, err := r.db.QueryContext(ctx,
		sectionColumns+`
WHERE document_id = ?
ORDER BY start_page ASC, level ASC, id ASC`,
		documentID)
	if err != nil {
		return nil, fmt.Errorf("list the sections of document %s: %w", documentID, err)
	}
	defer rows.Close()

	for rows.Next() {
		section, err := scanSection(rows)
		if err != nil {
			return nil, fmt.Errorf("scan a section of document %s: %w", documentID, err)
		}
		sections = append(sections, section)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate the sections of document %s: %w", documentID, err)
	}

	return sections, nil
}

// SectionAt returns the deepest section that owns the page.
//
// Deepest is the highest level, which is what makes the answer the most specific
// one available: a page inside a section is also inside its chapter and its
// part, and the citation a reader wants is the section. The start page is the
// tiebreak between two sections at the same level, and the id after that, so the
// answer is deterministic.
//
// A page no section owns is apperror.ErrNotFound and not an empty Section: "no
// structure here" has to be distinguishable from "the structure says nothing
// about this page", because only the second one is a bug.
func (r *SectionRepository) SectionAt(ctx context.Context, documentID string, page int) (*domain.Section, error) {
	if page < 1 {
		return nil, fmt.Errorf("section lookup for page %d, want at least 1: %w",
			page, apperror.ErrInvalidInput)
	}

	row := r.db.QueryRowContext(ctx,
		sectionColumns+`
WHERE document_id = ? AND start_page <= ? AND end_page >= ?
ORDER BY level DESC, start_page DESC, id DESC
LIMIT 1`,
		documentID, page, page)

	section, err := scanSection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find the section owning page %d of document %s: %w", page, documentID, err)
	}
	return section, nil
}

// scanner is the one method the two read paths share: *sql.Row and *sql.Rows
// both scan one section.
type scanner interface {
	Scan(dest ...any) error
}

// scanSection reads one row of sectionColumns.
func scanSection(row scanner) (*domain.Section, error) {
	var (
		section domain.Section
		source  string
		created string
	)

	if err := row.Scan(
		&section.ID,
		&section.DocumentID,
		&section.ParentID,
		&section.Level,
		&section.Title,
		&section.StartPage,
		&section.EndPage,
		&source,
		&created,
	); err != nil {
		return nil, err
	}

	instant, err := parseInstant(created)
	if err != nil {
		return nil, err
	}

	section.Source = domain.SectionSource(source)
	section.CreatedAt = instant

	return &section, nil
}
