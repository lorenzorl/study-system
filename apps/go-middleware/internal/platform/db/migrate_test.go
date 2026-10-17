package db_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
)

// markerMigration returns a migration that creates a table named after its
// version, so a test can observe whether it ran.
func markerMigration(version int) db.Migration {
	return db.Migration{
		Version:     version,
		Description: "create marker table",
		Up: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, "CREATE TABLE marker_"+itoa(version)+" (id TEXT PRIMARY KEY)")
			return err
		},
	}
}

func failingMigration(version int) db.Migration {
	return db.Migration{
		Version:     version,
		Description: "fails after a partial change",
		Up: func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "CREATE TABLE half_done (id TEXT PRIMARY KEY)"); err != nil {
				return err
			}
			return errors.New("boom")
		},
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func openMemory(t *testing.T) *sql.DB {
	t.Helper()

	sqlDB, err := db.Open(t.Context(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	return sqlDB
}

func tableExists(t *testing.T, sqlDB *sql.DB, table string) bool {
	t.Helper()

	var count int
	require.NoError(t, sqlDB.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
	).Scan(&count))

	return count == 1
}

func TestCurrentVersion_FreshDatabaseIsZero(t *testing.T) {
	version, err := db.CurrentVersion(t.Context(), openMemory(t))

	require.NoError(t, err)
	assert.Equal(t, 0, version)
}

func TestMigrate_AppliesEveryMigrationOnFreshDatabase(t *testing.T) {
	sqlDB := openMemory(t)

	err := db.Migrate(t.Context(), sqlDB, []db.Migration{
		markerMigration(1),
		markerMigration(2),
		markerMigration(3),
	})
	require.NoError(t, err)

	assert.True(t, tableExists(t, sqlDB, "marker_1"))
	assert.True(t, tableExists(t, sqlDB, "marker_2"))
	assert.True(t, tableExists(t, sqlDB, "marker_3"))
	assert.True(t, tableExists(t, sqlDB, "schema_version"))

	version, err := db.CurrentVersion(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Equal(t, 3, version)
}

func TestMigrate_IsIdempotent(t *testing.T) {
	sqlDB := openMemory(t)
	migrations := []db.Migration{markerMigration(1), markerMigration(2)}

	require.NoError(t, db.Migrate(t.Context(), sqlDB, migrations))
	require.NoError(t, db.Migrate(t.Context(), sqlDB, migrations), "second run must be a no-op")

	var applied int
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM schema_version`).Scan(&applied))

	assert.Equal(t, 2, applied, "each migration must be recorded exactly once")
}

func TestMigrate_AppliesInVersionOrderRegardlessOfInputOrder(t *testing.T) {
	sqlDB := openMemory(t)

	// Deliberately out of order.
	err := db.Migrate(t.Context(), sqlDB, []db.Migration{
		markerMigration(3),
		markerMigration(1),
		markerMigration(2),
	})
	require.NoError(t, err)

	rows, err := sqlDB.Query(`SELECT version FROM schema_version ORDER BY version`)
	require.NoError(t, err)
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var version int
		require.NoError(t, rows.Scan(&version))
		versions = append(versions, version)
	}

	assert.Equal(t, []int{1, 2, 3}, versions)
}

func TestMigrate_AppliesOnlyNewMigrations(t *testing.T) {
	sqlDB := openMemory(t)

	require.NoError(t, db.Migrate(t.Context(), sqlDB, []db.Migration{markerMigration(1)}))
	require.NoError(t, db.Migrate(t.Context(), sqlDB, []db.Migration{
		markerMigration(1), // already applied
		markerMigration(2), // new
	}))

	assert.True(t, tableExists(t, sqlDB, "marker_2"))

	version, err := db.CurrentVersion(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Equal(t, 2, version)
}

func TestMigrate_FailedMigrationLeavesNothingBehind(t *testing.T) {
	sqlDB := openMemory(t)

	err := db.Migrate(t.Context(), sqlDB, []db.Migration{
		markerMigration(1),
		failingMigration(2),
		markerMigration(3),
	})

	require.Error(t, err, "a failing migration must surface")
	assert.Contains(t, err.Error(), "boom")
	assert.Contains(t, err.Error(), "migration 2", "the error must name the migration")

	// The failed migration's DDL must be rolled back...
	assert.False(t, tableExists(t, sqlDB, "half_done"),
		"partial changes from a failed migration must not survive")

	// ...and it must not be recorded, nor may later migrations run.
	version, versionErr := db.CurrentVersion(t.Context(), sqlDB)
	require.NoError(t, versionErr)
	assert.Equal(t, 1, version, "the version must stay at the last success")
	assert.False(t, tableExists(t, sqlDB, "marker_3"), "later migrations must not run")
}

func TestMigrate_EmptyListStillCreatesVersionTable(t *testing.T) {
	sqlDB := openMemory(t)

	require.NoError(t, db.Migrate(t.Context(), sqlDB, nil))

	assert.True(t, tableExists(t, sqlDB, "schema_version"))
}

func TestMigrate_RejectsDuplicateVersions(t *testing.T) {
	err := db.Migrate(t.Context(), openMemory(t), []db.Migration{
		markerMigration(1),
		markerMigration(1),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate migration version 1")
}

func TestMigrate_RejectsNonPositiveVersions(t *testing.T) {
	for _, version := range []int{0, -1} {
		err := db.Migrate(t.Context(), openMemory(t), []db.Migration{markerMigration(version)})

		require.Error(t, err, "version %d must be rejected", version)
		assert.Contains(t, err.Error(), "must be positive")
	}
}

func TestMigrate_RejectsMigrationWithoutUpFunction(t *testing.T) {
	err := db.Migrate(t.Context(), openMemory(t), []db.Migration{
		{Version: 1, Description: "no up"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Up function")
}

func TestMigrate_ValidatesBeforeApplyingAnything(t *testing.T) {
	sqlDB := openMemory(t)

	// A valid migration followed by an invalid one: validation must reject the
	// whole set up front, so nothing is applied.
	err := db.Migrate(t.Context(), sqlDB, []db.Migration{
		markerMigration(1),
		markerMigration(1), // duplicate
	})

	require.Error(t, err)
	assert.False(t, tableExists(t, sqlDB, "marker_1"),
		"an invalid migration set must not be partially applied")
}

func TestOpen_MigrateOnFileDatabasePersistsAcrossReopen(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "persist.db")

	first, err := db.Open(t.Context(), dsn)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(t.Context(), first, []db.Migration{markerMigration(1)}))
	require.NoError(t, first.Close())

	second, err := db.Open(t.Context(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { second.Close() })

	version, err := db.CurrentVersion(t.Context(), second)
	require.NoError(t, err)

	assert.Equal(t, 1, version, "the applied version must survive a reopen")
	assert.True(t, tableExists(t, second, "marker_1"))
}
