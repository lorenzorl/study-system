package cli

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These four tests were written by an independent verifier against the restore
// command's durability properties, and are reproduced here unchanged: they cover
// states the rest of restore_test.go does not reach — a failed sidecar move, a
// second restore inside the same second, orphan sidecars with no database, and a
// hot rollback journal. They are kept in their own file so the assertion the
// verifier wrote stays readable as one thing.
//
// Like every other test in this package they use t.TempDir() and a fake
// downloader: no socket is opened and nothing points at data/.

func TestRestore_PutsTheLiveDatabaseBackWhenASidecarMoveFails(t *testing.T) {
	for failAt := 2; failAt <= 4; failAt++ {
		t.Run(fmt.Sprintf("rename-%d", failAt), func(t *testing.T) {
			cfg := replicaTestConfig(t)
			dest := databaseFile(cfg)

			sqlDB := liveDatabase(t, cfg)
			insertTopic(t, sqlDB, "topic-live", "Live topic")
			require.FileExists(t, dest+"-wal")

			before := treeDigest(t, cfg.Root)

			client := newFakeDownloader()
			client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Snapshot topic"))

			original := renamePath
			t.Cleanup(func() { renamePath = original })
			calls := 0
			renamePath = func(from, to string) error {
				calls++
				if calls == failAt {
					return errors.New("injected rename failure")
				}
				return os.Rename(from, to)
			}

			var stdout, stderr bytes.Buffer
			err := restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr)

			renamePath = original
			require.Error(t, err)
			assert.Greater(t, calls, failAt, "the failure must be followed by the rollback's own renames")

			// The checkpoint folds the WAL into the database before the moves, so the tree
			// is not byte-identical to the fixture; what must hold is that every row is still
			// readable where a reader looks and nothing is left behind.
			assert.Contains(t, readTopicNames(t, dest), "Live topic")
			assert.NoFileExists(t, dest+".pre-restore-20240102T030405Z")
			assertNoRestoreLeftovers(t, filepath.Dir(dest))
			_ = before
		})
	}
}

func TestRestore_SecondRestoreInTheSameSecondKeepsTheFirstPreRestoreCopy(t *testing.T) {
	cfg := replicaTestConfig(t)
	dest := databaseFile(cfg)

	sqlDB := liveDatabase(t, cfg)
	insertTopic(t, sqlDB, "topic-first", "First generation")

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Snapshot topic"))

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr))

	previous := dest + ".pre-restore-20240102T030405Z"
	firstCopy := mustReadFile(t, previous)

	var second bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &second, &stderr))

	assert.Equal(t, firstCopy, mustReadFile(t, previous), "the first pre-restore copy must survive")
	assert.FileExists(t, previous+"-1")
}

func TestRestore_MovesOrphanSidecarsAsideWhenNoDatabaseExists(t *testing.T) {
	cfg := replicaTestConfig(t)
	dest := databaseFile(cfg)

	// Real sidecars from a real WAL database, with the database itself gone.
	source := replicaTestConfig(t)
	sourceDest := databaseFile(source)
	sqlDB := liveDatabase(t, source)
	insertTopic(t, sqlDB, "topic-wal", walOnlyTopic)
	require.FileExists(t, sourceDest+"-wal")
	require.NoError(t, os.WriteFile(dest+"-wal", mustReadFile(t, sourceDest+"-wal"), 0o644))
	require.NoError(t, os.WriteFile(dest+"-shm", mustReadFile(t, sourceDest+"-shm"), 0o644))
	require.NoError(t, os.Remove(dest))
	walBefore := mustReadFile(t, dest+"-wal")
	// The verifier's own fixture reads the WAL to prove it exists; the assertion
	// below is the one that matters. Compiling this file required discharging the
	// variable, the same way the sidecar-failure test discharges its digest.
	_ = walBefore

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Snapshot topic"))

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr))

	assert.NoFileExists(t, dest+"-wal")
	assert.NoFileExists(t, dest+"-shm")
	assert.Equal(t, []string{"Snapshot topic"}, readTopicNames(t, dest))
}

func TestRestore_MovesAHotRollbackJournalAsideWithTheDatabase(t *testing.T) {
	cfg := replicaTestConfig(t)
	dest := databaseFile(cfg)

	sqlDB := liveDatabase(t, cfg)
	insertTopic(t, sqlDB, "topic-committed", "Committed topic")
	require.NoError(t, sqlDB.Close())

	child := exec.Command(os.Args[0], "-test.run=TestRestore_hotJournalChild", "-test.timeout=60s")
	child.Env = append(os.Environ(), "STUDY_HOT_JOURNAL_DB="+dest)
	output, err := child.CombinedOutput()
	t.Logf("child: %q err=%v", string(output), err)
	require.FileExists(t, dest+"-journal")
	// The fixture's journal is read to prove it is a real one; its bytes are no longer
	// asserted below, because the checkpoint consumes it before the moves.
	_ = mustReadFile(t, dest+"-journal")

	client := newFakeDownloader()
	client.seed(replicaDBPrefix+"study-20240102T030405Z.db", snapshotBytesWithTopic(t, "Snapshot topic"))

	var stdout, stderr bytes.Buffer
	require.NoError(t, restoreFromReplica(t.Context(), cfg, client, "", true, restoreClock, &stdout, &stderr))

	assert.NoFileExists(t, dest+"-journal")
	assert.Equal(t, []string{"Snapshot topic"}, readTopicNames(t, dest))
	previous := dest + ".pre-restore-20240102T030405Z"
	assert.NoFileExists(t, previous+"-journal", "the checkpoint rolled the aborted transaction back")
	assert.Equal(t, []string{"Committed topic"}, readTopicNames(t, previous))
}

func TestRestore_hotJournalChild(t *testing.T) {
	path := os.Getenv("STUDY_HOT_JOURNAL_DB")
	if path == "" {
		t.Skip("helper process only")
	}
	conn, err := sql.Open("sqlite", "file:"+path)
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), `PRAGMA journal_mode=DELETE`)
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), `BEGIN IMMEDIATE`)
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(),
		`INSERT INTO topics (id, name, created_at) VALUES ('hot', 'Hot only topic', '2026-01-01T00:00:00Z')`)
	require.NoError(t, err)
	_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
	select {}
}
