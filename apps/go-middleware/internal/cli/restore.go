package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/backup"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/r2"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
)

// The restore command replaces the local state with a snapshot from the replica.
// It is the one command in this feature that overwrites data it did not create, so
// every step below is ordered by what it protects rather than by what it produces.
//
// The order is the safety property:
//
//  1. The snapshot is chosen, downloaded, and validated before anything on disk is
//     touched. A snapshot that fails validation is never installed, and the way it
//     fails is before the plan, so the live database keeps its bytes and its inodes
//     (S10).
//  2. The plan says exactly what will be replaced, and the replacement needs -yes.
//     Without it the plan is printed and the command refuses (S10).
//  3. Only then is the previous database set aside together with its sidecars and
//     the snapshot renamed into place. Restore is all-or-nothing at one point in
//     time: no rows are merged and nothing is deleted (S3).
//  4. Artifacts are fetched last, because durability covers the PDFs too and not
//     only the database (S2). A file that already exists is never overwritten.
//
// The WAL property is the reason step 3 moves the sidecars. A new study.db placed
// next to a living study.db-wal and study.db-shm can have that WAL applied to it: the
// frames are valid pages of the previous database, SQLite has no reason to doubt
// them, and the result opens, reports success and is wrong. So the previous database
// and its -wal, -shm and -journal move out of the live path in the same step, which
// leaves the restored file with no sibling SQLite could apply.

// preRestoreSuffix marks the copy the previous database is moved to. It carries a
// UTC timestamp so an operator can tell two restores apart, and it is a suffix
// rather than a directory so the move stays inside the destination's filesystem.
const preRestoreSuffix = ".pre-restore-"

// restoreTempPattern names the file the snapshot is downloaded into. It lives beside
// the destination, never in the system temporary directory: the commit is a rename,
// and a rename is only atomic within one filesystem.
const restoreTempPattern = ".study-restore-*.db"

// sqliteSidecarSuffixes are the files SQLite keeps beside a database file. All of
// them move with it: -wal and -shm are the ones that can be applied to a newly
// installed file, and -journal is the rollback journal of a hot journal.
var sqliteSidecarSuffixes = []string{"-wal", "-shm", "-journal"}

// artifactFileNames are the two files every stored artifact consists of, in the
// order they are fetched: the payload is the thing that cannot be regenerated, so it
// goes first.
var artifactFileNames = []string{artifactPayloadName, artifactMetadataName}

// renamePath is os.Rename. It is a variable for one reason: the rollback path only
// runs when a move fails midway, and no filesystem state a test can arrange reaches
// it — a permission that blocks the commit also blocks the moves before it. One test
// swaps it to fail the commit and observes the previous database coming back.
var renamePath = os.Rename

// downloader is the narrow slice of *r2.Client this command uses. The restore only
// reads the replica, so unlike backup's uploader it has no Put and no Delete: a
// command that cannot write to the bucket cannot damage the backup it restores from.
type downloader interface {
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Head(ctx context.Context, key string) (r2.Object, bool, error)
	List(ctx context.Context, prefix string) ([]r2.Object, error)
}

// The real client must satisfy it; this fails to compile if its methods drift.
var _ downloader = (*r2.Client)(nil)

// movedFile is one file the restore moved out of the live path, remembered so the
// rollback can put it back where it was.
type movedFile struct {
	From string
	To   string
}

// runRestore replaces the local state with a snapshot from the R2 replica.
func runRestore(ctx context.Context, args []string, opts options, stdout, stderr io.Writer) error {
	fs := newFlagSet("restore", restoreSynopsis, stderr)
	from := fs.String("from", "",
		"restore the snapshot stored under this key instead of the newest one")
	yes := fs.Bool("yes", false,
		"confirm replacing the live database; without it the plan is printed and nothing is changed")
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

	// Refused before the replica is read, and returned unchanged: Validate names
	// every missing variable at once, so one look at .env fixes the whole group
	// (S8). It is also the first of the two gates in front of the overwrite.
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

	return restoreFromReplica(ctx, cfg, client, *from, *yes, time.Now().UTC(), stdout, stderr)
}

