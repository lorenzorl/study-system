package sqlite_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// legacySchema is the schema as the retired code created it: identical to the
// baseline, except that resources still carries the dropped Dify column. It is
// what data/study.db looks like today.
const legacySchema = `
CREATE TABLE topics (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL
);
CREATE TABLE concepts (
	id TEXT PRIMARY KEY,
	topic_id TEXT NOT NULL REFERENCES topics(id),
	title TEXT NOT NULL,
	file_path TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL
);
CREATE TABLE flashcards (
	id TEXT PRIMARY KEY,
	concept_id TEXT NOT NULL REFERENCES concepts(id),
	question TEXT NOT NULL,
	answer TEXT NOT NULL,
	obsidian_id TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL
);
CREATE TABLE resources (
	id TEXT PRIMARY KEY,
	topic_id TEXT NOT NULL REFERENCES topics(id),
	title TEXT NOT NULL,
	type TEXT NOT NULL,
	source_uri TEXT UNIQUE NOT NULL,
	dify_document_id TEXT,
	created_at TEXT NOT NULL
);
CREATE TABLE card_states (
	id TEXT PRIMARY KEY,
	flashcard_id TEXT UNIQUE NOT NULL REFERENCES flashcards(id),
	stability REAL NOT NULL DEFAULT 0,
	difficulty REAL NOT NULL DEFAULT 0,
	next_review TEXT NOT NULL,
	last_review TEXT NOT NULL
);
CREATE TABLE review_logs (
	id TEXT PRIMARY KEY,
	flashcard_id TEXT NOT NULL REFERENCES flashcards(id),
	grade INTEGER NOT NULL,
	duration_ms INTEGER NOT NULL,
	created_at TEXT NOT NULL
);`

func openFile(t *testing.T, name string) *sql.DB {
	t.Helper()

	sqlDB, err := db.Open(t.Context(), "file:"+filepath.Join(t.TempDir(), name))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	return sqlDB
}

func hasColumn(t *testing.T, sqlDB *sql.DB, table, column string) bool {
	t.Helper()

	var name string
	err := sqlDB.QueryRow(
		`SELECT name FROM pragma_table_info(?) WHERE name = ?`, table, column,
	).Scan(&name)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false
	case err != nil:
		require.NoError(t, err)
	}
	return true
}

func TestMigrations_FreshDatabaseGetsTheStudySchema(t *testing.T) {
	sqlDB := openFile(t, "fresh.db")

	require.NoError(t, db.Migrate(t.Context(), sqlDB, sqlite.Migrations()))

	for _, table := range []string{
		"topics", "concepts", "flashcards", "resources", "card_states", "review_logs",
	} {
		assert.True(t, hasTable(t, sqlDB, table), "table %s must exist", table)
	}

	version, err := db.CurrentVersion(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Equal(t, 2, version)

	assert.False(t, hasColumn(t, sqlDB, "resources", "dify_document_id"),
		"a fresh database must never have the legacy column")
}

// This is the acceptance test for adopting the real development database.
func TestMigrations_AdoptLegacyDatabaseWithoutDataLoss(t *testing.T) {
	sqlDB := openFile(t, "legacy.db")

	_, err := sqlDB.Exec(legacySchema)
	require.NoError(t, err, "failed to build the pre-migration schema")

	_, err = sqlDB.Exec(`
		INSERT INTO topics (id, name, created_at) VALUES ('t1', 'DDD', '2026-06-24T16:25:41Z');
		INSERT INTO concepts (id, topic_id, title, file_path, created_at)
			VALUES ('c1', 't1', 'Aggregates', 'manual/DDD/Aggregates.md', '2026-06-24T16:25:41Z');
	`)
	require.NoError(t, err)
	require.True(t, hasColumn(t, sqlDB, "resources", "dify_document_id"),
		"precondition: the legacy schema must carry the column")

	require.NoError(t, db.Migrate(t.Context(), sqlDB, sqlite.Migrations()))

	var topicName, conceptTitle string
	require.NoError(t, sqlDB.QueryRow(`SELECT name FROM topics WHERE id = 't1'`).Scan(&topicName))
	require.NoError(t, sqlDB.QueryRow(`SELECT title FROM concepts WHERE id = 'c1'`).Scan(&conceptTitle))

	assert.Equal(t, "DDD", topicName, "existing topics must survive the migration")
	assert.Equal(t, "Aggregates", conceptTitle, "existing concepts must survive the migration")

	assert.False(t, hasColumn(t, sqlDB, "resources", "dify_document_id"),
		"the legacy column must be dropped")

	version, err := db.CurrentVersion(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Equal(t, 2, version)
}

func TestMigrations_AreIdempotentOnALegacyDatabase(t *testing.T) {
	sqlDB := openFile(t, "legacy-twice.db")

	_, err := sqlDB.Exec(legacySchema)
	require.NoError(t, err)

	require.NoError(t, db.Migrate(t.Context(), sqlDB, sqlite.Migrations()))
	require.NoError(t, db.Migrate(t.Context(), sqlDB, sqlite.Migrations()),
		"a second boot must not fail or duplicate anything")

	var applied int
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM schema_version`).Scan(&applied))
	assert.Equal(t, 2, applied)
}

// The baseline must be safe against a database that never had the legacy column
// either, so migration 2 becomes a no-op rather than an error.
func TestMigrations_DropLegacyColumnIsANoOpWithoutIt(t *testing.T) {
	sqlDB := openFile(t, "no-legacy.db")

	_, err := sqlDB.Exec(`
		CREATE TABLE resources (
			id TEXT PRIMARY KEY,
			topic_id TEXT NOT NULL REFERENCES topics(id),
			title TEXT NOT NULL,
			type TEXT NOT NULL,
			source_uri TEXT UNIQUE NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE topics (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, created_at TEXT NOT NULL);
	`)
	require.NoError(t, err)
	require.False(t, hasColumn(t, sqlDB, "resources", "dify_document_id"))

	require.NoError(t, db.Migrate(t.Context(), sqlDB, sqlite.Migrations()))

	assert.False(t, hasColumn(t, sqlDB, "resources", "dify_document_id"))
}

func TestMigrations_BaselineIsIdempotentAgainstAnAlreadyCompleteSchema(t *testing.T) {
	sqlDB := openFile(t, "complete.db")

	_, err := sqlDB.Exec(legacySchema)
	require.NoError(t, err)

	// Running the baseline directly must not fail on existing tables.
	require.NoError(t, db.Migrate(t.Context(), sqlDB, sqlite.Migrations()[:1]))

	version, err := db.CurrentVersion(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Equal(t, 1, version)
}

func hasTable(t *testing.T, sqlDB *sql.DB, table string) bool {
	t.Helper()

	var count int
	require.NoError(t, sqlDB.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name = ?`, table,
	).Scan(&count))

	return count == 1
}
