// Package storage persists binary artifacts on the local filesystem.
//
// It exists so a pipeline never has to invent a path. An artifact is written
// through a generated identifier, read back through that identifier, and handed
// to an external tool through a path the store chose.
//
// The identifier is never supplied by a caller, and the original filename is
// kept as metadata only. That removes path traversal by construction rather
// than by sanitising: there is no caller-controlled input in any path.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
)

const (
	// payloadFileName is the fixed name of a stored payload.
	//
	// It is deliberately not derived from anything a caller or the metadata file
	// can influence. That makes "the payload lives at <root>/<id>/payload" a
	// trivial invariant instead of one that has to be re-validated, and it keeps
	// the store format-agnostic: a tool that needs a format hint gets it from
	// Artifact.OriginalName, not from a filename.
	payloadFileName = "payload"

	// metadataFileName holds the artifact's descriptive fields.
	metadataFileName = "meta.json"

	// workspaceDirName is the per-artifact scratch directory.
	workspaceDirName = "work"

	// incomingDirName stages writes before they are renamed into place. It lives
	// inside the root so the rename stays on one filesystem and is atomic.
	incomingDirName = "incoming"

	// copyChunkBytes bounds memory per transfer and gives the context check a
	// regular place to run.
	copyChunkBytes = 64 * 1024
)

// artifactIDPattern is the only shape a stored identifier may take. Every read
// path validates against it, so a hand-crafted identifier cannot escape the root.
var artifactIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Artifact describes a stored payload. It contains no filesystem path: callers
// that need one ask the store.
type Artifact struct {
	// ID is the generated identifier used for every subsequent operation.
	ID string `json:"id"`

	// OriginalName is the name the caller supplied, reduced to its base name. It
	// is metadata: it never becomes a path component.
	OriginalName string `json:"original_name"`

	// Size is the number of bytes stored.
	Size int64 `json:"size"`

	// StoredAt is when the artifact became visible.
	StoredAt time.Time `json:"stored_at"`
}

// Store is a filesystem-backed artifact store rooted at a single directory.
type Store struct {
	root     string
	maxBytes int64
}

// New creates a store rooted at root, rejecting any single artifact larger than
// maxBytes. The root is created if missing.
func New(root string, maxBytes int64) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("storage root must not be empty: %w", apperror.ErrInvalidInput)
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("storage maxBytes must be positive, got %d: %w", maxBytes, apperror.ErrInvalidInput)
	}

	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve storage root %q: %w", root, err)
	}

	if err := os.MkdirAll(filepath.Join(absolute, incomingDirName), 0o755); err != nil {
		return nil, fmt.Errorf("create storage root %q: %w", absolute, err)
	}

	return &Store{root: absolute, maxBytes: maxBytes}, nil
}

// Root is the absolute directory the store owns.
func (s *Store) Root() string { return s.root }

// MaxBytes is the largest artifact this store accepts.
func (s *Store) MaxBytes() int64 { return s.maxBytes }

// Put streams content into the store and returns the resulting artifact.
//
// The transfer is staged and renamed into place, so an interrupted or oversized
// transfer leaves nothing behind. Exceeding maxBytes returns apperror.ErrTooLarge
// once the limit is passed, without reading the remainder.
func (s *Store) Put(ctx context.Context, originalName string, content io.Reader) (Artifact, error) {
	if content == nil {
		return Artifact{}, fmt.Errorf("content reader must not be nil: %w", apperror.ErrInvalidInput)
	}

	id := uuid.New().String()
	staging := filepath.Join(s.root, incomingDirName, id)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return Artifact{}, fmt.Errorf("create staging directory: %w", err)
	}
	// Removing the staging directory is a no-op once the rename has happened,
	// and cleans up every failure path otherwise.
	defer os.RemoveAll(staging)

	written, err := writeStream(ctx, filepath.Join(staging, payloadFileName), content, s.maxBytes)
	if err != nil {
		return Artifact{}, err
	}

	artifact := Artifact{
		ID:           id,
		OriginalName: filepath.Base(originalName),
		Size:         written,
		StoredAt:     time.Now().UTC(),
	}

	if err := writeMetadata(filepath.Join(staging, metadataFileName), artifact); err != nil {
		return Artifact{}, err
	}

	if err := os.Rename(staging, filepath.Join(s.root, id)); err != nil {
		return Artifact{}, fmt.Errorf("commit artifact: %w", err)
	}

	return artifact, nil
}

// Open returns a reader over a stored artifact. The caller closes it.
func (s *Store) Open(ctx context.Context, id string) (io.ReadCloser, error) {
	path, err := s.Path(id)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("artifact %q payload missing: %w", id, apperror.ErrNotFound)
		}
		return nil, fmt.Errorf("open artifact %q: %w", id, err)
	}
	return file, nil
}