// restoreSynopsis is the command's own help text. The two decisions a reader cannot
// infer from the flags are stated here: the snapshot is validated before anything is
// replaced, and the previous database's WAL moves with it instead of being left
// behind.
const restoreSynopsis = `Replace the local state with a snapshot from the R2 replica.

The newest snapshot under db/ is installed, or the one named by -from. The snapshot
is downloaded and validated before anything is replaced: it must open, answer PRAGMA
integrity_check with "ok", and carry this application's tables. A snapshot that fails
any of those checks is not installed and nothing is changed.

The plan is printed first: the snapshot key, its size, the table count, the database
it replaces, and where the previous database moves to. Installing it requires -yes.
Without -yes the same plan is printed and the command refuses with a non-zero exit
code, having changed nothing.

The previous database is never deleted or truncated: it moves aside to the pre-restore
path the plan printed, and its -wal, -shm and -journal files move with it. Moving the
WAL away with the database is the point: a -wal left next to a newly installed file
can be applied to it, which produces a database that opens, reports success and is
wrong. A failed install moves the previous database and its sidecars back.

After the database is in place, the artifacts the replica holds that are missing here
are downloaded. An artifact that is already stored is never overwritten: it is
immutable and addressed by its generated id. A failed download is reported per
artifact and does not undo the database restore that already succeeded.`

// restoreFromReplica is the command without its flags or its configuration loading,
// so the tests can drive it with a fake downloader and a fixed clock.
//
// from is the snapshot key as stored in the replica (relative to the prefix), and
// empty means "the newest one". yes is the operator's explicit confirmation; without
// it this returns an error and writes nothing.
func restoreFromReplica(
	ctx context.Context,
	cfg *config.Config,
	client downloader,
	from string,
	yes bool,
	now time.Time,
	stdout, stderr io.Writer,
) error {
	destPath, err := databasePath(cfg.DatabaseDSN)
	if err != nil {
		return err
	}

	key, err := snapshotToRestore(ctx, client, from)
	if err != nil {
		return err
	}

	// The destination directory has to exist before the download can land in it, and
	// a home that has never run anything does not have one yet.
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("prepare %s for the snapshot: %w", dir, err)
	}

	tempPath, report, err := downloadValidatedSnapshot(ctx, client, key, dir)
	if err != nil {
		return err
	}
	// Best-effort: after a successful commit the path no longer exists, and the
	// sidecars of the validated snapshot are the engine's, not the restored file's.
	defer removeSnapshotFiles(tempPath)

	previousPath := preRestorePath(destPath, now)
	previousExists := fileExists(destPath)
	printRestorePlan(stdout, key, report, destPath, previousPath, previousExists)

	// The second gate. It is checked here, after the plan, so "restore without -yes"
	// answers with what would have happened instead of only "no".
	if !yes {
		return fmt.Errorf("refusing to restore without -yes: %s is unchanged", destPath)
	}

	if err := installSnapshot(tempPath, destPath, previousPath); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "restored %s from %s\n", destPath, key)
	if previousExists {
		fmt.Fprintf(stdout, "the previous database and its sidecars are at %s\n", previousPath)
	}

	// Only now, with the database already in place, are the artifacts fetched: the
	// database is what the command is for, and a failing artifact must not turn a
	// completed restore into a failed one.
	fetchMissingArtifacts(ctx, cfg, client, stdout, stderr)
	return nil
}

// printRestorePlan says exactly what the restore is about to replace, before it
// replaces anything.
func printRestorePlan(
	stdout io.Writer,
	key string,
	report backup.Report,
	destPath, previousPath string,
	previousExists bool,
) {
	fmt.Fprintln(stdout, "restore plan:")
	// The size and the table count are the validated file's, not the object's: they
	// describe what would be installed, which is the question the operator is being
	// asked to answer.
	fmt.Fprintf(stdout, "  snapshot: %s (%d bytes)\n", key, report.Bytes)
	fmt.Fprintf(stdout, "  tables:   %d\n", len(report.Tables))
	fmt.Fprintf(stdout, "  database: %s\n", destPath)
	if previousExists {
		fmt.Fprintf(stdout, "  previous: %s (the live database and its sidecars move here)\n", previousPath)
		return
	}
	fmt.Fprintf(stdout, "  previous: none, nothing exists at %s\n", destPath)
}

// snapshotToRestore picks the snapshot to install: the one named by from, or the
// newest the replica holds.
func snapshotToRestore(ctx context.Context, client downloader, from string) (string, error) {
	if from != "" {
		// A named key is confirmed before it is downloaded, so a typo answers "there
		// is no such snapshot" instead of a transport error from the first read.
		object, found, err := client.Head(ctx, from)
		if err != nil {
			return "", fmt.Errorf("look up %s: %w", from, err)
		}
		if !found {
			return "", fmt.Errorf("snapshot %q is not in the replica", from)
		}
		return object.Key, nil
	}

	objects, err := client.List(ctx, replicaDBPrefix)
	if err != nil {
		return "", fmt.Errorf("list %s: %w", replicaDBPrefix, err)
	}

	// The key layout is fixed width and UTC, so the newest snapshot is the greatest
	// key: sorting the strings is the whole rule, and no object metadata has to be
	// trusted for the order. isSnapshotKey is the same filter retention deletes by,
	// so "what this command wrote under db/" has one definition.
	newest := ""
	for _, object := range objects {
		if !isSnapshotKey(object.Key) {
			continue
		}
		if object.Key > newest {
			newest = object.Key
		}
	}

	if newest == "" {
		return "", fmt.Errorf("the replica holds no snapshot under %s: nothing to restore", replicaDBPrefix)
	}
	return newest, nil
}

