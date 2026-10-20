package composition

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
)

// The tests below are the regression net the two main() functions never had.
// They assert the assembly, not the behaviour: each context's own suite already
// proves what the use cases do, and these prove the objects the commands get are
// wired to the database they were given.

func testConfig(t *testing.T, dsn string) *config.Config {
	t.Helper()

	return &config.Config{
		Port:               config.DefaultPort,
		DatabaseDSN:        dsn,
		StorageRoot:        filepath.Join(t.TempDir(), "artifacts"),
		MaxUploadBytes:     config.DefaultMaxUploadBytes,
		CORSAllowedOrigins: config.DefaultCORSOrigins,
		LLMModel:           config.DefaultLLMModel,
		EmbeddingModel:     config.DefaultEmbeddingModel,
		OpenRouterBaseURL:  config.DefaultOpenRouterBaseURL,
	}
}

// fileDSN names a database file inside a temporary directory.
func fileDSN(t *testing.T) (string, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "study.db")
	return "file:" + path, path
}

func TestMigrations_CarryEveryContextInDisjointVersionBlocks(t *testing.T) {
	migrations := Migrations()
	require.NotEmpty(t, migrations)

	versions := make(map[int]bool, len(migrations))
	for _, migration := range migrations {
		assert.False(t, versions[migration.Version], "duplicate version %d", migration.Version)
		versions[migration.Version] = true
	}

	// The two blocks are the contract the rag context's migration file documents:
	// one shared schema_version table, so an overlap would be silent.
	inStudyBlock := 0
	inRAGBlock := 0
	for version := range versions {
		switch {
		case version >= 1 && version < 100:
			inStudyBlock++
		case version >= 100 && version < 200:
			inRAGBlock++
		default:
			t.Errorf("version %d belongs to no context's block", version)
		}
	}

	assert.NotZero(t, inStudyBlock)
	assert.NotZero(t, inRAGBlock)
}

func TestOpenDatabase_AppliesBothContextsMigrations(t *testing.T) {
	dsn, path := fileDSN(t)
	ctx := context.Background()

	sqlDB, err := OpenDatabase(ctx, testConfig(t, dsn))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	highest := 0
	for _, migration := range Migrations() {
		highest = max(highest, migration.Version)
	}

	version, err := db.CurrentVersion(ctx, sqlDB)
	require.NoError(t, err)
	assert.Equal(t, highest, version, "the recorded version must be the highest one applied")

	// One table from each context proves both blocks actually ran, which a
	// version number alone does not.
	for _, table := range []string{"topics", "documents"} {
		var count int
		require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count),
			"table %s is missing", table)
	}

	assert.FileExists(t, path)
}

func TestOpenReadOnlyDatabase_DoesNotCreateAMissingFile(t *testing.T) {
	dsn, path := fileDSN(t)
	ctx := context.Background()

	_, err := OpenReadOnlyDatabase(ctx, testConfig(t, dsn))
	require.Error(t, err)
	assert.NoFileExists(t, path, "a read-only open must never create the database")
}

