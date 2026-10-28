// Package backup takes and proves a transactionally consistent snapshot of a
// live SQLite database.
//
// The database is the part of the local state that cannot be regenerated, and a
// plain file copy of a WAL-mode database is not a backup: the copy can open
// cleanly and be silently incomplete. Observed while writing this package: with
// three committed rows in the WAL, a copy of the .db file alone opened without
// error and did not even contain the table. SQLite's VACUUM INTO writes the
// whole database from a read transaction instead, and this package validates
// that file on a connection of its own before anything can upload it, so an
// unverified snapshot never leaves the machine (S6, S10).
//
// The package knows no schema. Take and Validate take a *sql.DB and a path, and
// Report carries what validation learned: the integrity verdict, the table list,
// and the page count. It is deliberately the caller's job to assert the tables
// it expects, because PRAGMA integrity_check answers "ok" for a brand-new empty
// database too (observed) and integrity alone is therefore not proof that the
// snapshot holds the data that mattered.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrInvalidSnapshot is the sentinel every failure to produce a valid snapshot
// wraps: a refused destination, a source that cannot be copied, a file that is
// not a usable SQLite database.
//
// A caller that needs to tell "there is no valid snapshot" from "the upload
// failed" tests errors.Is(err, ErrInvalidSnapshot), and must treat a wrapped
// error as "nothing here may be uploaded".
var ErrInvalidSnapshot = errors.New("backup: invalid snapshot")

// vacuumInto is the copy. The destination is bound as a parameter, which this
// driver supports: "VACUUM INTO ?" with the path as an argument succeeded
// through modernc.org/sqlite v1.53.0 (observed), so the path never becomes part
// of the SQL text and needs no quote doubling. Take's destination contract is
// pinned by a test whose path contains a single quote and a space.
const vacuumInto = `VACUUM INTO ?`

// Snapshot describes a snapshot file that Take produced and validated.
type Snapshot struct {
	// Path is the destination the caller supplied.
	Path string

	// Bytes is the size of the validated file.
	Bytes int64

	// CreatedAt is when the snapshot finished validating, in UTC.
	CreatedAt time.Time
}

// Report is what Validate learned about a database file. It is the evidence a
// caller asserts against: IntegrityCheck "ok" says the file is not damaged, and
// Tables and PageCount say what it contains.
type Report struct {
	// Path is the file that was validated.
	Path string

	// Bytes is its size, measured after validation.
	Bytes int64

	// IntegrityCheck is the PRAGMA integrity_check verdict. On success it is
	// "ok"; Validate returns an error for anything else.
	IntegrityCheck string

	// Tables lists the database's own tables -- sqlite_ internal tables are the
	// engine's bookkeeping, not schema a caller can ask for -- sorted by name.
	// It is empty, not nil, for a database with no tables.
	Tables []string

	// PageCount is PRAGMA page_count. A usable database has at least one page,
	// so a zero here would mean the file never got a header.
	PageCount int64
}

// Take writes a transactionally consistent snapshot of db into destPath and
// proves the result is a usable SQLite database before returning it.
//
// The copy is SQLite's VACUUM INTO, never a file copy: it reads through a read
// transaction, so a write transaction another connection has in flight is not in
// the snapshot, while committed rows that still live only in the WAL are (S6).
// VACUUM INTO only reads the source, so a read-only handle is enough (observed: a
// mode=ro connection snapshotted a database whose newest row was still in the
// WAL, and the row was in the snapshot).
//
// Destination contract: Take refuses a path that already exists instead of
// replacing it. The refusal is checked here and not delegated to SQLite, because
// VACUUM INTO rejects a non-empty destination itself ("SQL logic error: output
// file already exists", observed) but silently overwrites a zero-byte one
// (observed).
//
// Failure contract: any failure -- a refused destination, a source that cannot
// be read, a snapshot that fails validation -- returns an error wrapping
// ErrInvalidSnapshot and removes the file this call created, so a failed
// snapshot leaves nothing behind that a later step could mistake for a good one.
//
// Concurrency contract: the copy runs on a connection from db's pool, so a
// caller whose pool has a single connection must not hold a transaction while
// calling Take. It then waits for that connection and fails at the context
// deadline (observed); that is a caller bug, not a snapshot.
func Take(ctx context.Context, db *sql.DB, destPath string) (Snapshot, error) {
	if err := refuseExistingDestination(destPath); err != nil {
		return Snapshot{}, err
	}

	if _, err := db.ExecContext(ctx, vacuumInto, destPath); err != nil {
		removeSnapshotFile(destPath)
		return Snapshot{}, fmt.Errorf("%w: copy into %q: %w", ErrInvalidSnapshot, destPath, err)
	}

	// Validate what was produced, not what was intended: the copy is only a
	// snapshot once it has been opened and read back on its own connection.
	report, err := Validate(ctx, destPath)
	if err != nil {
		removeSnapshotFile(destPath)
		return Snapshot{}, err
	}

	return Snapshot{Path: destPath, Bytes: report.Bytes, CreatedAt: time.Now().UTC()}, nil
}

