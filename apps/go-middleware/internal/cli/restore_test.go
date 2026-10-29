package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/composition"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/backup"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/r2"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
)

// The restore command is the only command in this feature that overwrites data it
// did not create, so every test here is about what survives: the snapshot, the
// database it replaces (moved aside, never deleted), and the artifacts that are
// already on disk.
//
// It is driven through restoreFromReplica with a fake downloader, so no test opens
// a socket. Everything else is real — a migrated SQLite database under t.TempDir(),
// a real VACUUM INTO snapshot, the real artifact store — which is what makes the
// WAL, the sidecars and the artifact layout the production ones.
//
// No test in this file points at data/: every path comes from t.TempDir().

// restoreClock is the fixed instant the restore names its pre-restore copy after,
// so the printed path is an exact assertion instead of a pattern.
var restoreClock = time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

// walOnlyTopic is the topic that exists only in the WAL of the live database at
// the moment a restore runs. It is the marker the WAL test looks for.
const walOnlyTopic = "topic that lives only in the WAL"

// fakeDownloader stands in for *r2.Client on the read side. It answers from the
// objects it holds and records every key it was asked for, which is how "the
// newest snapshot was chosen" and "an artifact that exists locally was never
// fetched" become observable without a bucket.
type fakeDownloader struct {
	objects map[string][]byte

	gets  []string
	heads int
	lists int

	getErrs map[string]error
	listErr error
}

func newFakeDownloader() *fakeDownloader {
	return &fakeDownloader{objects: make(map[string][]byte), getErrs: make(map[string]error)}
}

func (f *fakeDownloader) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.gets = append(f.gets, key)
	if err := f.getErrs[key]; err != nil {
		return nil, err
	}
	raw, ok := f.objects[key]
	if !ok {
		return nil, fmt.Errorf("get %q: not found", key)
	}
	return io.NopCloser(bytes.NewReader(raw)), nil
}

func (f *fakeDownloader) Head(ctx context.Context, key string) (r2.Object, bool, error) {
	if err := ctx.Err(); err != nil {
		return r2.Object{}, false, err
	}
	f.heads++
	raw, ok := f.objects[key]
	if !ok {
		return r2.Object{}, false, nil
	}
	return r2.Object{Key: key, Size: int64(len(raw))}, true, nil
}

func (f *fakeDownloader) List(ctx context.Context, prefix string) ([]r2.Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.lists++
	if f.listErr != nil {
		return nil, f.listErr
	}

	keys := make([]string, 0, len(f.objects))
	for key := range f.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	objects := make([]r2.Object, 0, len(keys))
	for _, key := range keys {
		objects = append(objects, r2.Object{Key: key, Size: int64(len(f.objects[key]))})
	}
	return objects, nil
}

func (f *fakeDownloader) seed(key string, content []byte) {
	f.objects[key] = content
}

// restoreTestConfig builds a configured home that has no database yet: a restore
// is also how a fresh home gets one.
func restoreTestConfig(t *testing.T) *config.Config {
	t.Helper()

	home := t.TempDir()
	t.Chdir(home)

	cfg, err := config.LoadWithFrom(config.Options{Home: home}, noEnvironment)
	require.NoError(t, err)

	cfg.Replica = config.ReplicaSettings{
		AccountID:       "account",
		Bucket:          "bucket",
		AccessKeyID:     "access-key-id",
		SecretAccessKey: "secret-access-key",
		Prefix:          "study-system",
	}

	return cfg
}

// databaseFile is the live database path the configuration names.
func databaseFile(cfg *config.Config) string {
	return filepath.Join(cfg.Root, "data", "study.db")
}

// snapshotBytesWithTopic builds a snapshot of this application's database holding
// one topic, in the exact form the replica stores: the bytes of a VACUUM INTO
// snapshot, taken by the production code path.
func snapshotBytesWithTopic(t *testing.T, topic string) []byte {
	t.Helper()

	dir := t.TempDir()

	sqlDB, err := db.Open(t.Context(), "file:"+filepath.Join(dir, "source.db"))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })

	require.NoError(t, db.Migrate(t.Context(), sqlDB, composition.Migrations()))
	insertTopic(t, sqlDB, "topic-"+strings.ReplaceAll(topic, " ", "-"), topic)

	snapshot, err := backup.Take(t.Context(), sqlDB, filepath.Join(dir, "snapshot.db"))
	require.NoError(t, err)

	return mustReadFile(t, snapshot.Path)
}

// snapshotBytesWithForeignSchema builds a valid SQLite database that is not this
// application's: it passes PRAGMA integrity_check and must still be refused.
func snapshotBytesWithForeignSchema(t *testing.T) []byte {
	t.Helper()

	dir := t.TempDir()

	sqlDB, err := db.Open(t.Context(), "file:"+filepath.Join(dir, "notes.db"))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })

	_, err = sqlDB.ExecContext(t.Context(), `CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL)`)
	require.NoError(t, err)

	snapshot, err := backup.Take(t.Context(), sqlDB, filepath.Join(dir, "snapshot.db"))
	require.NoError(t, err)

	return mustReadFile(t, snapshot.Path)
}

