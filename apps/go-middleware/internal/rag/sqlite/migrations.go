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
		{
			Version:     101,
			Description: "chunks and chunks_fts: the lexical index over recorded page text",
			Up:          migrateChunks,
		},
		{
			Version:     102,
			Description: "chunk_embeddings: stored vectors with the model and dimension that produced them",
			Up:          migrateChunkEmbeddings,
		},
		{
			Version:     103,
			Description: "document_sections: the parts, chapters and sections that own page ranges",
			Up:          migrateSections,
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

// migrateChunks creates the lexical index: one row per chunk, and one row per
// chunk in an FTS5 table that holds a second copy of the text.
//
// chunks is the source of truth. It carries the citation — document, job, page
// and ordinal — and it is what a hit joins back to, so a search result can name
// the book and the page a reader should open. A chunk belongs to exactly one
// page, which is what keeps that citation exact.
//
// chunks_fts is a plain FTS5 table rather than an external-content table over
// chunks. The copy costs about 1,3 MB for a 359-page book and buys a delete path
// with no triggers and no 'delete'-row ceremony: an external-content table is
// only told a row went away by inserting a special 'delete' row carrying the row's
// old values first, and getting that subtly wrong leaves terms matching text that
// no longer exists without raising anything. Indexing is replace-all-per-document,
// so the two tables are always written together inside one transaction and can
// never disagree outside it.
//
// The tokenizer is unicode61 with remove_diacritics 2, which folds accents in
// both directions: "obstaculo" finds "obstáculo" and the reverse (S5). That is
// what lets a Spanish query, always written by the user in Spanish (S1), find
// Spanish text however the accents were typed. chunk_id is UNINDEXED: it travels
// with a hit and is never searched.
func migrateChunks(ctx context.Context, tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS chunks (
		id          TEXT PRIMARY KEY,
		document_id TEXT NOT NULL REFERENCES documents(id),
		job_id      TEXT NOT NULL REFERENCES ingestion_jobs(id) ON DELETE CASCADE,
		page_number INTEGER NOT NULL CHECK (page_number >= 1),
		ordinal     INTEGER NOT NULL CHECK (ordinal >= 0),
		text        TEXT NOT NULL,
		created_at  TEXT NOT NULL
	);

	CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
		chunk_id UNINDEXED,
		text,
		tokenize = 'unicode61 remove_diacritics 2'
	);
	`

	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create chunk index: %w", err)
	}
	return nil
}

// migrateChunkEmbeddings creates the vector table: one row per chunk, carrying
// the model and the dimension that produced it.
//
// chunks stays the source of truth. A vector is derived data with the same
// rebuildable life cycle as the lexical index, and the foreign key cascades, so a
// reindex that replaces a document's chunks can never leave a vector pointing at
// a chunk that no longer exists.
//
// The row carries model and dimensions on purpose. Two encodings are ambiguous
// without them: a vector from a different model, and a vector whose blob length
// does not match its own dimension. The first is filtered out by model, and the
// second is rejected when it is read, so neither is silently compared against the
// current query vector.
func migrateChunkEmbeddings(ctx context.Context, tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS chunk_embeddings (
		chunk_id   TEXT PRIMARY KEY REFERENCES chunks(id) ON DELETE CASCADE,
		model      TEXT NOT NULL,
		dimensions INTEGER NOT NULL CHECK (dimensions > 0),
		vector     BLOB NOT NULL,
		created_at TEXT NOT NULL
	);
	`

	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create chunk embeddings: %w", err)
	}
	return nil
}

// migrateSections creates the document structure: one row per part, chapter or
// section, carrying the page range it owns and the source it came from.
//
// Two properties of the shape are deliberate:
//
//   - The table is derived data with the same rebuildable life cycle as the
//     chunk index. A run replaces a document's sections rather than editing
//     them, so the extractor can change without a data migration.
//   - end_page is stored and not derived from the next row's start_page. The
//     question the index asks is "which section owns page N", and a stored range
//     answers it with one BETWEEN on one index instead of a window over the
//     document's rows.
//
// level and source carry no CHECK, for the reason the ingestion vocabulary does
// not either: they may grow, and SQLite would require rebuilding the table to
// widen one. domain.Section is what validates them.
func migrateSections(ctx context.Context, tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS document_sections (
		id          TEXT PRIMARY KEY,
		document_id TEXT NOT NULL REFERENCES documents(id),
		parent_id   TEXT REFERENCES document_sections(id),
		level       INTEGER NOT NULL CHECK (level >= 1),
		title       TEXT NOT NULL,
		start_page  INTEGER NOT NULL CHECK (start_page >= 1),
		end_page    INTEGER NOT NULL CHECK (end_page >= start_page),
		source      TEXT NOT NULL,
		created_at  TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_document_sections_document_id
		ON document_sections (document_id, start_page);
	`

	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create section schema: %w", err)
	}
	return nil
}
