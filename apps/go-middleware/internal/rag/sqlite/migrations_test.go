package sqlite_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
	studysqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ingestionTables is the schema this context is responsible for.
var ingestionTables = []string{"documents", "ingestion_jobs", "job_pages"}

// chunkIndexTables is the lexical index this context adds on top of it.
var chunkIndexTables = []string{"chunks", "chunks_fts"}

// chunkIndexVersion is the migration that adds the chunk index. It is written
// out rather than derived from the list because a test that renumbers a shipped
// migration must fail: databases in the wild have already run it.
const chunkIndexVersion = 101

// chunkEmbeddingVersion is the migration that adds the vector table. It is the
// latest version this context owns.
const chunkEmbeddingVersion = 102

// embeddingTables is the semantic index this context adds on top of the chunk
// index.
var embeddingTables = []string{"chunk_embeddings"}

// studyTables is the schema the other context owns, listed here so an
// adoption test can prove this context's migration leaves it alone.
var studyTables = []string{"topics", "concepts", "flashcards", "resources", "card_states", "review_logs"}

// combinedMigrations is what the composition root applies: both contexts' lists
// in one run. That is the only place the two version blocks meet, so it is what
// the collision guard has to exercise.
func combinedMigrations() []db.Migration {
	return slices.Concat(studysqlite.Migrations(), ragsqlite.Migrations())
}

func openFile(t *testing.T, name string) *sql.DB {
	t.Helper()

	sqlDB, err := db.Open(t.Context(), "file:"+filepath.Join(t.TempDir(), name))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	return sqlDB
}

// testHash returns a valid content hash, so fixtures cannot accidentally be
// rejected by the shape check they are not testing.
func testHash(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func hasTable(t *testing.T, sqlDB *sql.DB, table string) bool {
	t.Helper()

	var count int
	require.NoError(t, sqlDB.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name = ?`, table,
	).Scan(&count))

	return count == 1
}

func hasIndex(t *testing.T, sqlDB *sql.DB, index string) bool {
	t.Helper()

	var count int
	require.NoError(t, sqlDB.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='index' AND name = ?`, index,
	).Scan(&count))

	return count == 1
}