// liveDatabase opens the home's database and leaves the handle open, which is what
// keeps SQLite from checkpointing and removing the -wal file when the test ends.
func liveDatabase(t *testing.T, cfg *config.Config) *sql.DB {
	t.Helper()

	sqlDB, err := db.Open(t.Context(), cfg.DatabaseDSN)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })

	return sqlDB
}

// insertTopic commits one topic. Nothing is closed afterwards, so with a single
// open connection the row stays in the WAL until the connection goes away.
func insertTopic(t *testing.T, sqlDB *sql.DB, id, name string) {
	t.Helper()

	_, err := sqlDB.ExecContext(t.Context(),
		`INSERT INTO topics (id, name, created_at) VALUES (?, ?, ?)`,
		id, name, "2026-01-01T00:00:00Z")
	require.NoError(t, err)
}

// readTopicNames reads the topics of the database at path on a read-only
// connection of its own, so the assertion asks what a later reader sees rather
// than what the restore intended.
func readTopicNames(t *testing.T, path string) []string {
	t.Helper()

	conn, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	require.NoError(t, err)
	defer func() { assert.NoError(t, conn.Close()) }()

	rows, err := conn.QueryContext(t.Context(), `SELECT name FROM topics ORDER BY name`)
	require.NoError(t, err)
	defer rows.Close()

	names := make([]string, 0, 4)
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())

	return names
}

// assertNoRestoreLeftovers fails when the restore left scratch files in dir. The
// download lands beside the destination so the commit can be a rename; nothing may
// remain there once the command is done, with or without success.
func assertNoRestoreLeftovers(t *testing.T, dir string) {
	t.Helper()

	leftovers, err := filepath.Glob(filepath.Join(dir, ".study-restore-*"))
	require.NoError(t, err)
	assert.Empty(t, leftovers, "the restore must not leave its temporary download behind")
}

// snapshotReport validates snapshot bytes exactly the way the command does, so a
// test can assert the plan's measured numbers instead of matching them loosely.
func snapshotReport(t *testing.T, snapshot []byte) backup.Report {
	t.Helper()

	path := filepath.Join(t.TempDir(), "snapshot.db")
	require.NoError(t, os.WriteFile(path, snapshot, 0o644))

	report, err := backup.Validate(t.Context(), path)
	require.NoError(t, err)
	return report
}

func TestRun_RestoreWithoutReplicaConfigurationReturnsTheValidationError(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{
		config.EnvR2AccountID,
		config.EnvR2Bucket,
		config.EnvR2AccessKeyID,
		config.EnvR2SecretAccessKey,
		config.EnvR2Endpoint,
		config.EnvR2Prefix,
	} {
		t.Setenv(name, "")
	}

	code, stdout, stderr := runCLI(t, "restore")

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)

	// Returned unchanged: Validate names every missing variable at once, and this
	// command refuses before it looks at the replica at all (S8).
	assert.Equal(t, "restore: "+config.ReplicaSettings{}.Validate().Error()+"\n", stderr)
}

func TestRun_RestoreRejectsAPositionalArgument(t *testing.T) {
	code, stdout, stderr := runCLI(t, "restore", "extra")

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "usage: study-system restore")
}

func TestRun_RestoreHelpDocumentsTheConfirmationAndTheSidecars(t *testing.T) {
	code, stdout, stderr := runCLI(t, "restore", "-h")

	require.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "usage: study-system restore")
	assert.Contains(t, stdout, "-from")
	assert.Contains(t, stdout, "-yes")
	// S10: the snapshot is downloaded and validated before anything is replaced,
	// and the confirmation is what authorises the replacement.
	assert.Contains(t, stdout, "validated")
	assert.Contains(t, stdout, "refuses")
	// The WAL property is stated where an operator reads it, not only in the code.
	assert.Contains(t, stdout, "-wal")
	// The previous database is recoverable by hand.
	assert.Contains(t, stdout, "pre-restore")
}

func TestRun_HelpListsTheRestoreCommandNextToTheBackupCommand(t *testing.T) {
	code, stdout, _ := runCLI(t, "help")

	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "backup")
	assert.Contains(t, stdout, "restore")
}

