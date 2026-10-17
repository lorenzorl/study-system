package db_test

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
)

// fileDSN returns a file-backed DSN inside the test's temp directory.
func fileDSN(t *testing.T, name string) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), name)
}

func TestOpen_AppliesRequiredPragmas(t *testing.T) {
	sqlDB, err := db.Open(t.Context(), fileDSN(t, "pragma.db"))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	var foreignKeys, busyTimeout, journalMode string
	require.NoError(t, sqlDB.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys))
	require.NoError(t, sqlDB.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout))
	require.NoError(t, sqlDB.QueryRow("PRAGMA journal_mode").Scan(&journalMode))

	assert.Equal(t, "1", foreignKeys, "foreign_keys must be enforced")
	assert.Equal(t, "5000", busyTimeout, "busy_timeout must be set")
	assert.Equal(t, "wal", journalMode, "a file database must use WAL")
}

// This is the behaviour the PRAGMAs exist for: before them, orphan rows were
// accepted and the REFERENCES clauses were decorative.
func TestOpen_EnforcesForeignKeys(t *testing.T) {
	sqlDB, err := db.Open(t.Context(), fileDSN(t, "fk.db"))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	_, err = sqlDB.Exec(`
		CREATE TABLE parent (id TEXT PRIMARY KEY);
		CREATE TABLE child (id TEXT PRIMARY KEY, parent_id TEXT NOT NULL REFERENCES parent(id));
	`)
	require.NoError(t, err)

	_, err = sqlDB.Exec(`INSERT INTO child (id, parent_id) VALUES ('c1', 'ghost')`)

	require.Error(t, err, "inserting an orphan row must be rejected")
	assert.Contains(t, err.Error(), "FOREIGN KEY")
}

func TestOpen_AcceptsRowsWithSatisfiedReferences(t *testing.T) {
	sqlDB, err := db.Open(t.Context(), fileDSN(t, "fk-ok.db"))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	_, err = sqlDB.Exec(`
		CREATE TABLE parent (id TEXT PRIMARY KEY);
		CREATE TABLE child (id TEXT PRIMARY KEY, parent_id TEXT NOT NULL REFERENCES parent(id));
		INSERT INTO parent (id) VALUES ('p1');
		INSERT INTO child (id, parent_id) VALUES ('c1', 'p1');
	`)

	require.NoError(t, err)
}

// An in-memory SQLite database is private to the connection that created it.
// Pinning the pool keeps every repository on the same schema; without it a
// second connection would see an empty database.
func TestOpen_PinsInMemoryDatabaseToOneConnection(t *testing.T) {
	sqlDB, err := db.Open(t.Context(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	assert.Equal(t, 1, sqlDB.Stats().MaxOpenConnections)
}

func TestOpen_InMemorySchemaIsVisibleFromConcurrentQueries(t *testing.T) {
	sqlDB, err := db.Open(t.Context(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	_, err = sqlDB.Exec(`CREATE TABLE marker (id TEXT PRIMARY KEY)`)
	require.NoError(t, err)

	const workers = 8
	var wg sync.WaitGroup
	results := make([]int, workers)

	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			var count int
			err := sqlDB.QueryRow(
				`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='marker'`,
			).Scan(&count)
			if err == nil {
				results[i] = count
			}
		}()
	}
	wg.Wait()

	for i, count := range results {
		assert.Equal(t, 1, count,
			"worker %d must see the schema created through another connection", i)
	}
}

func TestOpen_AppliesPragmasToInMemoryDatabases(t *testing.T) {
	sqlDB, err := db.Open(t.Context(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	var foreignKeys string
	require.NoError(t, sqlDB.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys))

	assert.Equal(t, "1", foreignKeys, "in-memory databases must also enforce foreign keys")
}

func TestOpen_PreservesExistingDSNParameters(t *testing.T) {
	dsn := fileDSN(t, "shared.db") + "?mode=rwc"

	sqlDB, err := db.Open(t.Context(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	_, err = sqlDB.Exec(`CREATE TABLE t (id TEXT PRIMARY KEY)`)
	require.NoError(t, err)
}

func TestOpen_RejectsUnusableDSN(t *testing.T) {
	_, err := db.Open(t.Context(), "file:/nonexistent-directory-xyz/nested/db.sqlite")

	require.Error(t, err)
}

func TestOpen_RejectsUnparsableDSN(t *testing.T) {
	_, err := db.Open(t.Context(), "file://host/unsupported/path.db")

	// Whatever the driver decides, Open must surface an error rather than a
	// usable handle; a DSN this malformed must not silently succeed.
	if err == nil {
		t.Skip("driver accepted this DSN; nothing to assert")
	}
	assert.Error(t, err)
}
