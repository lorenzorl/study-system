package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
)

// Migration versions this context owns.
//
// platform/db keeps ONE schema_version table for the whole database, keyed by
// version alone, and its runner treats every migration at or below
// max(version) as already applied. Two contexts sharing one ascending sequence
// therefore break in a way that is easy to miss: a call that receives only one
// context's list does not fail, it silently skips the other context's earlier
// versions. The study context owns 1-2, so this context takes 100-199, which
// makes cross-context collisions impossible and makes a single-context call
// harmless.
//
// Never renumber an applied migration: databases in the wild have run it.
const (
	// VersionBlockStart is the first version this context may use.
	VersionBlockStart = 100

	// VersionBlockEnd is the last version this context may use.
	VersionBlockEnd = 199
)

// Migrations returns the ingestion context's schema migrations, ascending.
//
// Version 100 is a baseline: every statement uses IF NOT EXISTS, so it is
// idempotent and safe against a database that already carries the study schema.
func Migrations() []db.Migration {
	return []db.Migration{
		{
			Version:     VersionBlockStart,
			Description: "baseline: documents, ingestion_jobs, job_pages",
			Up:          migrateBaseline,
		},
	}
}

// migrateBaseline creates the ingestion schema.
//
// Two properties of this schema carry the resumability requirement:
//
//   - job_pages is keyed by (job_id, page_number), so a page has exactly one
//     outcome row. Retrying a page updates it; it can never be recorded twice and
//     therefore never counted twice.
//   - job_pages.text holds the produced text. The output document is built by
//     reading pages in order, which is what makes an interrupted run resumable
//     without re-extracting anything and impossible to emit twice.
//
// Constraints are written deliberately narrow: the CHECKs here are structural
// (page numbering and the identity hash shape) and cannot become wrong. The
// status and method vocabularies are NOT constrained, because they may grow and
// SQLite would require rebuilding the table to widen a CHECK; the domain's
// PageStatus and PageMethod are the place that validates them.
func migrateBaseline(ctx context.Context, tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS documents (
		id             TEXT PRIMARY KEY,
		title          TEXT NOT NULL,
		artifact_id    TEXT NOT NULL,
		source_uri     TEXT NOT NULL DEFAULT '',
		content_sha256 TEXT NOT NULL UNIQUE
			CHECK (length(content_sha256) = 64 AND NOT content_sha256 GLOB '*[^0-9a-f]*'),
		created_at     TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS ingestion_jobs (
		id          TEXT PRIMARY KEY,
		document_id TEXT NOT NULL REFERENCES documents(id),
		status      TEXT NOT NULL,
		total_pages INTEGER NOT NULL CHECK (total_pages >= 1),
		started_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL,
		finished_at TEXT
	);

	CREATE TABLE IF NOT EXISTS job_pages (
		job_id      TEXT NOT NULL REFERENCES ingestion_jobs(id) ON DELETE CASCADE,
		page_number INTEGER NOT NULL CHECK (page_number >= 1),
		status      TEXT NOT NULL,
		method      TEXT NOT NULL,
		error       TEXT NOT NULL DEFAULT '',
		text        TEXT NOT NULL DEFAULT '',
		updated_at  TEXT NOT NULL,
		PRIMARY KEY (job_id, page_number)
	);

	CREATE INDEX IF NOT EXISTS idx_ingestion_jobs_document_id
		ON ingestion_jobs (document_id, started_at DESC);
	`

	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create ingestion schema: %w", err)
	}
	return nil
}