func TestRestore_WithoutYesPrintsThePlanAndChangesNothing(t *testing.T) {
	cfg := replicaTestConfig(t)
	client := newFakeDownloader()

	snapshot := snapshotBytesWithTopic(t, "Restored topic")
	report := snapshotReport(t, snapshot)
	key := replicaDBPrefix + "study-20240102T030405Z.db"
	client.seed(key, snapshot)

	dest := databaseFile(cfg)
	previous := dest + ".pre-restore-20240102T030405Z"
	before := treeDigest(t, cfg.Root)

	var stdout, stderr bytes.Buffer
	err := restoreFromReplica(t.Context(), cfg, client, "", false, restoreClock, &stdout, &stderr)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "-yes")

	assert.Equal(t, fmt.Sprintf(
		"restore plan:\n"+
			"  snapshot: %s (%d bytes)\n"+
			"  tables:   %d\n"+
			"  database: %s\n"+
			"  previous: %s (the live database and its sidecars move here)\n",
		key, len(snapshot), len(report.Tables), dest, previous), stdout.String())

	// The same plan is printed, and the refusal is the only thing after it.
	assert.NotContains(t, stdout.String(), "restored")
	assert.Empty(t, stderr.String())

	// Nothing on disk moved: the live database, its sidecars and everything else
	// under the home are byte for byte what they were.
	assert.Equal(t, before, treeDigest(t, cfg.Root))
	assert.NoFileExists(t, previous)
	assert.NoFileExists(t, previous+"-wal")
	assertNoRestoreLeftovers(t, filepath.Dir(dest))
}

func TestRestore_InstallsIntoAHomeThatHasNoDatabaseYet(t *testing.T) {
	cfg := restoreTestConfig(t)
	client := newFakeDownloader()

	key := replicaDBPrefix + "study-20240102T030405Z.db"
	client.seed(key, snapshotBytesWithTopic(t, "Fresh topic"))

	dest := databaseFile(cfg)
	require.NoFileExists(t, dest)

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr))

	assert.Contains(t, stdout.String(), "  previous: none, nothing exists at "+dest)
	assert.Contains(t, stdout.String(), "restored "+dest+" from "+key)
	assert.Equal(t, []string{"Fresh topic"}, readTopicNames(t, dest))
	assert.Empty(t, stderr.String())
	assertNoRestoreLeftovers(t, filepath.Dir(dest))
}

func TestRestore_ChoosesTheNewestSnapshotByDefault(t *testing.T) {
	cfg := replicaTestConfig(t)
	client := newFakeDownloader()

	olderKey := replicaDBPrefix + "study-20240101T000000Z.db"
	newerKey := replicaDBPrefix + "study-20240102T030405Z.db"
	client.seed(olderKey, snapshotBytesWithTopic(t, "Older topic"))
	client.seed(newerKey, snapshotBytesWithTopic(t, "Newest topic"))
	// A key under db/ that this command did not write is not a snapshot.
	client.seed(replicaDBPrefix+"NOTES.txt", []byte("not a snapshot"))

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr))

	// Only the newest was downloaded, and it is the one that was installed.
	assert.Equal(t, []string{newerKey}, client.gets)
	assert.Equal(t, []string{"Newest topic"}, readTopicNames(t, databaseFile(cfg)))
	assert.Contains(t, stdout.String(), "restored "+databaseFile(cfg)+" from "+newerKey)
	assert.Empty(t, stderr.String())
}

func TestRestore_FromInstallsTheNamedSnapshot(t *testing.T) {
	cfg := replicaTestConfig(t)
	client := newFakeDownloader()

	olderKey := replicaDBPrefix + "study-20240101T000000Z.db"
	newerKey := replicaDBPrefix + "study-20240102T030405Z.db"
	client.seed(olderKey, snapshotBytesWithTopic(t, "Older topic"))
	client.seed(newerKey, snapshotBytesWithTopic(t, "Newest topic"))

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, olderKey, true, restoreClock, &stdout, &stderr))

	assert.Equal(t, []string{olderKey}, client.gets, "-from names one snapshot and nothing else is downloaded")
	assert.Equal(t, []string{"Older topic"}, readTopicNames(t, databaseFile(cfg)))
	assert.Contains(t, stdout.String(), "restored "+databaseFile(cfg)+" from "+olderKey)
	assert.Empty(t, stderr.String())
}

func TestRestore_FromASnapshotThatIsNotInTheReplicaFails(t *testing.T) {
	cfg := replicaTestConfig(t)
	client := newFakeDownloader()

	before := treeDigest(t, cfg.Root)
	missing := replicaDBPrefix + "study-20240101T000000Z.db"

	var stdout, stderr bytes.Buffer
	err := restoreFromReplica(t.Context(), cfg, client, missing, true, restoreClock, &stdout, &stderr)

	require.Error(t, err)
	assert.Contains(t, err.Error(), missing)
	assert.Contains(t, err.Error(), "not in the replica")
	assert.Empty(t, client.gets)
	assert.Empty(t, stdout.String())
	assert.Equal(t, before, treeDigest(t, cfg.Root))
}

