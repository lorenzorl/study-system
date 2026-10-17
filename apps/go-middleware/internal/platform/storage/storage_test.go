package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
)

const testMaxBytes = 4096

func newStore(t *testing.T) *storage.Store {
	t.Helper()

	store, err := storage.New(t.TempDir(), testMaxBytes)
	require.NoError(t, err)
	return store
}

func put(t *testing.T, store *storage.Store, name string, content []byte) storage.Artifact {
	t.Helper()

	artifact, err := store.Put(t.Context(), name, bytes.NewReader(content))
	require.NoError(t, err)
	return artifact
}

func readAll(t *testing.T, store *storage.Store, id string) []byte {
	t.Helper()

	reader, err := store.Open(t.Context(), id)
	require.NoError(t, err)
	defer reader.Close()

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	return content
}

// --- construction ---

func TestNew_RejectsEmptyRoot(t *testing.T) {
	for _, root := range []string{"", "   "} {
		_, err := storage.New(root, testMaxBytes)

		require.Error(t, err, "root %q must be rejected", root)
		assert.ErrorIs(t, err, apperror.ErrInvalidInput)
	}
}

func TestNew_RejectsNonPositiveMaxBytes(t *testing.T) {
	for _, maxBytes := range []int64{0, -1} {
		_, err := storage.New(t.TempDir(), maxBytes)

		require.Error(t, err, "maxBytes %d must be rejected", maxBytes)
		assert.ErrorIs(t, err, apperror.ErrInvalidInput)
	}
}

func TestNew_CreatesRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "artifacts")

	store, err := storage.New(root, testMaxBytes)
	require.NoError(t, err)

	info, err := os.Stat(root)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, root, store.Root())
	assert.Equal(t, int64(testMaxBytes), store.MaxBytes())
}

// --- round trip ---

func TestPut_RoundTripsTextContent(t *testing.T) {
	store := newStore(t)
	content := []byte("question::answer\n")

	artifact := put(t, store, "book.pdf", content)

	assert.Equal(t, content, readAll(t, store, artifact.ID))
	assert.Equal(t, int64(len(content)), artifact.Size)
	assert.Equal(t, "book.pdf", artifact.OriginalName)
	assert.NotEmpty(t, artifact.ID)
	assert.False(t, artifact.StoredAt.IsZero())
}

func TestPut_RoundTripsBinaryContentUnchanged(t *testing.T) {
	store := newStore(t)
	content := make([]byte, 256)
	for i := range content {
		content[i] = byte(i)
	}

	artifact := put(t, store, "scan.pdf", content)

	assert.Equal(t, content, readAll(t, store, artifact.ID),
		"binary content must survive byte for byte")
}

func TestPut_EmptyContentIsAccepted(t *testing.T) {
	store := newStore(t)

	artifact := put(t, store, "empty.pdf", nil)

	assert.Equal(t, int64(0), artifact.Size)
	assert.Empty(t, readAll(t, store, artifact.ID))
}

func TestPut_AssignsDistinctIdentifiers(t *testing.T) {
	store := newStore(t)

	seen := map[string]bool{}
	for range 5 {
		artifact := put(t, store, "book.pdf", []byte("x"))

		assert.False(t, seen[artifact.ID], "identifier %q was reused", artifact.ID)
		seen[artifact.ID] = true
	}
}

func TestPut_IsSafeUnderConcurrency(t *testing.T) {
	store := newStore(t)

	const workers = 16
	ids := make([]string, workers)
	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			artifact, err := store.Put(t.Context(), "book.pdf", strings.NewReader("content"))
			if err == nil {
				ids[index] = artifact.ID
			}
		}(i)
	}
	wg.Wait()

	seen := map[string]bool{}
	for i, id := range ids {
		require.NotEmpty(t, id, "worker %d produced no artifact", i)
		assert.False(t, seen[id], "duplicate identifier %q", id)
		seen[id] = true
	}
}

// --- size limit ---