// Validate opens path on its own connection -- never through the source handle --
// and proves it is a usable SQLite database.
//
// It answers three questions the caller cannot assume: the file opens as a
// SQLite database, PRAGMA integrity_check finds no damage, and the schema can be
// read. The returned Report is what the caller compares against the database it
// expects, because "valid" is not the same as "this is my data".
func Validate(ctx context.Context, path string) (Report, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Report{}, fmt.Errorf("%w: inspect %q: %w", ErrInvalidSnapshot, path, err)
	}
	if info.IsDir() {
		return Report{}, fmt.Errorf("%w: %q is a directory", ErrInvalidSnapshot, path)
	}
	// Observed: a zero-byte file opens as an empty SQLite database, answers
	// integrity_check with "ok" and reports zero pages. Without this check, an
	// upload of nothing would validate.
	if info.Size() == 0 {
		return Report{}, fmt.Errorf("%w: %q is empty", ErrInvalidSnapshot, path)
	}

	conn, err := openReadOnly(path)
	if err != nil {
		return Report{}, fmt.Errorf("%w: open %q: %w", ErrInvalidSnapshot, path, err)
	}
	defer conn.Close()

	integrity, err := integrityCheck(ctx, conn)
	if err != nil {
		return Report{}, fmt.Errorf("%w: %q: %w", ErrInvalidSnapshot, path, err)
	}

	tables, err := listTables(ctx, conn)
	if err != nil {
		return Report{}, fmt.Errorf("%w: %q: %w", ErrInvalidSnapshot, path, err)
	}

	pages, err := pageCount(ctx, conn)
	if err != nil {
		return Report{}, fmt.Errorf("%w: %q: %w", ErrInvalidSnapshot, path, err)
	}

	return Report{
		Path:           path,
		Bytes:          info.Size(),
		IntegrityCheck: integrity,
		Tables:         tables,
		PageCount:      pages,
	}, nil
}

// refuseExistingDestination implements Take's destination contract: a path that
// already exists is the caller's, and Take does not replace or truncate it.
func refuseExistingDestination(path string) error {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return fmt.Errorf("%w: destination %q already exists; remove it first", ErrInvalidSnapshot, path)
	case errors.Is(err, fs.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("%w: inspect destination %q: %w", ErrInvalidSnapshot, path, err)
	}
}

// removeSnapshotFile removes a snapshot file that Take created and could not
// validate. It runs only after refuseExistingDestination proved the path was
// free, so the file cannot be a caller's. The removal error is dropped on
// purpose: the failure being reported is the one that matters, and a file this
// call could not remove will fail validation for anyone who tries to use it.
func removeSnapshotFile(path string) {
	_ = os.Remove(path)
}

// openReadOnly opens the file on a connection of its own, read-only.
//
// Two things it must not do: validate through the handle that produced the
// snapshot (that would prove nothing about the file), and write to the snapshot
// (a validation that changes the bytes about to be uploaded is not validation).
// mode=ro does both. The URI is built with url.URL rather than string
// concatenation so a path containing spaces, quotes, '#', '?' or '%' survives.
func openReadOnly(path string) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	dsn := url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=ro"}
	conn, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	return conn, nil
}

// integrityCheck runs the thorough check and returns its single verdict. A
// damaged file can fail the query itself ("database disk image is malformed",
// observed on a truncated snapshot) or answer with problem lines; both are
// validation failures.
func integrityCheck(ctx context.Context, conn *sql.DB) (string, error) {
	rows, err := conn.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return "", fmt.Errorf("integrity_check: %w", err)
	}
	defer rows.Close()

	var messages []string
	for rows.Next() {
		var message string
		if err := rows.Scan(&message); err != nil {
			return "", fmt.Errorf("integrity_check: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("integrity_check: %w", err)
	}

	switch {
	case len(messages) == 1 && messages[0] == "ok":
		return "ok", nil
	case len(messages) == 0:
		return "", errors.New("integrity_check returned no verdict")
	default:
		return "", fmt.Errorf("integrity_check reported %q", strings.Join(messages, "; "))
	}
}

// listTables lists the application's tables. Reading sqlite_schema is also the
// open-and-read probe: a file that opens but is not a database fails here.
func listTables(ctx context.Context, conn *sql.DB) ([]string, error) {
	rows, err := conn.QueryContext(ctx,
		`SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("read schema: %w", err)
	}
	defer rows.Close()

	tables := make([]string, 0, 8)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("read schema: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read schema: %w", err)
	}
	return tables, nil
}

// pageCount reports the database size in pages.
func pageCount(ctx context.Context, conn *sql.DB) (int64, error) {
	var pages int64
	if err := conn.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages); err != nil {
		return 0, fmt.Errorf("page_count: %w", err)
	}
	return pages, nil
}