func TestRestore_ReportsAReplicaWithNoSnapshotInsteadOfSucceeding(t *testing.T) {
	cfg := replicaTestConfig(t)
	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"NOTES.txt", []byte("not a snapshot"))

	before := treeDigest(t, cfg.Root)

	var stdout, stderr bytes.Buffer
	err := restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr)

	require.Error(t, err, "an empty result is an error, not a silent success")
	assert.Contains(t, err.Error(), "no snapshot under "+replicaDBPrefix)
	assert.Empty(t, client.gets)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, before, treeDigest(t, cfg.Root))
}

func TestRestore_RefusesASnapshotThatFailsValidationBeforeTouchingAnything(t *testing.T) {
	key := replicaDBPrefix + "study-20240102T030405Z.db"

	cases := []struct {
		name     string
		snapshot func(t *testing.T) []byte
		message  string
	}{
		{
			name:     "not a database",
			snapshot: func(*testing.T) []byte { return []byte("this is not a SQLite database") },
		},
		{
			name:     "a valid database that is not this application's",
			snapshot: snapshotBytesWithForeignSchema,
			message:  "missing tables",
		},
		{
			name:     "empty",
			snapshot: func(*testing.T) []byte { return nil },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := replicaTestConfig(t)
			client := newFakeDownloader()
			client.seed(key, tc.snapshot(t))

			dest := databaseFile(cfg)
			previous := dest + ".pre-restore-20240102T030405Z"
			before := treeDigest(t, cfg.Root)

			var stdout, stderr bytes.Buffer
			err := restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr)

			require.Error(t, err)
			assert.ErrorIs(t, err, backup.ErrInvalidSnapshot)
			if tc.message != "" {
				assert.Contains(t, err.Error(), tc.message)
			}

			// The failure is before the plan: nothing has been replaced, moved or
			// left behind.
			assert.Empty(t, stdout.String())
			assert.Equal(t, before, treeDigest(t, cfg.Root))
			assert.NoFileExists(t, previous)
			assertNoRestoreLeftovers(t, filepath.Dir(dest))
		})
	}
}

func TestRestore_MovesThePreviousDatabaseAndItsSidecarsAsideSoItsWALCannotBeApplied(t *testing.T) {
	cfg := replicaTestConfig(t)
	dest := databaseFile(cfg)

	// A live WAL-mode database with a stable row, and a snapshot of it taken
	// before the WAL-only row is committed: the restored content and the WAL
	// content must be distinguishable.
	sqlDB := liveDatabase(t, cfg)
	insertTopic(t, sqlDB, "topic-snapshot", "Snapshot topic")

	snapshotPath := filepath.Join(t.TempDir(), "snapshot.db")
	snapshot, err := backup.Take(t.Context(), sqlDB, snapshotPath)
	require.NoError(t, err)
	snapshotBytes := mustReadFile(t, snapshot.Path)

	// The newest committed row exists only in the WAL: the connection is still
	// open, so SQLite neither checkpoints it into the database file nor removes
	// the -wal. The bytes are asserted, which is what makes this the hazard the
	// restore has to survive and not merely a database with an open handle.
	insertTopic(t, sqlDB, "topic-wal", walOnlyTopic)
	require.FileExists(t, dest+"-wal")
	assert.Contains(t, string(mustReadFile(t, dest+"-wal")), walOnlyTopic,
		"the newest row must be WAL-resident for this test to mean anything")
	assert.NotContains(t, string(mustReadFile(t, dest)), walOnlyTopic,
		"the database file itself must not carry the newest row")

	client := newFakeDownloader()
	key := replicaDBPrefix + "study-20240102T030405Z.db"
	client.seed(key, snapshotBytes)

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr))

	previous := dest + ".pre-restore-20240102T030405Z"

	// The previous database and its sidecars left the live path together, at the path
	// the plan printed, and the previous state is recoverable by hand. Its bytes are not
	// the bytes the fixture wrote, because the write-ahead log is checkpointed before the
	// moves: the row that used to live only in the -wal is now inside the database file,
	// which is the property that closes the crash window.
	assert.Contains(t, string(mustReadFile(t, previous)), walOnlyTopic,
		"the checkpoint must have folded the WAL-only row into the database before the move")
	if wal, err := os.ReadFile(previous + "-wal"); err == nil {
		assert.Empty(t, wal, "the WAL travels with the previous database but holds nothing unique")
	}

	// No sidecar of the previous database is left where SQLite could apply it to
	// the restored file, which is the difference between a correct restore and one
	// that opens, reports success and is wrong.
	assert.NoFileExists(t, dest+"-wal")
	assert.NoFileExists(t, dest+"-shm")

	// What a reader gets is the snapshot, not the pre-restore WAL.
	assert.Equal(t, []string{"Snapshot topic"}, readTopicNames(t, dest))
	assert.Contains(t, stdout.String(), "restored "+dest+" from "+key)
	assert.Empty(t, stderr.String())
	assertNoRestoreLeftovers(t, filepath.Dir(dest))
}

