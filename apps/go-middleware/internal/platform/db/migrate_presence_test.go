package db_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
)

// The ingestion block ran first; study's versions sit below its maximum and have
// never run. Presence is the only rule that applies them.
func TestMigrate_AppliesTheLowBlockAfterTheHighBlock(t *testing.T) {
	sqlDB := openMemory(t)
	high := []db.Migration{markerMigration(100), markerMigration(103)}

	require.NoError(t, db.Migrate(t.Context(), sqlDB, []db.Migration{
		markerMigration(1), markerMigration(2),
	}))
	require.NoError(t, db.Migrate(t.Context(), sqlDB, append([]db.Migration{
		markerMigration(1), markerMigration(2),
	}, high...)))
	require.NoError(t, db.Migrate(t.Context(), sqlDB, append([]db.Migration{
		markerMigration(1), markerMigration(2), markerMigration(3),
	}, high...)))

	for _, table := range []string{"marker_1", "marker_2", "marker_3", "marker_100", "marker_103"} {
		assert.True(t, tableExists(t, sqlDB, table), "%s must exist", table)
	}
}

// The presence rule must never apply less than "version above the recorded
// maximum" did: it is a strict superset, and this states it as a property instead
// of an example. The rule it replaced is written here on purpose, so the property
// fails if the runner ever becomes stricter than what it replaced.
func TestMigrate_PresenceIsNeverStricterThanTheMaximumRule(t *testing.T) {
	universe := []int{1, 2, 3, 100}
	recordedSets := [][]int{{}, {1}, {100}, {1, 100}, {1, 2, 3, 100}, {2}}

	for _, recorded := range recordedSets {
		for mask := 1; mask < 1<<len(universe); mask++ {
			var migrations []db.Migration
			var versions []int
			maximum := 0

			for index, version := range universe {
				if mask&(1<<index) != 0 {
					migrations = append(migrations, markerMigration(version))
					versions = append(versions, version)
				}
			}
			for _, version := range recorded {
				if version > maximum {
					maximum = version
				}
			}

			sqlDB := openMemory(t)
			for _, version := range recorded {
				require.NoError(t, db.Migrate(t.Context(), sqlDB, []db.Migration{markerMigration(version)}))
			}
			require.NoError(t, db.Migrate(t.Context(), sqlDB, migrations))

			for _, version := range versions {
				if version <= maximum {
					continue // the old rule skipped it, so there is nothing to preserve
				}
				assert.True(t, tableExists(t, sqlDB, "marker_"+itoa(version)),
					"recorded=%v list=%v: version %d was applied before the change and must stay applied",
					recorded, versions, version)
			}
		}
	}
}

// A rejected migration must roll back and leave the file byte-identical, which is
// the property the presence rule could have broken: a re-run against existing
// data would show up here as a changed file.
func TestMigrate_FailureLeavesTheFileByteIdentical(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.db")
	dsn := "file:" + path

	first, err := db.Open(t.Context(), dsn)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(t.Context(), first, []db.Migration{markerMigration(1)}))
	require.NoError(t, first.Close())

	digest := func() string {
		bytes, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		sum := sha256.Sum256(bytes)
		return hex.EncodeToString(sum[:])
	}
	before := digest()

	second, err := db.Open(t.Context(), dsn)
	require.NoError(t, err)
	failing := db.Migration{
		Version:     2,
		Description: "fails after a partial change",
		Up: func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `CREATE TABLE half_done (id TEXT PRIMARY KEY)`); err != nil {
				return err
			}
			return sql.ErrNoRows
		},
	}
	require.Error(t, db.Migrate(t.Context(), second, []db.Migration{
		markerMigration(1), failing, markerMigration(3),
	}))
	require.NoError(t, second.Close())

	assert.Equal(t, before, digest(), "a rejected migration must leave the database bytes untouched")
}

// A legacy database may carry a TEXT version column. The runner must read the
// recorded versions from it rather than rediscovering them as unapplied.
func TestMigrate_LegacyTextVersionColumnIsRead(t *testing.T) {
	sqlDB := openMemory(t)
	_, err := sqlDB.Exec(
		`CREATE TABLE schema_version (version TEXT PRIMARY KEY, description TEXT, applied_at TEXT)`)
	require.NoError(t, err)
	for _, version := range []string{"1", "2"} {
		_, err = sqlDB.Exec(
			`INSERT INTO schema_version VALUES (?, 'legacy', '2026-01-01T00:00:00Z')`, version)
		require.NoError(t, err)
	}

	require.NoError(t, db.Migrate(t.Context(), sqlDB, []db.Migration{
		markerMigration(1), markerMigration(2),
	}))

	assert.False(t, tableExists(t, sqlDB, "marker_1"), "a recorded version must not be rediscovered")
	assert.False(t, tableExists(t, sqlDB, "marker_2"), "a recorded version must not be rediscovered")
}
