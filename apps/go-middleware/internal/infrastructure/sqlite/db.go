package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open creates a SQLite database connection and runs auto-migration.
// The dsn parameter should be a valid modernc.org/sqlite DSN
// (e.g., ":memory:" or "file:data.db").
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
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
		dify_document_id TEXT,
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

	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("execute migration: %w", err)
	}

	return nil
}