func TestRestore_PutsTheLiveDatabaseBackWhenTheCommitStepFails(t *testing.T) {
	cfg := replicaTestConfig(t)
	dest := databaseFile(cfg)

	sqlDB := liveDatabase(t, cfg)
	insertTopic(t, sqlDB, "topic-live", "Live topic")

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Snapshot topic"))

	// The commit rename is the one failure this test injects. It is the only way
	// to reach the rollback path on purpose: every real cause of a failed rename
	// also prevents the moves that come before it, so the rollback would never run.
	original := renamePath
	t.Cleanup(func() { renamePath = original })

	commitAttempted := false
	renamePath = func(from, to string) error {
		if !commitAttempted && to == dest {
			commitAttempted = true
			return errors.New("injected rename failure")
		}
		return os.Rename(from, to)
	}

	var stdout, stderr bytes.Buffer
	err := restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected rename failure")
	assert.True(t, commitAttempted, "the failure must be the commit, after the previous database moved aside")

	// A failed restore leaves the live data where a reader finds it. The bytes are not
	// the ones the fixture wrote, because the checkpoint folds the WAL into the database
	// before the moves: that is the fix, not a side effect. What must hold is that every
	// row survives and nothing is left at the pre-restore path.
	assert.Contains(t, readTopicNames(t, dest), "Live topic")
	assert.NoFileExists(t, dest+".pre-restore-20240102T030405Z")
	assert.NoFileExists(t, dest+".pre-restore-20240102T030405Z-wal")

	// And the snapshot was not applied.
	assert.NotContains(t, readTopicNames(t, dest), "Snapshot topic")
	assertNoRestoreLeftovers(t, filepath.Dir(dest))
}

func TestRestore_FetchesTheMissingArtifactsAndNeverOverwritesAPresentOne(t *testing.T) {
	cfg := replicaTestConfig(t)
	artifacts := seedArtifacts(t, cfg, "kept.pdf", "fetched.pdf")

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Artifacts topic"))

	// The first artifact stays where it is, with different content than the
	// replica holds: it is immutable and already here.
	keptPayload, keptMetadata := artifactPaths(t, cfg, artifacts[0].ID)
	keptMetadataBytes := mustReadFile(t, keptMetadata)
	client.seed(replicaArtifactPrefix+artifacts[0].ID+"/payload", []byte("replica payload of kept.pdf"))
	client.seed(replicaArtifactPrefix+artifacts[0].ID+"/meta.json", []byte(`{"id":"replica"}`))
	require.NoError(t, os.WriteFile(keptPayload, []byte("local payload of kept.pdf"), 0o644))

	// The second artifact is not stored here at all.
	fetchedPayload, fetchedMetadata := artifactPaths(t, cfg, artifacts[1].ID)
	fetchedMetadataBytes := mustReadFile(t, fetchedMetadata)
	require.NoError(t, os.RemoveAll(filepath.Dir(fetchedPayload)))
	client.seed(replicaArtifactPrefix+artifacts[1].ID+"/payload", []byte("replica payload of fetched.pdf"))
	client.seed(replicaArtifactPrefix+artifacts[1].ID+"/meta.json", fetchedMetadataBytes)

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr))

	assert.Equal(t, []string{
		replicaDBPrefix + "study-20240102T030405Z.db",
		replicaArtifactPrefix + artifacts[1].ID + "/payload",
		replicaArtifactPrefix + artifacts[1].ID + "/meta.json",
	}, client.gets, "an artifact that is already here is never even fetched")

	assert.Equal(t, []byte("local payload of kept.pdf"), mustReadFile(t, keptPayload))
	assert.Equal(t, keptMetadataBytes, mustReadFile(t, keptMetadata))
	assert.Equal(t, []byte("replica payload of fetched.pdf"), mustReadFile(t, fetchedPayload))
	assert.Equal(t, fetchedMetadataBytes, mustReadFile(t, fetchedMetadata))
	assert.Contains(t, stdout.String(), "artifacts: fetched 1, skipped 1 already present, 0 failed")
	assert.Empty(t, stderr.String())

	// What was fetched is a usable artifact, not just two files in the right place.
	store, err := storage.New(cfg.StorageRoot, cfg.MaxUploadBytes)
	require.NoError(t, err)
	stored, err := store.Stat(artifacts[1].ID)
	require.NoError(t, err)
	assert.Equal(t, artifacts[1].ID, stored.ID)
}

