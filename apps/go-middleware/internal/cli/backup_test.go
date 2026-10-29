package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
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

// The backup command is tested against a fake uploader, so no test here opens a
// socket: the narrow interface the command declares is the whole seam. Everything
// the command reads is real — a real migrated SQLite database under t.TempDir(),
// a real artifact store, a real VACUUM INTO snapshot — so the keys, the byte
// counts and the validated bytes are the production ones.
//
// Nothing in this file points at data/: every path comes from t.TempDir().

// noEnvironment is the environment source the tests use. Reading the process
// environment would let a developer's STUDY_DATABASE_DSN or STUDY_R2_* decide
// what a test reads and writes, which is exactly how a test ends up pointing at a
// live database.
func noEnvironment(string) (string, bool) { return "", false }

// fakeUploader stands in for *r2.Client. It records what the command asked for
// and answers List from the objects it already holds, which is what makes the
// skip and the retention rules observable without a bucket.
type fakeUploader struct {
	objects map[string][]byte

	puts    []string
	deletes []string

	headCalls int
	listCalls int

	// deleteErrs makes a named delete fail, which is how the "a failed delete
	// must not fail the run" rule is exercised.
	deleteErrs map[string]error

	listErr error
}

func newFakeUploader() *fakeUploader {
	return &fakeUploader{objects: make(map[string][]byte), deleteErrs: make(map[string]error)}
}

func (f *fakeUploader) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if int64(len(raw)) != size {
		return fmt.Errorf("put %q: declared %d bytes, streamed %d", key, size, len(raw))
	}
	f.objects[key] = raw
	f.puts = append(f.puts, key)
	return nil
}

func (f *fakeUploader) Head(ctx context.Context, key string) (r2.Object, bool, error) {
	f.headCalls++
	raw, ok := f.objects[key]
	if !ok {
		return r2.Object{}, false, nil
	}
	return r2.Object{Key: key, Size: int64(len(raw))}, true, nil
}

func (f *fakeUploader) List(ctx context.Context, prefix string) ([]r2.Object, error) {
	f.listCalls++
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

func (f *fakeUploader) Delete(ctx context.Context, key string) error {
	if err := f.deleteErrs[key]; err != nil {
		f.deletes = append(f.deletes, key)
		return err
	}
	delete(f.objects, key)
	f.deletes = append(f.deletes, key)
	return nil
}

// seed creates an object as if an earlier run had uploaded it.
func (f *fakeUploader) seed(key string, content string) {
	f.objects[key] = []byte(content)
}

// replicaTestConfig builds a configuration rooted at a fresh temporary home with a
// migrated database, and returns it. The replica settings are filled in so the
// command has a bucket to name; no client is ever built from them here.
func replicaTestConfig(t *testing.T) *config.Config {
	t.Helper()

	home := t.TempDir()
	t.Chdir(home)

	cfg, err := config.LoadWithFrom(config.Options{Home: home}, noEnvironment)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(home, "data"), 0o755))

	// The migrated database is the real one the command snapshots: both contexts'
	// migrations run, so the snapshot carries every table the command asserts.
	sqlDB, err := composition.OpenDatabase(t.Context(), cfg)
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	cfg.Replica = config.ReplicaSettings{
		AccountID:       "account",
		Bucket:          "bucket",
		AccessKeyID:     "access-key-id",
		SecretAccessKey: "secret-access-key",
		Prefix:          "study-system",
	}

	return cfg
}

// seedArtifacts stores real artifacts through the real store, so the payload and
// metadata files the command uploads are the ones production writes.
func seedArtifacts(t *testing.T, cfg *config.Config, names ...string) []storage.Artifact {
	t.Helper()

	store, err := storage.New(cfg.StorageRoot, cfg.MaxUploadBytes)
	require.NoError(t, err)

	artifacts := make([]storage.Artifact, 0, len(names))
	for _, name := range names {
		artifact, err := store.Put(t.Context(), name, strings.NewReader("payload of "+name))
		require.NoError(t, err)
		artifacts = append(artifacts, artifact)
	}
	return artifacts
}

// artifactPaths returns the payload and metadata paths of a stored artifact.
func artifactPaths(t *testing.T, cfg *config.Config, id string) (payload, metadata string) {
	t.Helper()

	store, err := storage.New(cfg.StorageRoot, cfg.MaxUploadBytes)
	require.NoError(t, err)

	payload, err = store.Path(id)
	require.NoError(t, err)
	return payload, filepath.Join(filepath.Dir(payload), "meta.json")
}

// fileSize is the exact byte count of a file the command reports.
func fileSize(t *testing.T, path string) int64 {
	t.Helper()

	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Size()
}

