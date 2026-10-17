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