func TestRestore_ReportsAFailedArtifactFetchAsAPartialSuccess(t *testing.T) {
	cfg := replicaTestConfig(t)
	artifacts := seedArtifacts(t, cfg, "broken.pdf", "intact.pdf")

	payload, metadata := artifactPaths(t, cfg, artifacts[0].ID)
	payloadBytes := mustReadFile(t, payload)
	metadataBytes := mustReadFile(t, metadata)
	require.NoError(t, os.RemoveAll(filepath.Dir(payload)))

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Artifacts topic"))
	client.seed(replicaArtifactPrefix+artifacts[0].ID+"/payload", payloadBytes)
	client.seed(replicaArtifactPrefix+artifacts[0].ID+"/meta.json", metadataBytes)
	client.getErrs[replicaArtifactPrefix+artifacts[0].ID+"/payload"] = errors.New("bucket said no")

	// The other artifact is in the replica and already here: it is counted as
	// skipped, which is what keeps a failed fetch from hiding the good case.
	intactPayload, intactMetadata := artifactPaths(t, cfg, artifacts[1].ID)
	client.seed(replicaArtifactPrefix+artifacts[1].ID+"/payload", mustReadFile(t, intactPayload))
	client.seed(replicaArtifactPrefix+artifacts[1].ID+"/meta.json", mustReadFile(t, intactMetadata))

	var stdout, stderr bytes.Buffer
	err := restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr)

	// F6: the restore is not a failure — the database is in place and nothing is
	// undone — but it is not a plain success either, because the local state is
	// incomplete. It carries the "partly done" code so a script can tell.
	var incomplete incompleteArtifacts
	require.ErrorAs(t, err, &incomplete, "a failed artifact is reported, not fatal")
	assert.Equal(t, 2, incomplete.ExitCode())

	assert.Contains(t, stdout.String(), "restored "+databaseFile(cfg))
	assert.Contains(t, stdout.String(), "artifacts: fetched 0, skipped 1 already present, 1 failed")
	assert.Contains(t, stderr.String(), "artifact "+artifacts[0].ID)
	assert.Contains(t, stderr.String(), "bucket said no")
	assert.NoFileExists(t, payload)
}

// The fake must satisfy the same narrow interface the real client does.
var _ downloader = (*fakeDownloader)(nil)

// F2: the crash window is between the database move and the sidecar moves. A crash
// there used to leave -wal/-shm at the live path with no database, and the next open
// creates an empty database and deletes the WAL, so the rows it held were gone.
// The checkpoint runs before the first move, so by the time the window opens the WAL
// has nothing left to lose. This test inspects the state *inside* the window, which is
// the only place the difference is observable: the rollback would otherwise put the
// sidecars back and hide whether the checkpoint happened.
func TestRestore_CheckpointsTheWALBeforeTheMovesSoTheWindowHoldsNothing(t *testing.T) {
	cfg := replicaTestConfig(t)
	dest := databaseFile(cfg)

	sqlDB := liveDatabase(t, cfg)
	insertTopic(t, sqlDB, "topic-wal", walOnlyTopic)
	require.FileExists(t, dest+"-wal", "the fixture must leave a WAL behind for this test to mean anything")

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Snapshot topic"))

	original := renamePath
	t.Cleanup(func() { renamePath = original })

	calls := 0
	renamePath = func(from, to string) error {
		calls++
		if calls == 2 {
			// The database has moved aside and the sidecars have not: this is the
			// window. Assert its state before failing the install.
			assert.NoFileExists(t, dest, "the database must already be moved aside")
			if info, err := os.Stat(dest + "-wal"); err == nil {
				assert.Zero(t, info.Size(),
					"the WAL must be truncated before the moves, or a crash here loses its rows")
			}
			return errors.New("injected rename failure")
		}
		return os.Rename(from, to)
	}

	var stdout, stderr bytes.Buffer
	err := restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr)
	renamePath = original

	require.Error(t, err)
	assert.GreaterOrEqual(t, calls, 2)
	assert.NotContains(t, stderr.String(), "could not checkpoint",
		"the checkpoint must succeed against a healthy database")
	assert.Contains(t, readTopicNames(t, dest), walOnlyTopic,
		"after the rollback the live database must still hold the WAL-only row")
}

// F4: a run killed between the download and the commit leaves its download in the
// database's directory, roughly the size of a snapshot, and nothing removed it. The
// sweep is narrow on purpose — this command deletes files, so it may only ever match
// the name its own download uses.
func TestRestore_SweepsLeftoverSnapshotDownloadsBeforeDownloading(t *testing.T) {
	cfg := replicaTestConfig(t)
	dest := databaseFile(cfg)
	dir := filepath.Dir(dest)

	// The shape a killed run leaves: the download and the sidecars validation put
	// beside it.
	leftover := filepath.Join(dir, ".study-restore-1234567890.db")
	require.NoError(t, os.WriteFile(leftover, []byte("half a snapshot"), 0o644))
	require.NoError(t, os.WriteFile(leftover+"-journal", []byte("stale journal"), 0o644))
	other := filepath.Join(dir, ".study-restore-notes.txt")

	// Everything that is not the download's own name survives, including a file
	// that starts the same way.
	require.NoError(t, os.WriteFile(other, []byte("keep me"), 0o644))

	sqlDB := liveDatabase(t, cfg)
	insertTopic(t, sqlDB, "topic-live", "Live topic")

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Snapshot topic"))

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr))

	assert.NoFileExists(t, leftover)
	assert.NoFileExists(t, leftover+"-journal")
	assert.Equal(t, []byte("keep me"), mustReadFile(t, other))

	// The count is reported, and before the download ran: the operator reads what
	// was cleared away on this run.
	assert.Contains(t, stdout.String(), "swept 1 leftover snapshot download(s) from "+dir)
	assert.Less(t, strings.Index(stdout.String(), "swept 1 leftover"), strings.Index(stdout.String(), "restore plan:"))

	assert.Equal(t, []string{"Snapshot topic"}, readTopicNames(t, dest))

	// The sweep took the leftover and the run took its own download: nothing of the
	// download's shape survives, and the operator's file was never its shape.
	remaining, err := filepath.Glob(filepath.Join(dir, restoreTempPattern))
	require.NoError(t, err)
	assert.Empty(t, remaining)
}

