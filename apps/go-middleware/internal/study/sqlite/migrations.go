package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
)

// Migrations returns the study context's schema migrations, in ascending order.
//
// Version 1 is a baseline and therefore idempotent: it must tolerate a database
// created before this migration system existed, which is exactly the situation
// of the development database in data/study.db.
func Migrations() []db.Migration {
	return []db.Migration{
		{
			Version:     1,
			Description: "baseline: topics, concepts, flashcards, resources, card_states, review_logs",
			Up:          migrateBaseline,
		},
		{
			Version:     2,
			Description: "drop legacy resources.dify_document_id column",
			Up:          dropLegacyDifyColumn,
		},
		{
			Version:     3,
			Description: "range_explanations and range_gaps: the reader's explanation of a page range and what it left out",
			Up:          migrateRangeExplanations,
		},
		{
			Version:     4,
			Description: "reading_progress: one row per page range, read or unread",
			Up:          migrateReadingProgress,
		},
		{
			Version:     5,
			Description: "study_plans and study_plan_items: the plan the agent produced and the ranges it recommends",
			Up:          migrateStudyPlan,
		},
	}
}

// migrateBaseline creates the study schema. Every statement uses IF NOT EXISTS so
// this is a no-op against a database that already has the tables.
func migrateBaseline(ctx context.Context, tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS topics (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS concepts (
		id TEXT PRIMARY KEY,
		topic_id TEXT NOT NULL REFERENCES topics(id),
		title TEXT NOT NULL,
		file_path TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS flashcards (
		id TEXT PRIMARY KEY,
		concept_id TEXT NOT NULL REFERENCES concepts(id),
		question TEXT NOT NULL,
		answer TEXT NOT NULL,
		obsidian_id TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS resources (
		id TEXT PRIMARY KEY,
		topic_id TEXT NOT NULL REFERENCES topics(id),
		title TEXT NOT NULL,
		type TEXT NOT NULL,
		source_uri TEXT UNIQUE NOT NULL,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS card_states (
		id TEXT PRIMARY KEY,
		flashcard_id TEXT UNIQUE NOT NULL REFERENCES flashcards(id),
		stability REAL NOT NULL DEFAULT 0,
		difficulty REAL NOT NULL DEFAULT 0,
		next_review TEXT NOT NULL,
		last_review TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS review_logs (
		id TEXT PRIMARY KEY,
		flashcard_id TEXT NOT NULL REFERENCES flashcards(id),
		grade INTEGER NOT NULL,
		duration_ms INTEGER NOT NULL,
		created_at TEXT NOT NULL
	);
	`

	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create study schema: %w", err)
	}
	return nil
}

// dropLegacyDifyColumn removes the column left behind by the retired Dify
// integration. Databases created before this migration still carry it, and the
// code no longer reads or writes it.
//
// The check makes the migration a no-op on freshly created databases, which
// never had the column.
func dropLegacyDifyColumn(ctx context.Context, tx *sql.Tx) error {
	const (
		table  = "resources"
		column = "dify_document_id"
	)

	exists, err := columnExists(ctx, tx, table, column)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	if _, err := tx.ExecContext(ctx, `ALTER TABLE resources DROP COLUMN dify_document_id`); err != nil {
		return fmt.Errorf("drop %s.%s: %w", table, column, err)
	}
	return nil
}

// migrateRangeExplanations adds the two tables behind POST /api/study/explanation.
//
// document_id is not a foreign key. The documents it names belong to the
// ingestion context: both contexts share one database file, but neither owns the
// other's schema, and a cross-context reference would couple a study migration to
// a rag table's shape.
//
// range_gaps.state is TEXT with no CHECK, following ingestion_jobs.status: the
// vocabulary (open, carded, and whatever the next slice adds) grows, and a stored
// row must not need a migration to gain a word.
func migrateRangeExplanations(ctx context.Context, tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS range_explanations (
		id TEXT PRIMARY KEY,
		document_id TEXT NOT NULL,
		from_page INTEGER NOT NULL,
		to_page INTEGER NOT NULL,
		text TEXT NOT NULL,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS range_gaps (
		id TEXT PRIMARY KEY,
		explanation_id TEXT NOT NULL REFERENCES range_explanations(id),
		point TEXT NOT NULL,
		page INTEGER NOT NULL,
		quote TEXT NOT NULL,
		state TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS range_gaps_explanation_id ON range_gaps(explanation_id);
	`

	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create range explanation schema: %w", err)
	}
	return nil
}

// migrateReadingProgress creates the coverage table: the other axis, the one that
// says where the reader is rather than what they can recall.
//
// The unit is the page range, not the page: a page counter produces numbers that
// mean nothing to a reader, while a range is the thing a plan recommends. The key
// is the range itself and not a document_sections id, because sections are derived
// data rebuilt on reprocess and a re-ingest would orphan the reader's work. There
// is no foreign key to documents for the same reason range_explanations has none:
// the two contexts share a database file, not a schema.
//
// state is TEXT with no CHECK, following ingestion_jobs.status, so the vocabulary
// can grow without a migration for every new word.
func migrateReadingProgress(ctx context.Context, tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS reading_progress (
		id TEXT PRIMARY KEY,
		document_id TEXT NOT NULL,
		from_page INTEGER NOT NULL CHECK (from_page >= 1),
		to_page INTEGER NOT NULL CHECK (to_page >= from_page),
		state TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE (document_id, from_page, to_page)
	);

	CREATE INDEX IF NOT EXISTS reading_progress_document_id
		ON reading_progress(document_id, from_page);
	`

	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create reading progress schema: %w", err)
	}

	return nil
}

// migrateStudyPlan creates the plan the agent produces and the app stores.
//
// Two tables and not one: a plan is a decision about a book, while an item is one
// recommended range with its own state, and the reader acts on items. `read` is not
// a column here — coverage lives in reading_progress and is derived for each item, so
// the same fact has one owner.
//
// superseded_at is a timestamp and not a deletion, so a plan the reader threw away
// keeps what it said (backlog, W2: a plan is partial and disposable).
func migrateStudyPlan(ctx context.Context, tx *sql.Tx) error {
	schema := `
	CREATE TABLE IF NOT EXISTS study_plans (
		id TEXT PRIMARY KEY,
		document_id TEXT NOT NULL,
		reading_mode TEXT NOT NULL,
		objective TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		superseded_at TEXT
	);

	CREATE INDEX IF NOT EXISTS study_plans_active
		ON study_plans(document_id, superseded_at);

	CREATE TABLE IF NOT EXISTS study_plan_items (
		id TEXT PRIMARY KEY,
		plan_id TEXT NOT NULL REFERENCES study_plans(id) ON DELETE CASCADE,
		position INTEGER NOT NULL CHECK (position >= 1),
		title TEXT NOT NULL,
		from_page INTEGER NOT NULL CHECK (from_page >= 1),
		to_page INTEGER NOT NULL CHECK (to_page >= from_page),
		note TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL,
		UNIQUE (plan_id, position)
	);
	`

	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create study plan schema: %w", err)
	}

	return nil
}

// columnExists reports whether table has a column of that name.
func columnExists(ctx context.Context, tx *sql.Tx, table, column string) (bool, error) {
	var name string
	err := tx.QueryRowContext(ctx,
		`SELECT name FROM pragma_table_info(?) WHERE name = ?`, table, column,
	).Scan(&name)

	switch {
	case err == sql.ErrNoRows:
		return false, nil
	case err != nil:
		return false, fmt.Errorf("inspect %s columns: %w", table, err)
	}
	return true, nil
}