func TestPut_RejectsContentOverTheLimit(t *testing.T) {
	store := newStore(t)

	_, err := store.Put(t.Context(), "huge.pdf", bytes.NewReader(make([]byte, testMaxBytes+1)))

	require.Error(t, err)
	assert.ErrorIs(t, err, apperror.ErrTooLarge)
}

func TestPut_AcceptsContentExactlyAtTheLimit(t *testing.T) {
	store := newStore(t)

	artifact, err := store.Put(t.Context(), "exact.pdf", bytes.NewReader(make([]byte, testMaxBytes)))
	require.NoError(t, err)

	assert.Equal(t, int64(testMaxBytes), artifact.Size)
}

// A rejected transfer must leave nothing that later looks like a valid artifact.
func TestPut_OversizedTransferLeavesNoArtifact(t *testing.T) {
	store := newStore(t)

	_, err := store.Put(t.Context(), "huge.pdf", bytes.NewReader(make([]byte, testMaxBytes+100)))
	require.Error(t, err)

	artifacts, listErr := store.List()
	require.NoError(t, listErr)
	assert.Empty(t, artifacts, "a rejected transfer must not be listed")

	entries, readErr := os.ReadDir(store.Root())
	require.NoError(t, readErr)
	for _, entry := range entries {
		if entry.Name() == "incoming" {
			continue
		}
		t.Errorf("unexpected leftover entry in the store root: %s", entry.Name())
	}
}

// --- interrupted and cancelled transfers ---

// failingReader yields some bytes and then fails, standing in for a dropped
// connection or a truncated disk read.
type failingReader struct {
	content []byte
	err     error
	done    bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	n := copy(p, r.content)
	return n, nil
}

func TestPut_InterruptedTransferLeavesNoArtifact(t *testing.T) {
	store := newStore(t)
	boom := errors.New("connection reset")

	_, err := store.Put(t.Context(), "book.pdf", &failingReader{content: []byte("partial"), err: boom})

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)

	artifacts, listErr := store.List()
	require.NoError(t, listErr)
	assert.Empty(t, artifacts, "an interrupted transfer must not be listed")
}

func TestPut_RespectsContextCancellation(t *testing.T) {
	store := newStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := store.Put(ctx, "book.pdf", bytes.NewReader([]byte("content")))

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)

	artifacts, listErr := store.List()
	require.NoError(t, listErr)
	assert.Empty(t, artifacts)
}

func TestPut_RejectsNilReader(t *testing.T) {
	_, err := newStore(t).Put(t.Context(), "book.pdf", nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, apperror.ErrInvalidInput)
}

// --- hostile original names never become paths ---

func TestPut_HostileOriginalNamesNeverReachTheFilesystem(t *testing.T) {
	hostile := []string{
		"../../etc/passwd",
		"../../../root/.ssh/id_rsa",
		"/etc/passwd",
		"..\\..\\windows\\system32\\config",
		"book.pdf/../../escape",
		"....//....//escape.txt",
		".",
		"..",
		"",
	}

	for _, name := range hostile {
		t.Run(name, func(t *testing.T) {
			store := newStore(t)

			artifact, err := store.Put(t.Context(), name, bytes.NewReader([]byte("safe")))
			require.NoError(t, err)

			// The artifact directory is the generated identifier, and it must be
			// the only new thing under the root.
			dir, err := store.Directory(artifact.ID)
			require.NoError(t, err)
			assert.Equal(t, store.Root(), filepath.Dir(dir))
			assert.Equal(t, artifact.ID, filepath.Base(dir))

			// The payload filename carries no caller input.
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			for _, entry := range entries {
				assert.NotContains(t, entry.Name(), "..")
				assert.NotContains(t, entry.Name(), string(filepath.Separator))
			}

			// And the content is still readable.
			assert.Equal(t, []byte("safe"), readAll(t, store, artifact.ID))
		})
	}
}

