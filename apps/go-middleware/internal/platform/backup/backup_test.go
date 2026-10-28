package backup_test

import (
	"bytes"
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/backup"
)

// note is the only schema these tests need. It is deliberately not the
// application schema: the package knows no schema, and neither do its tests.
type note struct {
	ID   string
	Body string
}

// openSource opens a file-backed database the way the application does: WAL,
// busy_timeout, and an explicit pool size. The pool size matters below: VACUUM
// INTO needs a connection of its own, so a pool of one serialises behind any
// open transaction.
func openSource(t *testing.T, path string, maxConns int) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(maxConns)
	conn.SetMaxIdleConns(maxConns)
	t.Cleanup(func() { conn.Close() })
	require.NoError(t, conn.Ping())
	return conn
}

// execAll runs statements that are setup, not the behaviour under test.
func execAll(t *testing.T, conn *sql.DB, statements ...string) {
	t.Helper()
	for _, statement := range statements {
		_, err := conn.Exec(statement)
		require.NoError(t, err, "exec %q", statement)
	}
}

// seedNotes writes two rows into the only table in the test schema.
func seedNotes(t *testing.T, conn *sql.DB) {
	t.Helper()
	execAll(t, conn,
		`CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL)`,
		`INSERT INTO notes (id, body) VALUES ('n1','first'),('n2','second')`,
	)
}

// seedBulk writes enough rows to make a multi-page database, so a truncated or
// corrupted copy has real content to lose.
func seedBulk(t *testing.T, conn *sql.DB) {
	t.Helper()
	execAll(t, conn, `CREATE TABLE bulk (id INTEGER PRIMARY KEY, pad TEXT NOT NULL)`)
	pad := strings.Repeat("x", 2000)
	for i := 0; i < 400; i++ {
		_, err := conn.Exec(`INSERT INTO bulk (pad) VALUES (?)`, pad)
		require.NoError(t, err)
	}
}

// readOnlyDSN addresses a database that must not be written to. Validate builds
// its own URI because paths can contain characters that a naive DSN would eat;
// tests use this only with temp paths that are safe.
func readOnlyDSN(path string) string {
	return "file:" + path + "?mode=ro"
}