// downloadValidatedSnapshot downloads key into a temporary file inside dir and
// proves it is this application's database before returning it.
//
// Nothing outside dir is touched, and the returned path is the caller's to commit:
// a snapshot that fails validation never becomes the live database, and its error
// wraps backup.ErrInvalidSnapshot so a caller can tell "there is no valid snapshot"
// from "the download failed" (S10).
func downloadValidatedSnapshot(
	ctx context.Context,
	client downloader,
	key, dir string,
) (string, backup.Report, error) {
	body, err := client.Get(ctx, key)
	if err != nil {
		return "", backup.Report{}, fmt.Errorf("download %s: %w", key, err)
	}
	defer body.Close()

	temp, err := os.CreateTemp(dir, restoreTempPattern)
	if err != nil {
		return "", backup.Report{}, fmt.Errorf("create a temporary file in %s: %w", dir, err)
	}
	tempPath := temp.Name()

	if _, err := io.Copy(temp, body); err != nil {
		temp.Close()
		removeSnapshotFiles(tempPath)
		return "", backup.Report{}, fmt.Errorf("write the snapshot into %s: %w", tempPath, err)
	}
	// The bytes have to be on disk before the rename makes them the live database.
	// Without this, a crash right after the commit could leave a live path holding
	// less than the snapshot it was promised.
	if err := temp.Sync(); err != nil {
		temp.Close()
		removeSnapshotFiles(tempPath)
		return "", backup.Report{}, fmt.Errorf("flush the snapshot to %s: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		removeSnapshotFiles(tempPath)
		return "", backup.Report{}, fmt.Errorf("close the snapshot at %s: %w", tempPath, err)
	}

	// Validate what arrived, not what was expected. Validate opens its own
	// connection and reads the file back, which is the same check the upload had to
	// pass before the object existed at all.
	report, err := backup.Validate(ctx, tempPath)
	if err != nil {
		removeSnapshotFiles(tempPath)
		return "", backup.Report{}, fmt.Errorf("validate %s: %w", key, err)
	}

	// Integrity is not schema: an empty database is "ok" too, so the application's
	// tables are asserted here, exactly as backup asserts them before uploading.
	if err := assertApplicationSchema(report); err != nil {
		removeSnapshotFiles(tempPath)
		return "", backup.Report{}, err
	}

	return tempPath, report, nil
}

// databasePath is the file behind the configured DSN.
//
// The DSN is the only place the database path is configured (S5), so it is read
// here rather than assumed: a custom STUDY_DB_DSN must be restored to the file it
// actually names.
func databasePath(dsn string) (string, error) {
	if strings.Contains(dsn, ":memory:") || strings.Contains(dsn, "mode=memory") {
		return "", fmt.Errorf("restore replaces a database file, but %s names an in-memory database", config.EnvDatabaseDSN)
	}

	path := strings.TrimPrefix(dsn, "file:")
	if end := strings.IndexByte(path, '?'); end >= 0 {
		path = path[:end]
	}
	if path == "" {
		return "", fmt.Errorf("restore replaces a database file, but %s names no file", config.EnvDatabaseDSN)
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	return absolute, nil
}

// preRestorePath names where the previous database and its sidecars move.
//
// The name is decided before the plan is printed, so what the operator approves is
// what happens. A second restore inside the same second must not land on the first
// one's copy — the previous database is never deleted or overwritten — so the name is
// extended until it is free.
func preRestorePath(destPath string, now time.Time) string {
	base := destPath + preRestoreSuffix + now.Format(snapshotKeyLayout)

	candidate := base
	for attempt := 1; pathTaken(candidate); attempt++ {
		candidate = fmt.Sprintf("%s-%d", base, attempt)
	}
	return candidate
}

// pathTaken reports whether a database or any of its sidecars already exists at
// path, which is what makes a pre-restore name unusable.
func pathTaken(path string) bool {
	for _, candidate := range append([]string{path}, sidecarPaths(path)...) {
		if fileExists(candidate) {
			return true
		}
	}
	return false
}

// sidecarPaths lists the files SQLite may keep beside the database at path.
func sidecarPaths(path string) []string {
	paths := make([]string, 0, len(sqliteSidecarSuffixes))
	for _, suffix := range sqliteSidecarSuffixes {
		paths = append(paths, path+suffix)
	}
	return paths
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// installSnapshot commits the validated file at tempPath to destPath, moving the
// live database and its sidecars to previousPath first.
//
// Either the snapshot becomes the database and the previous one is at previousPath
// with its sidecars, or nothing changed: every move that was made is undone. The
// previous database is never deleted or truncated, so a wrongly chosen snapshot is
// recoverable by hand.
func installSnapshot(tempPath, destPath, previousPath string) error {
	// A restore replaces a database, not the permissions the operator chose for it.
	mode := os.FileMode(0o644)
	if info, err := os.Stat(destPath); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.Chmod(tempPath, mode); err != nil {
		return fmt.Errorf("set the mode of %s: %w", tempPath, err)
	}

	moved, err := moveAside(destPath, previousPath)
	if err != nil {
		return putBackAfter("move the live database aside", err, moved)
	}

	if err := renamePath(tempPath, destPath); err != nil {
		return putBackAfter("install "+destPath, err, moved)
	}

	return nil
}

// moveAside moves the live database and every sidecar out of the live path.
//
// The order is the crash order, and it is deliberate: the database goes first and the
// sidecars after it. A crash between the two therefore leaves the live path without a
// database, which SQLite will not invent one out of a stray WAL for, and the previous
// database is recoverable from the pre-restore path. Moving the WAL first would be
// the opposite: the live database would lose committed rows that only it held.
func moveAside(destPath, previousPath string) ([]movedFile, error) {
	froms := append([]string{destPath}, sidecarPaths(destPath)...)
	moved := make([]movedFile, 0, len(froms))

	for _, from := range froms {
		to := previousPath + strings.TrimPrefix(from, destPath)

		if err := renamePath(from, to); err != nil {
			// A missing sidecar is the normal case, not a failure: -journal is
			// absent in WAL mode and -wal is absent after a clean checkpoint.
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return moved, fmt.Errorf("move %s to %s: %w", from, to, err)
		}
		moved = append(moved, movedFile{From: from, To: to})
	}

	return moved, nil
}

// putBackAfter rolls the moves back and returns cause, so an install that failed
// halfway leaves the live database exactly as it was.
func putBackAfter(action string, cause error, moved []movedFile) error {
	if err := putBack(moved); err != nil {
		// Both failures are reported: the operator has to know that the live path may
		// be incomplete, and which files are where.
		return fmt.Errorf("%s: %w (and putting the previous database back failed: %v)", action, cause, err)
	}
	return fmt.Errorf("%s: %w", action, cause)
}

// putBack returns every moved file to where it came from, walking the record in
// reverse so the last file moved aside is the first one restored. Because moveAside
// moves the database first and the sidecars after it, that means the sidecars come
// back before the database does — the mirror of the move, which is what an
// interrupted install needs. The order was traced by an independent verifier rather
// than inferred: this comment previously claimed the opposite, so check it against
// the loop below instead of trusting it.
func putBack(moved []movedFile) error {
	var failures []error

	for i := len(moved) - 1; i >= 0; i-- {
		file := moved[i]
		if err := renamePath(file.To, file.From); err != nil {
			failures = append(failures, fmt.Errorf("move %s back to %s: %w", file.To, file.From, err))
		}
	}

	return errors.Join(failures...)
}

// removeSnapshotFiles removes the temporary download and any sidecar the validation
// left beside it. It is best-effort: the failure being reported is the one that
// matters, and a file left behind cannot be mistaken for the live database.
func removeSnapshotFiles(path string) {
	_ = os.Remove(path)
	for _, sidecar := range sidecarPaths(path) {
		_ = os.Remove(sidecar)
	}
}

// fetchMissingArtifacts downloads the artifacts the replica holds and this machine
// does not, and reports what it did.
//
// It runs only after the database is in place, and it never fails the command: a
// fetching problem is reported per artifact because the database restore it follows
// has already succeeded and cannot be undone by anything here (S2).
func fetchMissingArtifacts(
	ctx context.Context,
	cfg *config.Config,
	client downloader,
	stdout, stderr io.Writer,
) {
	store, err := storage.New(cfg.StorageRoot, cfg.MaxUploadBytes)
	if err != nil {
		fmt.Fprintf(stderr, "warning: artifacts were not fetched: %v\n", err)
		return
	}

	objects, err := client.List(ctx, replicaArtifactPrefix)
	if err != nil {
		fmt.Fprintf(stderr, "warning: artifacts were not fetched: list %s: %v\n", replicaArtifactPrefix, err)
		return
	}

	provided, ids := artifactFilesByID(objects)

	fetched, skipped, failed := 0, 0, 0

	for _, id := range ids {
		dir, err := localArtifactDir(store, id)
		if err != nil {
			failed++
			fmt.Fprintf(stderr, "warning: artifact %s was not fetched: %v\n", id, err)
			continue
		}

		written, err := fetchArtifactFiles(ctx, client, id, dir, provided[id])
		switch {
		case err != nil:
			failed++
			fmt.Fprintf(stderr, "warning: artifact %s was not fully fetched: %v\n", id, err)
		case written:
			fetched++
		default:
			skipped++
		}
	}

	fmt.Fprintf(stdout, "artifacts: fetched %d, skipped %d already present, %d failed\n",
		fetched, skipped, failed)
}

// artifactFilesByID groups the artifact objects the replica holds by identifier, and
// returns the identifiers in sorted order so a run reads the same every time. Keys
// that do not fit the artifact layout are somebody else's and are ignored.
func artifactFilesByID(objects []r2.Object) (map[string]map[string]struct{}, []string) {
	provided := make(map[string]map[string]struct{})
	ids := make([]string, 0, len(objects))

	for _, object := range objects {
		rest, found := strings.CutPrefix(object.Key, replicaArtifactPrefix)
		if !found {
			continue
		}
		parts := strings.Split(rest, "/")
		if len(parts) != 2 {
			continue
		}

		id, name := parts[0], parts[1]
		if name != artifactPayloadName && name != artifactMetadataName {
			continue
		}

		if _, seen := provided[id]; !seen {
			provided[id] = make(map[string]struct{}, 2)
			ids = append(ids, id)
		}
		provided[id][name] = struct{}{}
	}

	sort.Strings(ids)
	return provided, ids
}

// localArtifactDir is where the artifact with this id lives locally.
//
// The store owns the identifier's shape, so the pattern is not restated here: a
// malformed id is refused by the store and is not written anywhere. Directory also
// requires the metadata file, so ErrNotFound is what "this artifact is not here yet"
// looks like, and the path is then the one the store would use.
func localArtifactDir(store *storage.Store, id string) (string, error) {
	dir, err := store.Directory(id)
	switch {
	case err == nil:
		return dir, nil
	case errors.Is(err, apperror.ErrNotFound):
		return filepath.Join(store.Root(), id), nil
	default:
		return "", err
	}
}

// fetchArtifactFiles downloads the files of one artifact that are missing locally,
// and reports whether it wrote any.
func fetchArtifactFiles(
	ctx context.Context,
	client downloader,
	id, dir string,
	names map[string]struct{},
) (bool, error) {
	written := false

	for _, name := range artifactFileNames {
		if _, ok := names[name]; !ok {
			continue
		}

		wrote, err := fetchArtifactFile(ctx, client, id, dir, name)
		if err != nil {
			return written, err
		}
		written = written || wrote
	}

	return written, nil
}

// fetchArtifactFile downloads one artifact file unless it is already there.
//
// It never overwrites: an artifact is immutable and addressed by a generated id, so
// a file that exists is the same file, and replacing it could only replace something
// with something else.
func fetchArtifactFile(ctx context.Context, client downloader, id, dir, name string) (bool, error) {
	path := filepath.Join(dir, name)

	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return false, nil
	case !errors.Is(err, fs.ErrNotExist):
		return false, fmt.Errorf("inspect %s: %w", path, err)
	}

	key := replicaArtifactPrefix + id + "/" + name
	body, err := client.Get(ctx, key)
	if err != nil {
		return false, fmt.Errorf("download %s: %w", key, err)
	}
	defer body.Close()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("prepare %s: %w", dir, err)
	}

	// Staged and renamed for the same reason the store stages its own writes: an
	// interrupted download must not leave a short file that later looks present.
	temp, err := os.CreateTemp(dir, ".incoming-*")
	if err != nil {
		return false, fmt.Errorf("create a temporary file in %s: %w", dir, err)
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := io.Copy(temp, body); err != nil {
		temp.Close()
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return false, fmt.Errorf("close %s: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return false, fmt.Errorf("store %s: %w", path, err)
	}
	committed = true

	return true, nil
}
