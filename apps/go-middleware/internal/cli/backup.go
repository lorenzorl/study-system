package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/composition"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/backup"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/r2"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
)

// The backup command replicates data/ into the R2 bucket. It is the one command
// whose whole job is durability (S2), and it is deliberately a one-way push: the
// local directory stays the working state, nothing here writes to it, and nothing
// reads the bucket back (S5). Restore is the other command.
//
// The unit of replication is the file (S3): the database is replicated as one
// whole snapshot per run, and each artifact as its own payload and metadata. There
// is no incremental database transfer and no reconciliation, so a run that fails
// halfway leaves a prefix that the next run completes rather than a partially
// applied state.
//
// Nothing unvalidated is ever uploaded (S10). The plan is built only after the
// snapshot has been taken, validated, and checked against the schema this
// application's migrations create, so no upload can be reached by a snapshot that
// failed those checks.

// Where the replica puts things. Both prefixes are relative to Config.Replica.Prefix,
// which the R2 client applies to every key: the command never concatenates the
// prefix itself, so there is one place where it is handled.
const (
	replicaDBPrefix       = "db/"
	replicaArtifactPrefix = "artifacts/"

	// snapshotKeyLayout makes the key sort chronologically when it is compared as a
	// string, which is the property retention depends on: it keeps the newest n
	// snapshots by sorting the keys, not by trusting object metadata.
	//
	// It is fixed width and UTC to the second. Two runs inside the same second
	// therefore address the same key, and the second one replaces the first: that is
	// why the snapshot is always uploaded and never skipped as "already present",
	// while an artifact, whose key carries its immutable identifier, is skipped.
	snapshotKeyLayout = "20060102T150405Z"

	// snapshotRetention is how many snapshots the prefix keeps. Full snapshots
	// accumulate linearly, so the count is bounded by a number rather than by
	// hope; a snapshot is only deleted once a newer one is safely uploaded, so the
	// change is always "14 copies, then 14 copies including the new one".
	snapshotRetention = 14

	// snapshotFileName is the name of the snapshot inside the temporary directory
	// it is taken into. The directory is fresh, which is what backup.Take requires:
	// it refuses a destination that already exists.
	snapshotFileName = "study.db"

	// snapshotTempDirPattern names the temporary directory holding the snapshot
	// while it is uploaded. It is created with os.MkdirTemp, so it lives in the
	// system temporary directory and never inside the repository or under data/.
	snapshotTempDirPattern = "study-system-backup-"
)

// Content types of the objects the replica holds, and the two file names every
// stored artifact consists of.
const (
	contentTypeSQLite   = "application/vnd.sqlite3"
	contentTypeJSON     = "application/json"
	contentTypeArtifact = "application/octet-stream"

	artifactPayloadName  = "payload"
	artifactMetadataName = "meta.json"
)

// requiredTables are the tables a freshly migrated database always has, and the
// evidence that a snapshot is a snapshot of this application rather than of some
// other valid SQLite file.
//
// It is the union of the two contexts' schemas: study owns topics, concepts,
// flashcards, resources, card_states and review_logs, and ingestion owns documents,
// ingestion_jobs, job_pages, chunks, chunk_embeddings and document_sections. The
// list is sorted so the message naming what is missing reads the same every run.
//
// chunks_fts is deliberately absent. It is an FTS5 virtual table, its presence in
// sqlite_schema depends on the module the reader was built with, and its content is
// rebuilt from chunks by "study-system reindex" — while chunks is the source of
// truth and is asserted above.
var requiredTables = []string{
	"card_states",
	"chunk_embeddings",
	"chunks",
	"concepts",
	"document_sections",
	"documents",
	"flashcards",
	"ingestion_jobs",
	"job_pages",
	"resources",
	"review_logs",
	"topics",
}

// uploader is the narrow slice of *r2.Client this command uses. Declaring it here
// keeps the command testable against a fake, so the tests need no socket, no
// server and no clock: the only thing they cannot check is the wire, which the r2
// package's own tests cover.
type uploader interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Head(ctx context.Context, key string) (r2.Object, bool, error)
	List(ctx context.Context, prefix string) ([]r2.Object, error)
	Delete(ctx context.Context, key string) error
}