func TestOpenReadOnlyDatabase_RejectsWrites(t *testing.T) {
	dsn, path := fileDSN(t)
	ctx := context.Background()

	writable, err := OpenDatabase(ctx, testConfig(t, dsn))
	require.NoError(t, err)
	require.NoError(t, writable.Close())

	sqlDB, err := OpenReadOnlyDatabase(ctx, testConfig(t, dsn))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO topics (id, name, created_at) VALUES ('id', 'name', '2026-01-01T00:00:00Z')`)
	require.Error(t, err, "a read-only handle must refuse a write")

	assert.FileExists(t, path)
}

func TestOpenPlanningDatabase_FallsBackToMemoryWithoutWritingToDisk(t *testing.T) {
	dsn, path := fileDSN(t)
	ctx := context.Background()

	sqlDB, err := OpenPlanningDatabase(ctx, testConfig(t, dsn))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	var count int
	require.NoError(t, sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM topics").Scan(&count),
		"the in-memory fallback must be migrated, not empty")

	assert.Equal(t, 0, count)
	assert.NoFileExists(t, path, "a dry run must not create the database")
}

func TestOpenPlanningDatabase_ReadsTheRealFileWithoutWriting(t *testing.T) {
	dsn, _ := fileDSN(t)
	ctx := context.Background()

	writable, err := OpenDatabase(ctx, testConfig(t, dsn))
	require.NoError(t, err)
	_, err = writable.ExecContext(ctx,
		`INSERT INTO topics (id, name, created_at) VALUES ('t1', 'Algorithms', '2026-01-01T00:00:00Z')`)
	require.NoError(t, err)
	require.NoError(t, writable.Close())

	sqlDB, err := OpenPlanningDatabase(ctx, testConfig(t, dsn))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	var name string
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT name FROM topics WHERE id = 't1'`).Scan(&name))
	assert.Equal(t, "Algorithms", name)

	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO topics (id, name, created_at) VALUES ('t2', 'Other', '2026-01-01T00:00:00Z')`)
	require.Error(t, err, "the planning handle is read-only when the file exists")
}

func TestNewClients_NoAPIKeyIsNotAnError(t *testing.T) {
	cfg := testConfig(t, "file:unused")
	cfg.OpenRouterAPIKey = ""

	clients, err := NewClients(cfg)
	require.NoError(t, err)

	assert.Nil(t, clients.Embedding)
	assert.Nil(t, clients.Model)
}

func TestNewClients_BuildsBothClientsFromOneCredential(t *testing.T) {
	cfg := testConfig(t, "file:unused")
	cfg.OpenRouterAPIKey = "test-key"

	clients, err := NewClients(cfg)
	require.NoError(t, err)

	assert.NotNil(t, clients.Embedding)
	assert.NotNil(t, clients.Model)
}

func TestNewClients_RejectsAMalformedBaseURL(t *testing.T) {
	cfg := testConfig(t, "file:unused")
	cfg.OpenRouterAPIKey = "test-key"
	cfg.OpenRouterBaseURL = "not-a-url"

	_, err := NewClients(cfg)
	require.Error(t, err)

	// config.Load already rejects a malformed root and names the variable, so a
	// real command never reaches this check. It is the second line of defence:
	// the client refuses to build an endpoint it cannot resolve.
	assert.Contains(t, err.Error(), "not-a-url")
}

func TestNewStudy_WiresEveryUseCaseToTheGivenDatabase(t *testing.T) {
	dsn, _ := fileDSN(t)
	ctx := context.Background()

	sqlDB, err := OpenDatabase(ctx, testConfig(t, dsn))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	study := NewStudy(sqlDB)

	topic, err := study.CreateTopic.Execute(ctx, "Algorithms")
	require.NoError(t, err)

	conceptID, err := study.SyncConcept.Execute(ctx, topic.Name, "Dijkstra", "algorithms/dijkstra.md")
	require.NoError(t, err)
	assert.NotEmpty(t, conceptID)

	tree, err := study.ListConcepts.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, tree, 1)
	require.Len(t, tree[0].Concepts, 1)
	assert.Equal(t, "Dijkstra", tree[0].Concepts[0].Title)

	// The review path is wired to the same database too, which is what a state
	// read after a write proves.
	due, err := study.GetDueCards.Execute(ctx)
	require.NoError(t, err)
	assert.Empty(t, due)
}

func TestNewRAG_ListDocumentsOnAnEmptyCorpusIsAnEmptySlice(t *testing.T) {
	dsn, _ := fileDSN(t)
	ctx := context.Background()

	sqlDB, err := OpenDatabase(ctx, testConfig(t, dsn))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	documents, err := NewRAG(sqlDB).ListDocuments().Execute(ctx)
	require.NoError(t, err)

	assert.NotNil(t, documents)
	assert.Empty(t, documents)
}

func TestNewRAG_SearchWithoutAClientStaysLexical(t *testing.T) {
	dsn, _ := fileDSN(t)
	ctx := context.Background()

	sqlDB, err := OpenDatabase(ctx, testConfig(t, dsn))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	// No client and no key: the lexical stage must answer from the database and
	// never reach for a model, which is what makes search work on a machine with
	// no credential.
	results, err := NewRAG(sqlDB).Search(Clients{}, config.DefaultEmbeddingModel).Search(ctx, "dijkstra", 10)
	require.NoError(t, err)

	assert.NotNil(t, results)
	assert.Empty(t, results)
}

func TestNewRAG_IndexReportsAnUnknownDocument(t *testing.T) {
	dsn, _ := fileDSN(t)
	ctx := context.Background()

	sqlDB, err := OpenDatabase(ctx, testConfig(t, dsn))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	_, err = NewRAG(sqlDB).Index().Execute(ctx, "missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperror.ErrNotFound), "got %v", err)
}

func TestNewRAG_StatusReportsAnUnknownDocument(t *testing.T) {
	dsn, _ := fileDSN(t)
	ctx := context.Background()

	sqlDB, err := OpenDatabase(ctx, testConfig(t, dsn))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	status := NewRAG(sqlDB).Status(nil, config.DefaultEmbeddingModel)

	_, err = status.Execute(ctx, "missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperror.ErrNotFound), "got %v", err)
}

func TestRAGEmbed_WithoutAClientIsAnError(t *testing.T) {
	dsn, _ := fileDSN(t)
	ctx := context.Background()

	sqlDB, err := OpenDatabase(ctx, testConfig(t, dsn))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	// There is no lexical fallback for writing a vector, so the absence of a
	// client is refused here rather than surfacing as a nil dereference mid-run.
	embedder, err := NewRAG(sqlDB).Embed(Clients{}, config.DefaultEmbeddingModel)
	require.Error(t, err)
	assert.Nil(t, embedder)
	assert.Contains(t, err.Error(), config.EnvOpenRouterAPIKey)
	assert.True(t, errors.Is(err, apperror.ErrInvalidInput), "got %v", err)
}