func TestPut_SanitisesOriginalNameIntoMetadataOnly(t *testing.T) {
	store := newStore(t)

	artifact := put(t, store, "../../books/domain.pdf", []byte("x"))

	assert.Equal(t, "domain.pdf", artifact.OriginalName,
		"directory components must be stripped from the recorded name")
}

// The payload filename is fixed, so neither a caller-supplied name nor a
// metadata field can influence where content is read from.
func TestPut_AlwaysStoresTheSamePayloadName(t *testing.T) {
	names := []string{
		"book.pdf",
		"BOOK.PDF",
		"book.tar.gz",
		"noextension",
		"trailing.",
		"weird.pdf%00",
		"../../etc/passwd",
		"",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			store := newStore(t)
			artifact := put(t, store, name, []byte("x"))

			dir, err := store.Directory(artifact.ID)
			require.NoError(t, err)

			entries, err := os.ReadDir(dir)
			require.NoError(t, err)

			var stored []string
			for _, entry := range entries {
				stored = append(stored, entry.Name())
			}

			assert.ElementsMatch(t, []string{"payload", "meta.json"}, stored)
		})
	}
}

// --- identifier validation ---

func TestReadPaths_RejectMalformedIdentifiers(t *testing.T) {
	store := newStore(t)
	malformed := []string{
		"",
		"not-a-uuid",
		"../escape",
		"..",
		"/etc/passwd",
		"a/../../b",
		"550e8400-e29b-41d4-a716-44665544000",   // one char short
		"550e8400-e29b-41d4-a716-4466554400000", // one char long
		"550E8400-E29B-41D4-A716-446655440000",  // uppercase
		"550e8400e29b41d4a716446655440000",      // separators removed
	}

	for _, id := range malformed {
		t.Run(id, func(t *testing.T) {
			_, err := store.Open(t.Context(), id)
			assert.ErrorIs(t, err, apperror.ErrInvalidInput, "Open must reject")

			_, err = store.Path(id)
			assert.ErrorIs(t, err, apperror.ErrInvalidInput, "Path must reject")

			_, err = store.Directory(id)
			assert.ErrorIs(t, err, apperror.ErrInvalidInput, "Directory must reject")

			_, err = store.Stat(id)
			assert.ErrorIs(t, err, apperror.ErrInvalidInput, "Stat must reject")

			_, err = store.Workspace(id)
			assert.ErrorIs(t, err, apperror.ErrInvalidInput, "Workspace must reject")

			assert.ErrorIs(t, store.Delete(id), apperror.ErrInvalidInput, "Delete must reject")
		})
	}
}

func TestReadPaths_WellFormedUnknownIdentifierIsNotFound(t *testing.T) {
	store := newStore(t)
	unknown := "550e8400-e29b-41d4-a716-446655440000"

	_, err := store.Stat(unknown)
	assert.ErrorIs(t, err, apperror.ErrNotFound)

	assert.ErrorIs(t, store.Delete(unknown), apperror.ErrNotFound)
}

// --- paths handed to external tools ---

func TestPath_ReturnsAReadableFileInsideTheStore(t *testing.T) {
	store := newStore(t)
	artifact := put(t, store, "book.pdf", []byte("pdf bytes"))

	path, err := store.Path(artifact.ID)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(path, store.Root()), "path must stay inside the root")
	assert.Equal(t, "payload", filepath.Base(path))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("pdf bytes"), content)
}

// --- workspace ---

func TestWorkspace_LivesInsideTheArtifactAndIsRemovedWithIt(t *testing.T) {
	store := newStore(t)
	artifact := put(t, store, "book.pdf", []byte("x"))

	dir, err := store.Directory(artifact.ID)
	require.NoError(t, err)

	workspace, err := store.Workspace(artifact.ID)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dir, "work"), workspace)
	assert.True(t, strings.HasPrefix(workspace, store.Root()))

	require.NoError(t, os.WriteFile(filepath.Join(workspace, "page-001.jpg"), []byte("render"), 0o644))

	require.NoError(t, store.Delete(artifact.ID))

	_, err = os.Stat(workspace)
	assert.True(t, os.IsNotExist(err), "the workspace must go with the artifact")
}