// readNotes reads the test schema through a fresh connection: the snapshot has
// to be usable by someone who never saw the source handle.
func readNotes(t *testing.T, dsn string) []note {
	t.Helper()
	conn, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	defer conn.Close()

	rows, err := conn.Query(`SELECT id, body FROM notes ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()

	var notes []note
	for rows.Next() {
		var n note
		require.NoError(t, rows.Scan(&n.ID, &n.Body))
		notes = append(notes, n)
	}
	require.NoError(t, rows.Err())
	return notes
}

// The core acceptance test for S6: a database written through one live WAL
// connection is snapshotted, and a fresh connection reads back exactly the rows
// the writer committed -- including the last row, which only exists in the WAL
// at snapshot time and which a plain file copy loses.
func TestTake_SnapshotsLiveWALDatabase(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	source := filepath.Join(dir, "study.db")

	// One connection keeps `wal_autocheckpoint` on the connection that does the
	// writes, so the last row is guaranteed to still be in the WAL.
	live := openSource(t, source, 1)
	execAll(t, live, `PRAGMA wal_autocheckpoint=0`)
	seedNotes(t, live)
	execAll(t, live, `INSERT INTO notes (id, body) VALUES ('n3','committed just before the snapshot')`)

	wal, err := os.Stat(source + "-wal")
	require.NoError(t, err, "the last row must still live in the WAL at snapshot time")
	require.Positive(t, wal.Size())

	snapshotPath := filepath.Join(t.TempDir(), "snapshot.db")
	snapshot, err := backup.Take(ctx, live, snapshotPath)
	require.NoError(t, err)

	assert.Equal(t, snapshotPath, snapshot.Path)
	info, err := os.Stat(snapshotPath)
	require.NoError(t, err)
	assert.Equal(t, info.Size(), snapshot.Bytes)
	assert.WithinDuration(t, time.Now(), snapshot.CreatedAt, time.Minute)

	// The snapshot is read by a connection that never saw the source handle.
	assert.Equal(t, []note{
		{ID: "n1", Body: "first"},
		{ID: "n2", Body: "second"},
		{ID: "n3", Body: "committed just before the snapshot"},
	}, readNotes(t, readOnlyDSN(snapshotPath)))

	report, err := backup.Validate(ctx, snapshotPath)
	require.NoError(t, err)
	assert.Equal(t, "ok", report.IntegrityCheck)
	assert.Equal(t, []string{"notes"}, report.Tables)
	assert.Positive(t, report.PageCount)

	// S6's counterexample, pinned: copying the live file alone is wrong. Observed
	// here, the copy opens cleanly and does not even know the table; at other
	// checkpoint boundaries it opens and is simply missing the newest rows. It
	// must never equal the live database, which is why Take never copies a file.
	naivePath := filepath.Join(t.TempDir(), "naive.db")
	raw, err := os.ReadFile(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(naivePath, raw, 0o600))

	naive, err := sql.Open("sqlite", "file:"+naivePath)
	require.NoError(t, err)
	defer naive.Close()
	var naiveCount int
	naiveErr := naive.QueryRowContext(ctx, `SELECT count(*) FROM notes`).Scan(&naiveCount)
	t.Logf("plain file copy: rows=%d err=%v", naiveCount, naiveErr)
	require.False(t, naiveErr == nil && naiveCount == 3,
		"a plain file copy must not reproduce the live database; S6 exists because it can look fine and be wrong")
}

// The destination contract: Take refuses a path that already exists instead of
// replacing it, and the caller's file survives untouched.
func TestTake_RefusesExistingDestination(t *testing.T) {
	ctx := t.Context()
	live := openSource(t, filepath.Join(t.TempDir(), "study.db"), 4)
	seedNotes(t, live)

	t.Run("non-empty file", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "snapshot.db")
		const sentinel = "this file is not ours to replace"
		require.NoError(t, os.WriteFile(dest, []byte(sentinel), 0o600))

		_, err := backup.Take(ctx, live, dest)

		require.Error(t, err)
		assert.ErrorIs(t, err, backup.ErrInvalidSnapshot)
		kept, readErr := os.ReadFile(dest)
		require.NoError(t, readErr)
		assert.Equal(t, sentinel, string(kept), "the caller's file must survive untouched")
	})

	// VACUUM INTO rejects a non-empty destination by itself ("output file
	// already exists", observed) but silently overwrites a zero-byte one
	// (observed), so the refusal cannot be delegated to SQLite.
	t.Run("zero-byte file is not silently truncated", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "empty.db")
		require.NoError(t, os.WriteFile(dest, nil, 0o600))

		_, err := backup.Take(ctx, live, dest)

		require.Error(t, err)
		assert.ErrorIs(t, err, backup.ErrInvalidSnapshot)
		info, statErr := os.Stat(dest)
		require.NoError(t, statErr)
		assert.Zero(t, info.Size(), "Take must not write to a path that already exists")
	})
}

// A failed snapshot leaves nothing behind: VACUUM INTO creates the output file
// before it reads the source, so a source that cannot be read leaves an empty
// file that a later step could mistake for a snapshot.
func TestTake_RemovesPartialFileWhenSnapshotFails(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	source := filepath.Join(dir, "study.db")

	live := openSource(t, source, 1)
	seedBulk(t, live)
	require.NoError(t, live.Close())

	raw, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Greater(t, len(raw), 5120, "the fixture database must have a page to corrupt")
	// Corrupt the table's root page; the copy fails while reading it.
	copy(raw[4096:5120], bytes.Repeat([]byte{0xFF}, 1024))
	corrupt := filepath.Join(dir, "corrupt.db")
	require.NoError(t, os.WriteFile(corrupt, raw, 0o600))
	corruptDB, err := sql.Open("sqlite", "file:"+corrupt)
	require.NoError(t, err)
	defer corruptDB.Close()

	dest := filepath.Join(t.TempDir(), "snapshot.db")
	_, err = backup.Take(ctx, corruptDB, dest)

	require.Error(t, err)
	assert.ErrorIs(t, err, backup.ErrInvalidSnapshot)
	_, statErr := os.Stat(dest)
	assert.ErrorIs(t, statErr, fs.ErrNotExist, "a failed snapshot must leave nothing behind")
}

// Observed: VACUUM INTO only reads the source, so a read-only handle can take
// the snapshot -- and it still sees the rows that live in the WAL. That is the
// answer to whether Take needs a read-write handle: it does not.
func TestTake_AcceptsReadOnlySource(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	source := filepath.Join(dir, "study.db")

	writer := openSource(t, source, 1)
	execAll(t, writer, `PRAGMA wal_autocheckpoint=0`)
	seedNotes(t, writer)
	execAll(t, writer, `INSERT INTO notes (id, body) VALUES ('n3','only in the WAL')`)

	reader, err := sql.Open("sqlite", readOnlyDSN(source)+"&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	defer reader.Close()

	dest := filepath.Join(t.TempDir(), "snapshot.db")
	_, err = backup.Take(ctx, reader, dest)

	require.NoError(t, err, "VACUUM INTO only reads the source, so a read-only handle is enough")
	assert.Len(t, readNotes(t, readOnlyDSN(dest)), 3,
		"a read-only source still sees the committed rows the WAL holds")
}

// The snapshot is transactionally consistent: a row another connection has
// written but not committed is not in it.
func TestTake_ExcludesUncommittedRowsFromOpenWriteTransaction(t *testing.T) {
	ctx := t.Context()
	live := openSource(t, filepath.Join(t.TempDir(), "study.db"), 4)
	seedNotes(t, live)

	tx, err := live.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO notes (id, body) VALUES ('pending','not committed')`)
	require.NoError(t, err)

	dest := filepath.Join(t.TempDir(), "snapshot.db")
	_, err = backup.Take(ctx, live, dest)

	require.NoError(t, err)
	assert.Equal(t, []note{
		{ID: "n1", Body: "first"},
		{ID: "n2", Body: "second"},
	}, readNotes(t, readOnlyDSN(dest)))
}