// The sweep unlinks entries; it must never follow a symlink into somebody else's
// file, which is the difference between removing a leftover and deleting data.
func TestRestore_SweepsLeftoversWithoutFollowingSymlinks(t *testing.T) {
	cfg := replicaTestConfig(t)
	dest := databaseFile(cfg)
	dir := filepath.Dir(dest)

	victim := filepath.Join(t.TempDir(), "victim.db")
	require.NoError(t, os.WriteFile(victim, []byte("not the restore's file"), 0o644))

	link := filepath.Join(dir, ".study-restore-symlink.db")
	require.NoError(t, os.Symlink(victim, link))

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Snapshot topic"))

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr))

	assert.NoFileExists(t, link, "the leftover entry is removed")
	assert.Equal(t, []byte("not the restore's file"), mustReadFile(t, victim),
		"the file the link pointed at is not the restore's to delete")
}

// F5: with orphan sidecars and no database the plan said "previous: none" and the
// completion line said nothing, so the operator could not find what had moved.
func TestRestore_NamesOrphanSidecarsInThePlanAndOnCompletion(t *testing.T) {
	cfg := replicaTestConfig(t)
	dest := databaseFile(cfg)
	previous := dest + ".pre-restore-20240102T030405Z"

	// A previous run left its sidecars and no database.
	require.NoError(t, os.Remove(dest))
	require.NoError(t, os.WriteFile(dest+"-wal", []byte("stale wal"), 0o644))
	require.NoError(t, os.WriteFile(dest+"-shm", []byte("stale shm"), 0o644))

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Snapshot topic"))

	var plan, stderr bytes.Buffer
	require.Error(t, restoreFromReplica(t.Context(), cfg, client, "", false, restoreClock, &plan, &stderr))

	assert.Contains(t, plan.String(), "  previous: none, nothing exists at "+dest)
	assert.Contains(t, plan.String(), dest+"-wal -> "+previous+"-wal")
	assert.Contains(t, plan.String(), dest+"-shm -> "+previous+"-shm")

	// And the completion line names where they actually are.
	var stdout bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr))

	assert.Contains(t, stdout.String(), "the previous sidecars are at "+previous+"-wal, "+previous+"-shm")
	assert.NoFileExists(t, dest+"-wal")
	assert.Equal(t, []byte("stale wal"), mustReadFile(t, previous+"-wal"))
	assert.Equal(t, []string{"Snapshot topic"}, readTopicNames(t, dest))
}

// F3: the commit is a rename, and a rename is only durable once the directory that
// holds the new entry is flushed. No filesystem state a test can arrange shows
// whether the flush happened, so the seam makes it observable — the same reason
// renamePath is a variable.
func TestRestore_SyncsTheDestinationDirectoryAfterTheCommit(t *testing.T) {
	cfg := replicaTestConfig(t)
	dest := databaseFile(cfg)

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Snapshot topic"))

	original := syncDir
	t.Cleanup(func() { syncDir = original })
	var synced []string
	syncDir = func(dir string) error {
		synced = append(synced, dir)
		return nil
	}

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr))

	assert.Equal(t, []string{filepath.Dir(dest)}, synced,
		"the new name must be flushed in the directory that holds it, or a power loss undoes the commit")
	assert.Equal(t, []string{"Snapshot topic"}, readTopicNames(t, dest))
}

// The rename already happened when the flush runs, so a flush that fails is
// reported to nobody and must not turn a completed restore into an error: there is
// nothing left to roll back.
func TestRestore_ADirectorySyncFailureDoesNotFailACommittedRestore(t *testing.T) {
	cfg := replicaTestConfig(t)
	dest := databaseFile(cfg)

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Snapshot topic"))

	original := syncDir
	t.Cleanup(func() { syncDir = original })
	syncDir = func(string) error { return errors.New("injected sync failure") }

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr),
		"the snapshot is already the live database: a failed directory flush cannot un-commit it")

	assert.Contains(t, stdout.String(), "restored "+dest+" from "+replicaDBPrefix+"study-20240102T030405Z.db")
	assert.Equal(t, []string{"Snapshot topic"}, readTopicNames(t, dest))
	assertNoRestoreLeftovers(t, filepath.Dir(dest))
}

