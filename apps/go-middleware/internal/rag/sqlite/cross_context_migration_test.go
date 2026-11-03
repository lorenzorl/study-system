package sqlite_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
	studysqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// The ingestion block reaches 103 before study's block exists at all, so study's
// versions sit below the recorded maximum and must still be applied. This is the
// cross-context case the presence rule exists for, run against the real lists.
func TestMigrations_StudyBlockAppliesAfterTheIngestionBlock(t *testing.T) {
	sqlDB := openFile(t, "ingestion-first.db")

	require.NoError(t, db.Migrate(t.Context(), sqlDB, ragsqlite.Migrations()))
	require.False(t, hasTable(t, sqlDB, "range_explanations"),
		"precondition: study v3 has not run")

	require.NoError(t, db.Migrate(t.Context(), sqlDB, combinedMigrations()))

	for _, table := range []string{"range_explanations", "range_gaps"} {
		assert.True(t, hasTable(t, sqlDB, table), "study v3 must run below the recorded maximum")
	}
}

// The recorded row count is derived from the lists, so a count assertion cannot
// drift away from what the lists actually contain.
func TestMigrations_RecordedRowsMatchTheLists(t *testing.T) {
	sqlDB := openFile(t, "row-count.db")
	require.NoError(t, db.Migrate(t.Context(), sqlDB, combinedMigrations()))

	want := len(studysqlite.Migrations()) + len(ragsqlite.Migrations())
	assert.Equal(t, want, schemaVersionRows(t, sqlDB),
		"every migration in both lists must be recorded exactly once")

	known := make(map[int]bool)
	for _, migration := range slices.Concat(studysqlite.Migrations(), ragsqlite.Migrations()) {
		known[migration.Version] = true
	}

	rows, err := sqlDB.Query(`SELECT version FROM schema_version`)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var version int
		require.NoError(t, rows.Scan(&version))
		assert.True(t, known[version], "version %d is recorded but no list contains it", version)
	}
	require.NoError(t, rows.Err())
}