// The corresponding hazard, observed: VACUUM INTO needs a connection, so a
// caller whose pool is one connection and who holds a transaction makes Take
// wait. It fails at the context deadline and leaves no file; it is not a dead
// process and it does not produce a snapshot of the uncommitted state.
func TestTake_SingleConnectionPoolWithOpenTransactionFailsAtDeadline(t *testing.T) {
	ctx := t.Context()
	live := openSource(t, filepath.Join(t.TempDir(), "study.db"), 1)
	seedNotes(t, live)

	tx, err := live.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO notes (id, body) VALUES ('held','connection')`)
	require.NoError(t, err)

	deadline, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	dest := filepath.Join(t.TempDir(), "snapshot.db")
	_, err = backup.Take(deadline, live, dest)

	require.Error(t, err)
	assert.ErrorIs(t, err, backup.ErrInvalidSnapshot)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	_, statErr := os.Stat(dest)
	assert.ErrorIs(t, statErr, fs.ErrNotExist)
}

// Observed: the destination is bound as a query parameter ("VACUUM INTO ?"), so
// a path with a single quote and a space needs no escaping and cannot inject
// SQL.
func TestTake_DestinationPathWithQuoteAndSpace(t *testing.T) {
	ctx := t.Context()
	live := openSource(t, filepath.Join(t.TempDir(), "study.db"), 4)
	seedNotes(t, live)

	dir := filepath.Join(t.TempDir(), "o'brien's snapshots")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	dest := filepath.Join(dir, "it's a snapshot.db")

	snapshot, err := backup.Take(ctx, live, dest)

	require.NoError(t, err)
	assert.Equal(t, dest, snapshot.Path)
	report, err := backup.Validate(ctx, dest)
	require.NoError(t, err)
	assert.Equal(t, []string{"notes"}, report.Tables)
}

// Observed: a VACUUM INTO snapshot is a single file in rollback-journal mode.
// It needs no -wal or -shm beside it, which is what makes it safe to upload.
func TestTake_SnapshotStandsAloneWithoutWALSidecars(t *testing.T) {
	ctx := t.Context()
	source := filepath.Join(t.TempDir(), "study.db")
	live := openSource(t, source, 1)
	execAll(t, live, `PRAGMA wal_autocheckpoint=0`)
	seedNotes(t, live)
	execAll(t, live, `INSERT INTO notes (id, body) VALUES ('n3','still in the WAL')`)

	dest := filepath.Join(t.TempDir(), "snapshot.db")
	_, err := backup.Take(ctx, live, dest)
	require.NoError(t, err)

	for _, sidecar := range []string{"-wal", "-shm"} {
		_, statErr := os.Stat(dest + sidecar)
		assert.ErrorIs(t, statErr, fs.ErrNotExist, "the snapshot must not depend on a %s sidecar", sidecar)
	}

	// Copy only the snapshot, with no sidecars, into a directory of its own and
	// validate it there.
	raw, err := os.ReadFile(dest)
	require.NoError(t, err)
	alone := filepath.Join(t.TempDir(), "alone.db")
	require.NoError(t, os.WriteFile(alone, raw, 0o600))

	report, err := backup.Validate(ctx, alone)
	require.NoError(t, err)
	assert.Equal(t, []string{"notes"}, report.Tables)
	assert.Len(t, readNotes(t, readOnlyDSN(alone)), 3)
}

// Every "there is no valid snapshot here" failure wraps the sentinel, so a
// caller can tell it apart from an upload failure and refuse to upload.
func TestValidate_RejectsMissingPath(t *testing.T) {
	_, err := backup.Validate(t.Context(), filepath.Join(t.TempDir(), "absent.db"))

	require.Error(t, err)
	assert.ErrorIs(t, err, backup.ErrInvalidSnapshot)
}

func TestValidate_RejectsGarbageFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garbage.db")
	require.NoError(t, os.WriteFile(path, []byte("not a database, just prose from a failed upload"), 0o600))

	_, err := backup.Validate(t.Context(), path)

	require.Error(t, err)
	assert.ErrorIs(t, err, backup.ErrInvalidSnapshot)
}

func TestValidate_RejectsDirectory(t *testing.T) {
	_, err := backup.Validate(t.Context(), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, backup.ErrInvalidSnapshot)
}

// A truncated snapshot can keep a valid header and lose its tail. Observed:
// integrity_check then fails with "database disk image is malformed".
func TestValidate_RejectsTruncatedSnapshot(t *testing.T) {
	ctx := t.Context()
	live := openSource(t, filepath.Join(t.TempDir(), "study.db"), 1)
	seedBulk(t, live)

	snapshotPath := filepath.Join(t.TempDir(), "snapshot.db")
	_, err := backup.Take(ctx, live, snapshotPath)
	require.NoError(t, err)
	raw, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	require.Greater(t, len(raw), 4096, "the fixture snapshot must have more than one page")

	truncated := filepath.Join(t.TempDir(), "truncated.db")
	require.NoError(t, os.WriteFile(truncated, raw[:len(raw)/2], 0o600))

	_, err = backup.Validate(ctx, truncated)

	require.Error(t, err)
	assert.ErrorIs(t, err, backup.ErrInvalidSnapshot)
}

// Observed: a zero-byte file opens as an empty SQLite database and answers
// integrity_check with "ok", so its size has to be checked explicitly.
func TestValidate_RejectsZeroByteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zero.db")
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	_, err := backup.Validate(t.Context(), path)

	require.Error(t, err)
	assert.ErrorIs(t, err, backup.ErrInvalidSnapshot)
}

// Observed: integrity_check answers "ok" for a brand-new empty database too, so
// integrity alone does not prove a snapshot holds the data that mattered. The
// Report exposes the table list and page count instead, and this test pins the
// distinction: both snapshots are "ok", only the tables tell them apart.
func TestValidate_ValidEmptyDatabaseIsNotASnapshotOfAPopulatedOne(t *testing.T) {
	ctx := t.Context()

	empty := openSource(t, filepath.Join(t.TempDir(), "empty.db"), 1)
	emptySnapshot := filepath.Join(t.TempDir(), "empty-snapshot.db")
	_, err := backup.Take(ctx, empty, emptySnapshot)
	require.NoError(t, err, "an empty database is structurally valid; usability is the caller's judgement")

	emptyReport, err := backup.Validate(ctx, emptySnapshot)
	require.NoError(t, err)
	assert.Equal(t, "ok", emptyReport.IntegrityCheck, "observed: integrity_check answers ok for an empty database")
	assert.Empty(t, emptyReport.Tables)
	assert.Positive(t, emptyReport.PageCount)

	populated := openSource(t, filepath.Join(t.TempDir(), "populated.db"), 1)
	seedNotes(t, populated)
	populatedSnapshot := filepath.Join(t.TempDir(), "populated-snapshot.db")
	_, err = backup.Take(ctx, populated, populatedSnapshot)
	require.NoError(t, err)

	populatedReport, err := backup.Validate(ctx, populatedSnapshot)
	require.NoError(t, err)
	assert.Equal(t, "ok", populatedReport.IntegrityCheck)
	assert.Equal(t, []string{"notes"}, populatedReport.Tables)

	assert.NotEqual(t, populatedReport.Tables, emptyReport.Tables,
		"a caller must be able to tell an empty database from its own")
}

// Integrity is load-bearing on its own: a database whose schema still reads but
// whose pages are damaged must not validate. Observed: integrity_check answers
// with problem lines ("Tree 2 page 4: btreeInitPage() returns error code 11")
// rather than "ok", and Validate turns that into ErrInvalidSnapshot.
func TestValidate_RejectsDamagedPagesWhenSchemaStillReads(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	source := filepath.Join(dir, "study.db")

	live := openSource(t, source, 1)
	seedBulk(t, live)
	require.NoError(t, live.Close())

	raw, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Greater(t, len(raw), 4096*4, "the fixture needs an interior b-tree page")

	// Page 4 is inside the table's b-tree; page 1 (the schema) and page 2 (the
	// table root) stay readable, so only the integrity check can catch this.
	copy(raw[4096*3:4096*3+1024], bytes.Repeat([]byte{0xFF}, 1024))
	damaged := filepath.Join(t.TempDir(), "damaged.db")
	require.NoError(t, os.WriteFile(damaged, raw, 0o600))

	_, err = backup.Validate(ctx, damaged)

	require.Error(t, err)
	assert.ErrorIs(t, err, backup.ErrInvalidSnapshot)
}