// The hot-journal case, as the code behaves after the F2 fix. This test exists because
// the independent verifier's TestRestore_MovesAHotRollbackJournalAsideWithTheDatabase
// asserts the older property and no longer holds: the checkpoint opens the live
// database before the moves, and to change its journal mode SQLite rolls the aborted
// transaction back, which deletes the journal. Nothing committed is lost — the previous
// database still opens and still holds the committed row — so what needs pinning is that
// no journal is left where SQLite could apply it, and that the previous copy needs none.
func TestRestore_ConsumesAHotRollbackJournalBeforeTheMoves(t *testing.T) {
	cfg := replicaTestConfig(t)
	dest := databaseFile(cfg)

	sqlDB := liveDatabase(t, cfg)
	insertTopic(t, sqlDB, "topic-committed", "Committed topic")
	require.NoError(t, sqlDB.Close())

	child := exec.Command(os.Args[0], "-test.run=TestRestore_hotJournalChild", "-test.timeout=60s")
	child.Env = append(os.Environ(), "STUDY_HOT_JOURNAL_DB="+dest)
	output, err := child.CombinedOutput()
	require.Error(t, err, "the helper is killed before it can exit cleanly")
	t.Logf("child: %q err=%v", string(output), err)
	require.FileExists(t, dest+"-journal", "the fixture must leave a hot journal behind")

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Snapshot topic"))

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr))

	previous := dest + ".pre-restore-20240102T030405Z"

	assert.NoFileExists(t, dest+"-journal", "no journal may be left where the restored file could pick it up")
	assert.Equal(t, []string{"Snapshot topic"}, readTopicNames(t, dest))

	// The aborted transaction was rolled back, so the previous database is the last
	// committed state and needs no journal beside it.
	assert.NoFileExists(t, previous+"-journal")
	assert.Equal(t, []string{"Committed topic"}, readTopicNames(t, previous))
}

// partialOutcome is a command result that carries its own exit code, standing in for
// an incomplete restore without needing a bucket.
type partialOutcome struct{ code int }

func (p partialOutcome) Error() string {
	return "the database was restored, but 1 artifact could not be fetched"
}
func (p partialOutcome) ExitCode() int { return p.code }

// F6: one command needs three answers. The dispatcher must return the code the result
// asks for instead of collapsing everything that is not nil into 1.
func TestExitCodeFor_CarriesACommandsOwnExitCode(t *testing.T) {
	var stderr bytes.Buffer

	code := exitCodeFor("restore", partialOutcome{code: 2}, &stderr)

	assert.Equal(t, 2, code, "a result that names its own code must get it")
	assert.Contains(t, stderr.String(), "restore: the database was restored",
		"the outcome still has to be reported, not only counted")
}

// The mapping the other commands already relied on must not move.
func TestExitCodeFor_KeepsTheExistingMapping(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var stderr bytes.Buffer
		assert.Equal(t, 0, exitCodeFor("list", nil, &stderr))
		assert.Empty(t, stderr.String())
	})

	t.Run("help", func(t *testing.T) {
		var stderr bytes.Buffer
		assert.Equal(t, 0, exitCodeFor("list", errHelp, &stderr), "asking for help is not a failure")
		assert.Empty(t, stderr.String(), "the flag package already wrote the usage text")
	})

	t.Run("usage", func(t *testing.T) {
		var stderr bytes.Buffer
		assert.Equal(t, 1, exitCodeFor("list", errUsage, &stderr))
		assert.Empty(t, stderr.String(), "the flag package already wrote the message")
	})

	t.Run("failure", func(t *testing.T) {
		var stderr bytes.Buffer
		assert.Equal(t, 1, exitCodeFor("list", errors.New("boom"), &stderr))
		assert.Contains(t, stderr.String(), "list: boom")
	})
}

// A restore whose artifacts are all present stays a plain success: the new code must
// not leak into the complete case.
func TestRestore_CompletesWithNoErrorWhenEveryArtifactIsPresent(t *testing.T) {
	cfg := replicaTestConfig(t)
	artifacts := seedArtifacts(t, cfg, "present.pdf")

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Artifacts topic"))
	payload, metadata := artifactPaths(t, cfg, artifacts[0].ID)
	client.seed(replicaArtifactPrefix+artifacts[0].ID+"/payload", mustReadFile(t, payload))
	client.seed(replicaArtifactPrefix+artifacts[0].ID+"/meta.json", mustReadFile(t, metadata))

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr))
	assert.Contains(t, stdout.String(), "artifacts: fetched 0, skipped 1 already present, 0 failed")
}