// The real client must satisfy it; this fails to compile if its methods drift.
var _ uploader = (*r2.Client)(nil)

// replicaObject is one object to write, with the local file it comes from.
type replicaObject struct {
	Key         string
	Path        string
	Bytes       int64
	ContentType string

	// AlreadyPresent means the bucket already holds this key, so the upload is
	// skipped. Only artifact keys are ever skipped.
	AlreadyPresent bool
}

// replicaPlan is everything the command decided before it wrote anything.
type replicaPlan struct {
	// SnapshotKey is the key the validated snapshot is written under.
	SnapshotKey string

	// Objects are the uploads in order: the snapshot first, then each artifact's
	// payload and metadata.
	Objects []replicaObject
}

// runBackup snapshots the database and the artifact store into the R2 replica.
func runBackup(ctx context.Context, args []string, opts options, stdout, stderr io.Writer) error {
	fs := newFlagSet("backup", backupSynopsis, stderr)
	dryRun := fs.Bool("dry-run", false,
		"report the objects and the byte total without uploading or deleting anything")
	if err := parseFlags(fs, args, stdout); err != nil {
		return err
	}
	if err := rejectExtraArgs(fs); err != nil {
		return err
	}

	cfg, err := loadConfigAndAnnounce(opts, stderr)
	if err != nil {
		return err
	}

	// Refused before anything is read, and returned unchanged: Validate names every
	// missing variable at once, so one look at .env fixes the whole group (S8).
	if err := cfg.Replica.Validate(); err != nil {
		return err
	}

	client, err := r2.New(r2.Config{
		Endpoint:        cfg.Replica.EffectiveEndpoint(),
		Bucket:          cfg.Replica.Bucket,
		AccessKeyID:     cfg.Replica.AccessKeyID,
		SecretAccessKey: cfg.Replica.SecretAccessKey,
		Prefix:          cfg.Replica.Prefix,
	})
	if err != nil {
		return fmt.Errorf("open the replica client: %w", err)
	}

	return replicate(ctx, cfg, client, *dryRun, time.Now().UTC(), stdout, stderr)
}

// backupSynopsis is the command's own help text. The dry run's one decision that a
// reader cannot infer is stated here: it takes a snapshot, into a temporary
// directory, because the byte total it prints has to be exact.
const backupSynopsis = `Copy the database and the artifact store into the R2 replica.

The database is replicated as one transactionally consistent snapshot per run, and
every stored artifact as its own payload and metadata. The snapshot is uploaded only
after it validates and carries the application schema, so a snapshot that fails is
never uploaded. Objects already in the bucket are skipped, and after the new snapshot
is up, all but the newest 14 snapshots under db/ are deleted.

data/ is the working state and this command only reads it: it never migrates, moves
or rewrites anything there, and it works on a copy.

-dry-run takes the snapshot into a temporary directory, reports its exact size, and
prints the objects and the byte total it would upload and delete. It uploads and
deletes nothing; the temporary snapshot is removed before the command returns.`

// replicate is the command without its flags or its configuration loading, so the
// tests can drive it with a fake uploader, a fixed clock and a temporary home.
func replicate(
	ctx context.Context,
	cfg *config.Config,
	client uploader,
	dryRun bool,
	now time.Time,
	stdout, stderr io.Writer,
) error {
	plan, cleanup, err := planReplica(ctx, cfg, client, now)
	if err != nil {
		return err
	}
	// The snapshot only exists to be uploaded. Removing it is best-effort: a
	// leftover temporary file costs disk, while a failure to remove it must not
	// turn a successful upload into a failed run.
	defer cleanup()

	if dryRun {
		return reportDryRun(ctx, cfg, client, plan, stdout)
	}

	uploaded, skipped, bytes := 0, 0, int64(0)

	for _, object := range plan.Objects {
		if object.AlreadyPresent {
			skipped++
			writeObjectLine(stdout, object, false)
			continue
		}

		if err := putObject(ctx, client, object); err != nil {
			return err
		}
		uploaded++
		bytes += object.Bytes
		writeObjectLine(stdout, object, false)
	}

	fmt.Fprintf(stdout, "uploaded %d objects (%d bytes), skipped %d already present\n",
		uploaded, bytes, skipped)

	// Only now, with the new snapshot safely in the bucket, is anything deleted.
	pruneSnapshots(ctx, client, stdout, stderr)
	return nil
}