// snapshotKeys builds n existing snapshot keys, one second apart, in the order the
// command's timestamp layout sorts them.
func snapshotKeys(start time.Time, n int) []string {
	keys := make([]string, 0, n)
	for i := 0; i < n; i++ {
		keys = append(keys, replicaDBPrefix+"study-"+start.Add(time.Duration(i)*time.Second).Format(snapshotKeyLayout)+".db")
	}
	return keys
}

// contradictingDatabase builds a configuration whose database is a valid SQLite
// file that is not this application's: the snapshot of it validates as SQLite and
// must still be refused for not carrying the application schema.
func contradictingDatabase(t *testing.T) *config.Config {
	t.Helper()

	home := t.TempDir()
	t.Chdir(home)

	cfg, err := config.LoadWithFrom(config.Options{Home: home}, noEnvironment)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(home, "data"), 0o755))

	sqlDB, err := db.Open(t.Context(), cfg.DatabaseDSN)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })

	_, err = sqlDB.ExecContext(t.Context(), `CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL)`)
	require.NoError(t, err)

	return cfg
}

func TestRun_BackupWithoutReplicaConfigurationReturnsTheValidationError(t *testing.T) {
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

	code, stdout, stderr := runCLI(t, "backup")

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)

	// The error is returned unchanged: it already names every missing variable at
	// once, and wrapping it would bury that list behind another prefix.
	assert.Equal(t, "backup: "+config.ReplicaSettings{}.Validate().Error()+"\n", stderr)
}

func TestRun_BackupRejectsAPositionalArgument(t *testing.T) {
	code, stdout, stderr := runCLI(t, "backup", "extra")

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "usage: study-system backup")
}

func TestRun_BackupHelpDocumentsTheDryRunAndTheLocalWorkingState(t *testing.T) {
	code, stdout, stderr := runCLI(t, "backup", "-h")

	require.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "usage: study-system backup")
	assert.Contains(t, stdout, "-dry-run")
	// The dry run's choice is printed, not left for a reader to infer: it takes a
	// snapshot into a temporary directory so the byte total is exact.
	assert.Contains(t, stdout, "temporary directory")
	// S5: data/ stays the working state and no durable file is modified. The help also
	// says what opening the database leaves behind, because an end-to-end rehearsal
	// showed study.db-shm appearing beside it: a help text claiming the command "only
	// reads" would be false, so this asserts the disclosure rather than the slogan.
	assert.Contains(t, stdout, "data/ is the working state and this command never migrates")
	assert.Contains(t, stdout, "study.db-shm")
}

func TestRun_HelpListsTheBackupCommand(t *testing.T) {
	code, stdout, _ := runCLI(t, "help")

	require.Equal(t, 0, code)
	for _, cmd := range commands() {
		assert.Contains(t, stdout, cmd.name, "every command must be listed in the usage text")
	}
}

func TestReplicate_WritesTheSnapshotAndEveryArtifactFileUnderTheKeyLayout(t *testing.T) {
	cfg := replicaTestConfig(t)
	artifacts := seedArtifacts(t, cfg, "book.pdf")
	uploader := newFakeUploader()

	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	var stdout, stderr bytes.Buffer
	require.NoError(t, replicate(t.Context(), cfg, uploader, false, now, &stdout, &stderr))

	payloadKey := replicaArtifactPrefix + artifacts[0].ID + "/payload"
	metadataKey := replicaArtifactPrefix + artifacts[0].ID + "/meta.json"

	// The snapshot comes first, then each artifact's payload and metadata: the
	// database is the part that cannot be regenerated (S2).
	assert.Equal(t, []string{
		"db/study-20240102T030405Z.db",
		payloadKey,
		metadataKey,
	}, uploader.puts)

	payloadPath, metadataPath := artifactPaths(t, cfg, artifacts[0].ID)
	assert.Equal(t, []byte("payload of book.pdf"), uploader.objects[payloadKey])
	assert.Equal(t, mustReadFile(t, metadataPath), uploader.objects[metadataKey])
	assert.Equal(t, fileSize(t, payloadPath), int64(len(uploader.objects[payloadKey])))

	// What was uploaded is a snapshot of this application, not merely a valid
	// SQLite file: it validates and carries every table the migrations create.
	uploaded := uploader.objects["db/study-20240102T030405Z.db"]
	require.True(t, strings.HasPrefix(string(uploaded), "SQLite format 3\x00"), "the snapshot must be a SQLite file")

	path := filepath.Join(t.TempDir(), "uploaded.db")
	require.NoError(t, os.WriteFile(path, uploaded, 0o644))

	report, err := backup.Validate(t.Context(), path)
	require.NoError(t, err)
	assert.Equal(t, "ok", report.IntegrityCheck)
	for _, table := range requiredTables {
		assert.Contains(t, report.Tables, table)
	}

	assert.Contains(t, stdout.String(), fmt.Sprintf("uploaded 3 objects (%d bytes), skipped 0 already present",
		int64(len(uploaded))+fileSize(t, payloadPath)+fileSize(t, metadataPath)))
	assert.Empty(t, stderr.String())
}