func schemaVersionRows(t *testing.T, sqlDB *sql.DB) int {
	t.Helper()

	var count int
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM schema_version`).Scan(&count))

	return count
}

// insertDocument writes a documents row directly, so a schema test does not
// depend on the adapter it is not testing.
func insertDocument(t *testing.T, sqlDB *sql.DB, id, contentSHA256 string) {
	t.Helper()

	_, err := sqlDB.Exec(
		`INSERT INTO documents (id, title, artifact_id, source_uri, content_sha256, created_at)
		 VALUES (?, 'a book', 'artifact-id', '/books/a.pdf', ?, '2026-10-17T12:00:00Z')`,
		id, contentSHA256)
	require.NoError(t, err)
}

func insertJob(t *testing.T, sqlDB *sql.DB, id, documentID string) {
	t.Helper()

	_, err := sqlDB.Exec(
		`INSERT INTO ingestion_jobs (id, document_id, status, total_pages, started_at, updated_at, finished_at)
		 VALUES (?, ?, 'pending', 10, '2026-10-17T12:00:00Z', '2026-10-17T12:00:00Z', NULL)`,
		id, documentID)
	require.NoError(t, err)
}

func TestMigrations_FreshDatabaseGetsTheIngestionSchema(t *testing.T) {
	sqlDB := openFile(t, "rag-only.db")

	require.NoError(t, db.Migrate(t.Context(), sqlDB, ragsqlite.Migrations()))

	for _, table := range ingestionTables {
		assert.True(t, hasTable(t, sqlDB, table), "table %s must exist", table)
	}
	assert.True(t, hasIndex(t, sqlDB, "idx_ingestion_jobs_document_id"),
		"the document lookup a resume performs must be indexed")

	version, err := db.CurrentVersion(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Equal(t, chunkEmbeddingVersion, version)
}

// The composition root applies both lists in one run. A shared ascending
// sequence would fail here with a duplicate version, or worse, skip silently —
// which is why this context owns a version block instead.
func TestMigrations_CombinedListsApplyWithoutCollision(t *testing.T) {
	sqlDB := openFile(t, "both.db")

	require.NoError(t, db.Migrate(t.Context(), sqlDB, combinedMigrations()))

	for _, table := range append(slices.Clone(studyTables), ingestionTables...) {
		assert.True(t, hasTable(t, sqlDB, table), "table %s must exist", table)
	}
	for _, table := range chunkIndexTables {
		assert.True(t, hasTable(t, sqlDB, table), "table %s must exist", table)
	}
	for _, table := range embeddingTables {
		assert.True(t, hasTable(t, sqlDB, table), "table %s must exist", table)
	}

	version, err := db.CurrentVersion(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Equal(t, chunkEmbeddingVersion, version)
	assert.Equal(t, 5, schemaVersionRows(t, sqlDB),
		"two study migrations plus three ingestion migrations must be recorded")
}

// Adoption: the study schema is already applied and holds data. This context's
// migration must add its tables without touching anything else.
func TestMigrations_AddsTheSchemaToAnExistingStudyDatabase(t *testing.T) {
	sqlDB := openFile(t, "existing-study.db")

	require.NoError(t, db.Migrate(t.Context(), sqlDB, studysqlite.Migrations()))

	_, err := sqlDB.Exec(
		`INSERT INTO topics (id, name, created_at) VALUES ('t1', 'DDD', '2026-06-24T16:25:41Z')`)
	require.NoError(t, err)
	require.False(t, hasTable(t, sqlDB, "documents"), "precondition: rag tables must not exist yet")

	require.NoError(t, db.Migrate(t.Context(), sqlDB, combinedMigrations()))

	var topicName string
	require.NoError(t, sqlDB.QueryRow(`SELECT name FROM topics WHERE id = 't1'`).Scan(&topicName))
	assert.Equal(t, "DDD", topicName, "existing study data must survive the ingestion migration")

	for _, table := range ingestionTables {
		assert.True(t, hasTable(t, sqlDB, table), "table %s must exist", table)
	}

	version, err := db.CurrentVersion(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Equal(t, chunkEmbeddingVersion, version)
}

func TestMigrations_AreIdempotent(t *testing.T) {
	sqlDB := openFile(t, "twice.db")

	require.NoError(t, db.Migrate(t.Context(), sqlDB, combinedMigrations()))
	require.NoError(t, db.Migrate(t.Context(), sqlDB, combinedMigrations()),
		"a second boot must not fail or duplicate anything")

	assert.Equal(t, 5, schemaVersionRows(t, sqlDB))
	for _, table := range chunkIndexTables {
		assert.True(t, hasTable(t, sqlDB, table), "table %s must survive a second migrate", table)
	}
	for _, table := range embeddingTables {
		assert.True(t, hasTable(t, sqlDB, table), "table %s must survive a second migrate", table)
	}
}

// The block is what makes a single-context Migrate call safe. If a version ever
// leaves the block, that property is gone, so it is asserted rather than assumed.
func TestMigrations_VersionsStayInsideTheRagBlock(t *testing.T) {
	migrations := ragsqlite.Migrations()
	require.NotEmpty(t, migrations)

	for _, migration := range migrations {
		assert.GreaterOrEqual(t, migration.Version, ragsqlite.VersionBlockStart,
			"migration %q is below this context's version block", migration.Description)
		assert.LessOrEqual(t, migration.Version, ragsqlite.VersionBlockEnd,
			"migration %q is above this context's version block", migration.Description)
	}
}

// The chunk index is added by its own migration, so a database that already ran
// the baseline adopts it in place.
func TestMigrations_FreshDatabaseGetsTheChunkIndex(t *testing.T) {
	sqlDB := openFile(t, "chunks.db")
	require.NoError(t, db.Migrate(t.Context(), sqlDB, ragsqlite.Migrations()))

	for _, table := range chunkIndexTables {
		assert.True(t, hasTable(t, sqlDB, table), "table %s must exist", table)
	}

	version, err := db.CurrentVersion(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Equal(t, chunkEmbeddingVersion, version,
		"a fresh migrate must reach the vector table, not stop at the chunk index")

	insertDocument(t, sqlDB, "doc-1", testHash("doc-1"))
	insertJob(t, sqlDB, "job-1", "doc-1")

	_, err = sqlDB.Exec(
		`INSERT INTO chunks (id, document_id, job_id, page_number, ordinal, text, created_at)
		 VALUES ('chunk-1', 'doc-1', 'job-1', 1, 0, 'el obstáculo del diseño', '2026-10-17T12:00:00Z')`)
	require.NoError(t, err)

	// chunks_fts holds its own copy: writing the chunk row alone does not index
	// it, which is the price of a copy instead of an external-content table.
	_, err = sqlDB.Exec(`INSERT INTO chunks_fts (chunk_id, text) VALUES ('chunk-1', 'el obstáculo del diseño')`)
	require.NoError(t, err)

	// The search itself is the proof that the virtual table is FTS5 with the
	// accent-folding tokenizer rather than a table that merely has the right name.
	var found int
	require.NoError(t, sqlDB.QueryRow(
		`SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH ?`, `"obstaculo"`,
	).Scan(&found))
	assert.Equal(t, 1, found, "an unaccented term must match the accented text")
}

// One outcome per page is the invariant a resumed run depends on. It is enforced
// by the primary key, not by the caller's discipline.
func TestMigrations_OneOutcomePerPageIsEnforcedByThePrimaryKey(t *testing.T) {
	sqlDB := openFile(t, "one-outcome.db")
	require.NoError(t, db.Migrate(t.Context(), sqlDB, ragsqlite.Migrations()))

	insertDocument(t, sqlDB, "doc-1", testHash("doc-1"))
	insertJob(t, sqlDB, "job-1", "doc-1")

	page := `INSERT INTO job_pages (job_id, page_number, status, method, error, text, updated_at)
	         VALUES ('job-1', 7, 'extracted', 'extract', '', 'text', '2026-10-17T12:00:00Z')`

	_, err := sqlDB.Exec(page)
	require.NoError(t, err)

	_, err = sqlDB.Exec(page)
	require.Error(t, err, "a second outcome for the same page must be rejected")

	var count int
	require.NoError(t, sqlDB.QueryRow(
		`SELECT count(*) FROM job_pages WHERE job_id = 'job-1' AND page_number = 7`).Scan(&count))
	assert.Equal(t, 1, count)

	_, err = sqlDB.Exec(
		`INSERT INTO job_pages (job_id, page_number, status, method, error, text, updated_at)
		 VALUES ('job-1', 0, 'extracted', 'extract', '', 'text', '2026-10-17T12:00:00Z')`)
	require.Error(t, err, "page numbering is 1-based and the CHECK must reject 0")
}

// The foreign keys must be live for the new tables too: they are applied per
// connection through the DSN, and a table created later is only covered if the
// PRAGMAs really are per connection.
func TestMigrations_ForeignKeysAreEnforcedForTheNewTables(t *testing.T) {
	sqlDB := openFile(t, "foreign-keys.db")
	require.NoError(t, db.Migrate(t.Context(), sqlDB, ragsqlite.Migrations()))

	_, err := sqlDB.Exec(
		`INSERT INTO ingestion_jobs (id, document_id, status, total_pages, started_at, updated_at, finished_at)
		 VALUES ('job-1', 'no-such-document', 'pending', 10, '2026-10-17T12:00:00Z', '2026-10-17T12:00:00Z', NULL)`)
	require.Error(t, err, "a job must not reference an unknown document")

	_, err = sqlDB.Exec(
		`INSERT INTO job_pages (job_id, page_number, status, method, error, text, updated_at)
		 VALUES ('no-such-job', 1, 'extracted', 'extract', '', 'text', '2026-10-17T12:00:00Z')`)
	require.Error(t, err, "a page outcome must not reference an unknown job")

	insertDocument(t, sqlDB, "doc-1", testHash("doc-1"))
	insertJob(t, sqlDB, "job-1", "doc-1")

	_, err = sqlDB.Exec(
		`INSERT INTO chunks (id, document_id, job_id, page_number, ordinal, text, created_at)
		 VALUES (?, 'no-such-document', 'job-1', 1, 0, 'text', '2026-10-17T12:00:00Z')`, uuid.NewString())
	require.Error(t, err, "a chunk must not reference an unknown document")

	_, err = sqlDB.Exec(
		`INSERT INTO chunks (id, document_id, job_id, page_number, ordinal, text, created_at)
		 VALUES (?, 'doc-1', 'no-such-job', 1, 0, 'text', '2026-10-17T12:00:00Z')`, uuid.NewString())
	require.Error(t, err, "a chunk must not reference an unknown job")
}

// A content hash is the document's identity, so two spellings of one digest must
// not be able to become two documents. The schema rejects the wrong shape.
func TestMigrations_ContentHashShapeIsEnforced(t *testing.T) {
	sqlDB := openFile(t, "hash-shape.db")
	require.NoError(t, db.Migrate(t.Context(), sqlDB, ragsqlite.Migrations()))

	valid := testHash("valid")
	insertDocument(t, sqlDB, "doc-valid", valid)

	for name, hash := range map[string]string{
		"uppercase hex":  strings.ToUpper(valid),
		"too short":      valid[:63],
		"not hex at all": strings.Repeat("z", 64),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := sqlDB.Exec(
				`INSERT INTO documents (id, title, artifact_id, source_uri, content_sha256, created_at)
				 VALUES (?, 'a book', 'artifact-id', '', ?, '2026-10-17T12:00:00Z')`,
				uuid.NewString(), hash)
			require.Error(t, err, "the content hash CHECK must reject %s", name)
		})
	}

	var count int
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM documents`).Scan(&count))
	assert.Equal(t, 1, count, "only the well-formed document may exist")
}