// planReplica takes the snapshot, proves it is this application's, and decides the
// upload set. It returns the cleanup that removes the temporary directory holding
// the snapshot, because the upload has to happen before it is removed.
//
// Nothing is written to the bucket here: the presence of artifact objects is read
// with one List per prefix, and the deletion of old snapshots is decided later,
// from a listing taken after the upload.
func planReplica(
	ctx context.Context,
	cfg *config.Config,
	client uploader,
	now time.Time,
) (replicaPlan, func(), error) {
	tempDir, err := os.MkdirTemp("", snapshotTempDirPattern)
	if err != nil {
		return replicaPlan{}, nil, fmt.Errorf("create a temporary directory for the snapshot: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	snapshot, err := takeValidatedSnapshot(ctx, cfg, filepath.Join(tempDir, snapshotFileName))
	if err != nil {
		cleanup()
		return replicaPlan{}, nil, err
	}

	snapshotKey := replicaDBPrefix + "study-" + now.Format(snapshotKeyLayout) + ".db"

	plan := replicaPlan{
		SnapshotKey: snapshotKey,
		Objects: []replicaObject{{
			Key:         snapshotKey,
			Path:        snapshot.Path,
			Bytes:       snapshot.Bytes,
			ContentType: contentTypeSQLite,
		}},
	}

	artifacts, err := artifactObjects(cfg)
	if err != nil {
		cleanup()
		return replicaPlan{}, nil, err
	}

	// One List for the whole artifact prefix, not one Head per file: the artifact
	// count only grows, and the request count must not grow with it.
	present, err := listKeys(ctx, client, replicaArtifactPrefix)
	if err != nil {
		cleanup()
		return replicaPlan{}, nil, err
	}
	for _, object := range artifacts {
		_, object.AlreadyPresent = present[object.Key]
		plan.Objects = append(plan.Objects, object)
	}

	return plan, cleanup, nil
}

// takeValidatedSnapshot writes a snapshot of the live database into destPath and
// proves two things before returning it: the file is a usable SQLite database, and
// it carries the schema this application's migrations create.
//
// The database is opened read-only. This command must not be the thing that
// migrates data/ (S5), and the snapshot does not need write access: VACUUM INTO
// reads through a read transaction and a read-only handle still sees rows that live
// only in the WAL.
func takeValidatedSnapshot(ctx context.Context, cfg *config.Config, destPath string) (backup.Snapshot, error) {
	sqlDB, err := composition.OpenReadOnlyDatabase(ctx, cfg)
	if err != nil {
		return backup.Snapshot{}, fmt.Errorf("open the database to snapshot: %w", err)
	}
	defer sqlDB.Close()

	// Take already validates what it produced; the second Validate is what returns
	// the Report, and the table assertion below is the part only this caller can do
	// because the backup package knows no schema.
	snapshot, err := backup.Take(ctx, sqlDB, destPath)
	if err != nil {
		return backup.Snapshot{}, err
	}

	report, err := backup.Validate(ctx, snapshot.Path)
	if err != nil {
		return backup.Snapshot{}, err
	}

	if err := assertApplicationSchema(report); err != nil {
		return backup.Snapshot{}, err
	}

	return snapshot, nil
}

// assertApplicationSchema fails when a valid database is not this application's.
//
// It is the check that makes the upload mean something: PRAGMA integrity_check
// answers "ok" for an empty database too, so integrity alone says the file is not
// damaged, while this says the file is the data that mattered.
func assertApplicationSchema(report backup.Report) error {
	present := make(map[string]struct{}, len(report.Tables))
	for _, name := range report.Tables {
		present[name] = struct{}{}
	}

	missing := make([]string, 0, len(requiredTables))
	for _, name := range requiredTables {
		if _, ok := present[name]; !ok {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("%w: snapshot %q does not carry the application schema: missing tables %s",
			backup.ErrInvalidSnapshot, report.Path, strings.Join(missing, ", "))
	}
	return nil
}

// artifactObjects lists the files to upload for every stored artifact.
func artifactObjects(cfg *config.Config) ([]replicaObject, error) {
	store, err := storage.New(cfg.StorageRoot, cfg.MaxUploadBytes)
	if err != nil {
		return nil, fmt.Errorf("open storage at %s: %w", cfg.StorageRoot, err)
	}

	artifacts, err := store.List()
	if err != nil {
		return nil, err
	}

	objects := make([]replicaObject, 0, 2*len(artifacts))

	for _, artifact := range artifacts {
		payloadPath, err := store.Path(artifact.ID)
		if err != nil {
			return nil, err
		}
		// The metadata file sits beside the payload. The store owns both names and
		// Path is the authority for the payload's, so this derives the other one
		// from it instead of restating the layout in a second place.
		metadataPath := filepath.Join(filepath.Dir(payloadPath), artifactMetadataName)

		files := []struct {
			path        string
			name        string
			contentType string
		}{
			{payloadPath, artifactPayloadName, contentTypeArtifact},
			{metadataPath, artifactMetadataName, contentTypeJSON},
		}

		for _, file := range files {
			info, err := os.Stat(file.path)
			if err != nil {
				// An artifact whose files cannot be read is not uploadable, and
				// skipping it silently would lose exactly what this command exists
				// to keep (S2).
				return nil, fmt.Errorf("inspect %s%s/%s: %w", replicaArtifactPrefix, artifact.ID, file.name, err)
			}

			objects = append(objects, replicaObject{
				Key:         replicaArtifactPrefix + artifact.ID + "/" + file.name,
				Path:        file.path,
				Bytes:       info.Size(),
				ContentType: file.contentType,
			})
		}
	}

	return objects, nil
}

// listKeys returns the set of keys the bucket holds under prefix.
func listKeys(ctx context.Context, client uploader, prefix string) (map[string]struct{}, error) {
	objects, err := client.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", prefix, err)
	}

	keys := make(map[string]struct{}, len(objects))
	for _, object := range objects {
		keys[object.Key] = struct{}{}
	}
	return keys, nil
}

// listSnapshotKeys returns the snapshot keys the bucket holds, sorted.
func listSnapshotKeys(ctx context.Context, client uploader) ([]string, error) {
	objects, err := client.List(ctx, replicaDBPrefix)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", replicaDBPrefix, err)
	}

	keys := make([]string, 0, len(objects))
	for _, object := range objects {
		if isSnapshotKey(object.Key) {
			keys = append(keys, object.Key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// isSnapshotKey reports whether key is one of this command's snapshots.
//
// Retention deletes what it finds, so it only deletes what it recognises: a key
// under db/ that this command did not write is somebody else's object, and deleting
// it would be worse than leaving an extra snapshot behind.
func isSnapshotKey(key string) bool {
	name, found := strings.CutPrefix(key, replicaDBPrefix)
	if !found {
		return false
	}
	if strings.Contains(name, "/") {
		return false
	}
	_, ok := strings.CutSuffix(name, ".db")
	return ok && strings.HasPrefix(name, "study-")
}

// staleSnapshotKeys returns the snapshot keys to delete: everything older than the
// newest keep, oldest first. The keys sort chronologically because the timestamp
// they carry has a fixed width, so sorting the strings is the whole rule.
func staleSnapshotKeys(keys []string, keep int) []string {
	if keep < 1 || len(keys) == 0 {
		return nil
	}

	sorted := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)

	if len(sorted) <= keep {
		return nil
	}
	return sorted[:len(sorted)-keep]
}

// putObject streams one file to the bucket.
func putObject(ctx context.Context, client uploader, object replicaObject) error {
	file, err := os.Open(object.Path)
	if err != nil {
		return fmt.Errorf("open %s to upload: %w", object.Key, err)
	}
	defer file.Close()

	if err := client.Put(ctx, object.Key, file, object.Bytes, object.ContentType); err != nil {
		return fmt.Errorf("upload %s: %w", object.Key, err)
	}
	return nil
}

// writeObjectLine reports one object: what happened to it, or with dryRun, what
// would have happened.
func writeObjectLine(w io.Writer, object replicaObject, dryRun bool) {
	action := objectAction(object, dryRun)

	if object.AlreadyPresent {
		fmt.Fprintf(w, "%s %s (already present)\n", action, object.Key)
		return
	}
	fmt.Fprintf(w, "%s %s (%d bytes)\n", action, object.Key, object.Bytes)
}

// objectAction is the verb phrase of one object's line. It is spelled out rather
// than assembled, so the real run and the dry run each read as a sentence.
func objectAction(object replicaObject, dryRun bool) string {
	switch {
	case dryRun && object.AlreadyPresent:
		return "would skip"
	case dryRun:
		return "would upload"
	case object.AlreadyPresent:
		return "skipped"
	default:
		return "uploaded"
	}
}

// reportDryRun prints exactly what a real run would do, and writes nothing.
//
// The snapshot it measures is real, which is the only way the byte total can be
// exact; it is taken into a temporary directory and discarded by the caller.
func reportDryRun(
	ctx context.Context,
	cfg *config.Config,
	client uploader,
	plan replicaPlan,
	stdout io.Writer,
) error {
	// The listing is taken before the upload, so the new key is added to it here.
	existing, err := listSnapshotKeys(ctx, client)
	if err != nil {
		return err
	}
	stale := staleSnapshotKeys(append(existing, plan.SnapshotKey), snapshotRetention)

	fmt.Fprintln(stdout, "dry run: nothing will be uploaded or deleted")
	// Keys are relative to the prefix the client applies, so the target is named:
	// "db/study-....db" on its own would not say where it lands.
	fmt.Fprintf(stdout, "replica: bucket %s, prefix %q\n", cfg.Replica.Bucket, cfg.Replica.Prefix)

	uploaded, skipped, bytes := 0, 0, int64(0)
	for _, object := range plan.Objects {
		if object.AlreadyPresent {
			skipped++
			writeObjectLine(stdout, object, true)
			continue
		}
		uploaded++
		bytes += object.Bytes
		writeObjectLine(stdout, object, true)
	}

	fmt.Fprintf(stdout, "would upload %d objects (%d bytes), skip %d already present\n",
		uploaded, bytes, skipped)

	if len(stale) > 0 {
		fmt.Fprintf(stdout, "would delete %d snapshots beyond the newest %d\n",
			len(stale), snapshotRetention)
	}

	return nil
}

// pruneSnapshots deletes the snapshots beyond the newest snapshotRetention.
//
// It runs only after the new snapshot is uploaded and it never fails the run: the
// data is safe by then, so a delete that does not go through is housekeeping to
// report, not a reason to tell an operator the backup failed.
func pruneSnapshots(ctx context.Context, client uploader, stdout, stderr io.Writer) {
	keys, err := listSnapshotKeys(ctx, client)
	if err != nil {
		fmt.Fprintf(stderr, "warning: could not list snapshots for retention: %v\n", err)
		return
	}

	deleted := 0
	for _, key := range staleSnapshotKeys(keys, snapshotRetention) {
		if err := client.Delete(ctx, key); err != nil {
			fmt.Fprintf(stderr, "warning: could not delete old snapshot %s: %v\n", key, err)
			continue
		}
		deleted++
	}

	if deleted > 0 {
		fmt.Fprintf(stdout, "deleted %d snapshots beyond the newest %d\n", deleted, snapshotRetention)
	}
}