func TestReplicate_NeverUploadsASnapshotWithoutTheApplicationSchema(t *testing.T) {
	cfg := contradictingDatabase(t)
	seedArtifacts(t, cfg, "book.pdf")
	uploader := newFakeUploader()

	var stdout, stderr bytes.Buffer
	err := replicate(t.Context(), cfg, uploader, false, time.Now().UTC(), &stdout, &stderr)

	require.Error(t, err)
	assert.ErrorIs(t, err, backup.ErrInvalidSnapshot)
	assert.Contains(t, err.Error(), "missing tables")
	assert.Contains(t, err.Error(), "topics", "the message must name the schema it expected")

	// S10: an unvalidated snapshot is never uploaded, and neither is anything else
	// in the same run.
	assert.Empty(t, uploader.puts)
	assert.Empty(t, uploader.deletes)
	assert.Empty(t, stdout.String())
}

func TestReplicate_SkipsObjectsAlreadyPresentWithOneListPerPrefix(t *testing.T) {
	cfg := replicaTestConfig(t)
	artifacts := seedArtifacts(t, cfg, "book.pdf", "notes.pdf")

	uploader := newFakeUploader()

	// One artifact is fully replicated already; the other is not.
	presentPayload := replicaArtifactPrefix + artifacts[0].ID + "/payload"
	presentMetadata := replicaArtifactPrefix + artifacts[0].ID + "/meta.json"
	uploader.seed(presentPayload, "payload of book.pdf")
	uploader.seed(presentMetadata, "{}")

	missingPayload := replicaArtifactPrefix + artifacts[1].ID + "/payload"
	missingMetadata := replicaArtifactPrefix + artifacts[1].ID + "/meta.json"

	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	var stdout, stderr bytes.Buffer
	require.NoError(t, replicate(t.Context(), cfg, uploader, false, now, &stdout, &stderr))

	// The snapshot is always written, even though a key exists for it: its key is
	// fixed to the second and the newest bytes belong under it. The artifacts are
	// the objects that are skipped.
	assert.Equal(t, []string{
		"db/study-20240102T030405Z.db",
		missingPayload,
		missingMetadata,
	}, uploader.puts)

	// The artifact count grows, the request count must not: one List for the
	// artifacts and one for the retention listing, and no Head per file.
	assert.Equal(t, 2, uploader.listCalls)
	assert.Equal(t, 0, uploader.headCalls)

	assert.Contains(t, stdout.String(), "skipped "+presentPayload+" (already present)")
	assert.Contains(t, stdout.String(), "skipped "+presentMetadata+" (already present)")
	assert.Contains(t, stdout.String(), "uploaded 3 objects")
	assert.Contains(t, stdout.String(), "skipped 2 already present")
}

func TestReplicate_DryRunPrintsTheExactSetAndWritesNothing(t *testing.T) {
	cfg := replicaTestConfig(t)
	artifacts := seedArtifacts(t, cfg, "book.pdf")

	uploader := newFakeUploader()

	payloadKey := replicaArtifactPrefix + artifacts[0].ID + "/payload"
	metadataKey := replicaArtifactPrefix + artifacts[0].ID + "/meta.json"
	uploader.seed(metadataKey, "{}")

	// Fifteen snapshots are already stored, so the dry run also has to say what it
	// would delete, without deleting any of them.
	older := snapshotKeys(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), snapshotRetention+1)
	for _, key := range older {
		uploader.seed(key, "snapshot")
	}

	before := treeDigest(t, cfg.Root)

	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	snapshotKey := "db/study-20240102T030405Z.db"

	payloadPath, _ := artifactPaths(t, cfg, artifacts[0].ID)
	payloadBytes := fileSize(t, payloadPath)

	var stdout, stderr bytes.Buffer
	require.NoError(t, replicate(t.Context(), cfg, uploader, true, now, &stdout, &stderr))

	require.Empty(t, uploader.puts, "a dry run must not upload")
	require.Empty(t, uploader.deletes, "a dry run must not delete")

	// The snapshot's size is measured, not printed as a promise: the exact total is
	// the reason the dry run takes one. Its own bytes are the only number in the
	// output the test cannot know in advance, so it is matched as "<bytes>".
	assert.Regexp(t, `\A`+
		regexpQuote("dry run: nothing will be uploaded or deleted\n")+
		regexpQuote("replica: bucket bucket, prefix \"study-system\"\n")+
		regexpQuote("would upload "+snapshotKey)+` \(\d+ bytes\)`+
		regexpQuote("\nwould upload "+payloadKey+fmt.Sprintf(" (%d bytes)", payloadBytes))+
		regexpQuote("\nwould skip "+metadataKey+" (already present)")+
		`\nwould upload 2 objects \(\d+ bytes\), skip 1 already present`+
		regexpQuote("\nwould delete 2 snapshots beyond the newest 14\n")+
		`\z`, stdout.String())

	// Nothing durable under the home changed: the snapshot went to a temporary
	// directory outside it, and the database was only read.
	assert.Equal(t, before, treeDigest(t, cfg.Root))
	assert.Empty(t, stderr.String())
}