func TestWorkspace_IsIdempotent(t *testing.T) {
	store := newStore(t)
	artifact := put(t, store, "book.pdf", []byte("x"))

	first, err := store.Workspace(artifact.ID)
	require.NoError(t, err)

	second, err := store.Workspace(artifact.ID)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

// --- delete ---

func TestDelete_RemovesTheArtifactEntirely(t *testing.T) {
	store := newStore(t)
	artifact := put(t, store, "book.pdf", []byte("x"))

	dir, err := store.Directory(artifact.ID)
	require.NoError(t, err)

	require.NoError(t, store.Delete(artifact.ID))

	_, err = os.Stat(dir)
	assert.True(t, os.IsNotExist(err))

	_, err = store.Stat(artifact.ID)
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

func TestDelete_IsNotIdempotent(t *testing.T) {
	store := newStore(t)
	artifact := put(t, store, "book.pdf", []byte("x"))

	require.NoError(t, store.Delete(artifact.ID))

	assert.ErrorIs(t, store.Delete(artifact.ID), apperror.ErrNotFound)
}

// --- list ---

func TestList_EmptyStoreReturnsNothing(t *testing.T) {
	artifacts, err := newStore(t).List()

	require.NoError(t, err)
	assert.Empty(t, artifacts)
}

func TestList_ReturnsArtifactsOldestFirst(t *testing.T) {
	store := newStore(t)

	first := put(t, store, "one.pdf", []byte("1"))
	time.Sleep(5 * time.Millisecond)
	second := put(t, store, "two.pdf", []byte("2"))

	artifacts, err := store.List()
	require.NoError(t, err)

	require.Len(t, artifacts, 2)
	assert.Equal(t, first.ID, artifacts[0].ID)
	assert.Equal(t, second.ID, artifacts[1].ID)
}

func TestList_SkipsEntriesThatAreNotArtifacts(t *testing.T) {
	store := newStore(t)
	artifact := put(t, store, "book.pdf", []byte("x"))

	// Noise the store must tolerate rather than report.
	require.NoError(t, os.WriteFile(filepath.Join(store.Root(), "stray.txt"), []byte("x"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(store.Root(), "not-a-uuid"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(store.Root(), "550e8400-e29b-41d4-a716-446655440000"), 0o755))

	artifacts, err := store.List()
	require.NoError(t, err)

	require.Len(t, artifacts, 1, "only real artifacts must be listed")
	assert.Equal(t, artifact.ID, artifacts[0].ID)
}

// --- tampered metadata ---

func TestResolve_TamperedMetadataCannotEscapeTheArtifactDirectory(t *testing.T) {
	store := newStore(t)
	artifact := put(t, store, "book.pdf", []byte("x"))

	dir, err := store.Directory(artifact.ID)
	require.NoError(t, err)

	metaPath := filepath.Join(dir, "meta.json")
	raw, err := os.ReadFile(metaPath)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	payload["original_name"] = "../../../etc/passwd"
	payload["id"] = "../../../etc/passwd"

	tampered, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(metaPath, tampered, 0o644))

	// The payload location is not derived from metadata at all, so tampering
	// cannot redirect a read.
	path, err := store.Path(artifact.ID)
	require.NoError(t, err)

	assert.Equal(t, dir, filepath.Dir(path))
	assert.Equal(t, "payload", filepath.Base(path))
	assert.Equal(t, []byte("x"), readAll(t, store, artifact.ID))
}

func TestStat_RejectsCorruptMetadata(t *testing.T) {
	store := newStore(t)
	artifact := put(t, store, "book.pdf", []byte("x"))

	dir, err := store.Directory(artifact.ID)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "meta.json"), []byte("not json"), 0o644))

	_, err = store.Stat(artifact.ID)
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperror.ErrNotFound, "a corrupt artifact is not a missing one")
}
