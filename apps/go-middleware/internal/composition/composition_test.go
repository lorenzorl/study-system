package composition

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	studydomain "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
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

func TestNewEmbeddingClients_BuildsOnlyTheEmbeddingClient(t *testing.T) {
	cfg := testConfig(t, "file:unused")
	cfg.OpenRouterAPIKey = "test-key"

	// -search and -embed never transcribe, so they must not build — or be able to
	// fail on — a chat client they never use.
	clients, err := NewEmbeddingClients(cfg)
	require.NoError(t, err)

	assert.NotNil(t, clients.Embedding)
	assert.Nil(t, clients.Model)
}

func TestNewTranscriptionClients_BuildsOnlyTheChatClient(t *testing.T) {
	cfg := testConfig(t, "file:unused")
	cfg.OpenRouterAPIKey = "test-key"

	// The conversion run leaves the index and the vectors to later steps.
	clients, err := NewTranscriptionClients(cfg)
	require.NoError(t, err)

	assert.Nil(t, clients.Embedding)
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
	require.NotEmpty(t, conceptID)

	// CreateConcept is the manual path; it must work beside the synced one.
	manual, err := study.CreateConcept.Execute(ctx, topic.ID, "BFS")
	require.NoError(t, err)
	assert.Equal(t, topic.ID, manual.TopicID)

	resourceID, err := study.SyncResource.Execute(ctx, topic.Name, "CLRS", "book", "isbn:9780262046305")
	require.NoError(t, err)
	assert.NotEmpty(t, resourceID)

	processed, err := study.SyncFlashcards.Execute(ctx, conceptID, []studydomain.Flashcard{
		{ObsidianID: "card-1", Question: "What does Dijkstra compute?", Answer: "Shortest paths"},
		{ObsidianID: "card-2", Question: "Which structure does it use?", Answer: "A priority queue"},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, processed)

	// Syncing creates an initial card state whose next review is now, so both
	// cards are due: this read is what proves the write reached the same database.
	due, err := study.GetDueCards.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, due, 2)

	reviewed, err := study.SubmitReview.Execute(ctx, due[0].FlashcardID, 3, 4200)
	require.NoError(t, err)
	assert.Equal(t, due[0].FlashcardID, reviewed.FlashcardID)

	// A reviewed card is scheduled days ahead, so the due list shrinks to one.
	due, err = study.GetDueCards.Execute(ctx)
	require.NoError(t, err)
	assert.Len(t, due, 1)

	tree, err := study.ListConcepts.Execute(ctx)
	require.NoError(t, err)
	require.Len(t, tree, 1)
	assert.Len(t, tree[0].Concepts, 2)
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

func TestRAGIngest_ZeroClientsDoesNotBecomeATypedNilTranscriber(t *testing.T) {
	dsn, _ := fileDSN(t)
	ctx := context.Background()

	sqlDB, err := OpenDatabase(ctx, testConfig(t, dsn))
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	// The dry-run shape: no client, no store. If the factory passed the zero
	// Clients' model field straight through, the interface would hold a nil
	// *llm.Client — not nil — and the use case's own nil check would pass.
	useCase := NewRAG(sqlDB).Ingest(Clients{}, nil)

	// Reflection is the only seam: the transcriber is unexported and a nil
	// interface is not observable from behaviour a dry run would exercise.
	transcriber := reflect.ValueOf(useCase).Elem().FieldByName("transcriber")
	require.True(t, transcriber.IsValid(), "the use case no longer has a transcriber field")

	assert.True(t, transcriber.IsNil(),
		"a zero Clients must narrow to a nil domain.PageTranscriber, not a nil *llm.Client")
}

func TestRequireEmbedding_AcceptableStates(t *testing.T) {
	withoutKey := testConfig(t, "file:unused")
	withoutKey.OpenRouterAPIKey = ""

	// No key is a valid configuration: the server and the commands degrade to
	// lexical search on purpose.
	require.NoError(t, RequireEmbedding(withoutKey, Clients{}))

	withKey := testConfig(t, "file:unused")
	withKey.OpenRouterAPIKey = "test-key"

	clients, err := NewEmbeddingClients(withKey)
	require.NoError(t, err)
	require.NoError(t, RequireEmbedding(withKey, clients))
}

func TestRequireEmbedding_ReportsAKeyThatBuiltNoClient(t *testing.T) {
	cfg := testConfig(t, "file:unused")
	cfg.OpenRouterAPIKey = "test-key"

	// The state a drifted wiring would produce: a key is set, so the caller
	// expects the hybrid stage, but no embedding client exists. Without this
	// guard the search would silently answer lexically.
	err := RequireEmbedding(cfg, Clients{})
	require.Error(t, err)

	assert.Contains(t, err.Error(), config.EnvOpenRouterAPIKey)
	assert.Contains(t, err.Error(), "internal wiring error")
}

func TestOpenPlanningDatabase_ReportsAnExistingPathItCannotOpen(t *testing.T) {
	dsn, path := fileDSN(t)
	ctx := context.Background()

	// A directory where the database file should be: the path exists, so the
	// "the file is not there yet" fallback does not apply, and planning against an
	// empty schema would report a book the database knows as never ingested.
	require.NoError(t, os.MkdirAll(path, 0o755))

	_, err := OpenPlanningDatabase(ctx, testConfig(t, dsn))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open planning database")
}