// Path returns the filesystem path of a stored artifact's payload, for handing
// to an external tool that operates on files rather than readers.
func (s *Store) Path(id string) (string, error) {
	dir, err := s.Directory(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, payloadFileName), nil
}

// Directory returns the directory holding an artifact, which is where its
// workspace lives.
func (s *Store) Directory(id string) (string, error) {
	if !artifactIDPattern.MatchString(id) {
		// A malformed identifier is bad input, not a missing artifact.
		return "", fmt.Errorf("malformed artifact id: %w", apperror.ErrInvalidInput)
	}

	dir := filepath.Join(s.root, id)

	// Defence in depth: the identifier pattern already forbids separators, so
	// this cannot trigger today. It guards the invariant if the pattern is ever
	// loosened.
	if !withinRoot(s.root, dir) {
		return "", fmt.Errorf("artifact %q resolves outside the storage root: %w", id, apperror.ErrInvalidInput)
	}

	if _, err := os.Stat(filepath.Join(dir, metadataFileName)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("artifact %q: %w", id, apperror.ErrNotFound)
		}
		return "", fmt.Errorf("inspect artifact %q: %w", id, err)
	}

	return dir, nil
}

// Stat returns an artifact's metadata.
func (s *Store) Stat(id string) (Artifact, error) {
	dir, err := s.Directory(id)
	if err != nil {
		return Artifact{}, err
	}

	raw, err := os.ReadFile(filepath.Join(dir, metadataFileName))
	if err != nil {
		return Artifact{}, fmt.Errorf("read metadata for artifact %q: %w", id, err)
	}

	var artifact Artifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return Artifact{}, fmt.Errorf("parse metadata for artifact %q: %w", id, err)
	}

	return artifact, nil
}

// Workspace returns a per-artifact scratch directory, creating it on first use.
// It is where intermediate files such as page renders belong, so that removing
// the artifact removes its intermediates too.
func (s *Store) Workspace(id string) (string, error) {
	dir, err := s.Directory(id)
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, workspaceDirName)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", fmt.Errorf("create workspace for artifact %q: %w", id, err)
	}
	return path, nil
}

// Delete removes an artifact and everything under it, including its workspace.
// Deleting an absent artifact returns apperror.ErrNotFound.
func (s *Store) Delete(id string) error {
	dir, err := s.Directory(id)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete artifact %q: %w", id, err)
	}
	return nil
}

// List returns every artifact in the store, oldest first. Entries that are not
// valid artifacts are skipped rather than reported: the store owns its root and
// anything unrecognised in it is not an artifact.
func (s *Store) List() ([]Artifact, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("list storage root: %w", err)
	}

	artifacts := make([]Artifact, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !artifactIDPattern.MatchString(entry.Name()) {
			continue
		}

		artifact, err := s.Stat(entry.Name())
		if err != nil {
			// A directory that cannot describe itself is not a usable artifact.
			continue
		}
		artifacts = append(artifacts, artifact)
	}

	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].StoredAt.Before(artifacts[j].StoredAt)
	})

	return artifacts, nil
}

// writeStream copies content to path with a hard byte ceiling.
//
// The limit is enforced by copying at most maxBytes+1 bytes: reaching that count
// proves the source is over the limit, without reading the rest of it.
func writeStream(ctx context.Context, path string, content io.Reader, maxBytes int64) (int64, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("create %q: %w", filepath.Base(path), err)
	}
	defer file.Close()

	buffer := make([]byte, copyChunkBytes)
	var written int64

	for {
		if err := ctx.Err(); err != nil {
			return written, fmt.Errorf("write artifact: %w", err)
		}

		// Read one byte past the ceiling so overflow is detectable.
		remaining := min(maxBytes-written+1, int64(len(buffer)))

		read, readErr := content.Read(buffer[:remaining])
		if read > 0 {
			if _, err := file.Write(buffer[:read]); err != nil {
				return written, fmt.Errorf("write artifact: %w", err)
			}
			written += int64(read)
			if written > maxBytes {
				return written, fmt.Errorf("artifact exceeds %d bytes: %w", maxBytes, apperror.ErrTooLarge)
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return written, fmt.Errorf("read artifact source: %w", readErr)
		}
	}

	if err := file.Sync(); err != nil {
		return written, fmt.Errorf("sync artifact: %w", err)
	}
	return written, nil
}

func writeMetadata(path string, artifact Artifact) error {
	raw, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}

// withinRoot reports whether path is inside root.
func withinRoot(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