func TestReplicate_RetentionKeepsTheNewestFourteenAndDeletesTheOlderOnes(t *testing.T) {
	cfg := replicaTestConfig(t)
	uploader := newFakeUploader()

	older := snapshotKeys(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), snapshotRetention+2)
	for _, key := range older {
		uploader.seed(key, "snapshot")
	}
	// A key inside db/ that this command did not write is not a snapshot and must
	// not be deleted by retention.
	uploader.seed(replicaDBPrefix+"NOTES.txt", "not a snapshot")

	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	var stdout, stderr bytes.Buffer
	require.NoError(t, replicate(t.Context(), cfg, uploader, false, now, &stdout, &stderr))

	// Seventeen snapshots became seventeen with the new one; the newest fourteen
	// stay, so the three oldest are the ones that go, oldest first.
	assert.Equal(t, older[:3], uploader.deletes)
	assert.Contains(t, stdout.String(), "deleted 3 snapshots beyond the newest 14")

	for _, key := range older[3:] {
		assert.Contains(t, uploader.objects, key, "a snapshot inside the retained window must survive")
	}
	assert.Contains(t, uploader.objects, replicaDBPrefix+"NOTES.txt")
	assert.Contains(t, uploader.objects, "db/study-20240102T030405Z.db")
	assert.Empty(t, stderr.String())
}

func TestReplicate_ReportsAFailedDeleteWithoutFailingTheRun(t *testing.T) {
	cfg := replicaTestConfig(t)
	uploader := newFakeUploader()

	older := snapshotKeys(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), snapshotRetention+2)
	for _, key := range older {
		uploader.seed(key, "snapshot")
	}
	uploader.deleteErrs[older[1]] = errors.New("bucket said no")

	var stdout, stderr bytes.Buffer
	err := replicate(t.Context(), cfg, uploader, false, time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC), &stdout, &stderr)

	// The new snapshot is already up, so retention is housekeeping: it reports the
	// failure and the run still succeeds.
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "deleted 2 snapshots beyond the newest 14")
	assert.Contains(t, stderr.String(), "could not delete old snapshot "+older[1])
	assert.Contains(t, stderr.String(), "bucket said no")

	// The remaining deletions are still attempted.
	assert.Equal(t, older[:3], uploader.deletes)
	assert.NotContains(t, uploader.objects, older[0])
	assert.NotContains(t, uploader.objects, older[2])
}

func TestReplicate_ReportsAFailedRetentionListingWithoutFailingTheRun(t *testing.T) {
	cfg := replicaTestConfig(t)
	uploader := newFakeUploader()
	uploader.listErr = errors.New("list unavailable")

	var stdout, stderr bytes.Buffer
	err := replicate(t.Context(), cfg, uploader, false, time.Now().UTC(), &stdout, &stderr)

	// The artifacts listing is the first List, so this failure is real work: the
	// artifacts could not be checked for presence and the run must fail.
	require.Error(t, err)
	assert.Empty(t, uploader.puts)
}

// --- helpers -----------------------------------------------------------------

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}

// regexpQuote escapes text for the regexp assertions that have to tolerate a
// measured byte count in the middle of an exact line.
func regexpQuote(text string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`, `.`, `\.`, `+`, `\+`, `*`, `\*`, `?`, `\?`,
		`(`, `\(`, `)`, `\)`, `|`, `\|`, `[`, `\[`, `]`, `\]`,
		`{`, `\{`, `}`, `\}`, `^`, `\^`, `$`, `\$`,
	)
	return replacer.Replace(text)
}

// treeDigest maps every durable file under root to its content digest, which is how
// "a dry run wrote nothing here" becomes observable.
//
// The database's -wal and -shm companions are excluded on purpose. SQLite creates
// them for any read-only connection to a WAL database — search does exactly the same
// — they hold no rows of their own once the read finishes, and including them would
// make this assertion about the engine's shared memory instead of about the data.
func treeDigest(t *testing.T, root string) map[string]string {
	t.Helper()

	digests := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, "-wal") || strings.HasSuffix(path, "-shm") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		digests[path] = hex.EncodeToString(sum[:])
		return nil
	})
	require.NoError(t, err)
	return digests
}

// The fake must satisfy the same narrow interface the real client does.
var _ uploader = (*fakeUploader)(nil)
