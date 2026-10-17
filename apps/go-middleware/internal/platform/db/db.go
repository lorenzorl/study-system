// Package db owns SQLite connection setup and the schema migration mechanism.
//
// It knows how to open a database correctly and how to apply versioned
// migrations. It knows nothing about any context's schema: each context exports
// its own migration list, and the composition root passes it in.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// requiredPragmas are the connection settings this package enforces.
//
// They go through the DSN because `_pragma` is applied to every connection the
// driver opens. That matters: `foreign_keys` is a per-connection setting, so
// issuing it once against a pooled *sql.DB would silently configure one
// connection and leave the rest unprotected.
//
//   - foreign_keys(1): enforce the REFERENCES clauses. Without it they are
//     decorative and orphan rows insert cleanly.
//   - busy_timeout(5000): wait up to 5s for a lock instead of failing
//     immediately with SQLITE_BUSY.
//   - journal_mode(WAL): let readers proceed during a write, which the coming
//     ingestion job needs while the API serves requests.
var requiredPragmas = []string{
	"foreign_keys(1)",
	"busy_timeout(5000)",
	"journal_mode(WAL)",
}

// maxOpenConns bounds file handles while still allowing concurrent readers
// under WAL. Writes serialise on their own.
const maxOpenConns = 4

// Open opens the SQLite database named by dsn, with the required PRAGMAs
// applied to every connection.
//
// The DSN keeps its own shape: `:memory:` and `file:...` are both supported, and
// any parameters already present are preserved.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	sqlDB, err := sql.Open("sqlite", withPragmas(dsn))
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dsn, err)
	}

	if isMemory(dsn) {
		// An in-memory database is private to the connection that created it.
		// Allowing a second connection would hand out a different, empty
		// database, so the pool must stay at one.
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	} else {
		sqlDB.SetMaxOpenConns(maxOpenConns)
		sqlDB.SetMaxIdleConns(maxOpenConns)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite %q: %w", dsn, err)
	}

	return sqlDB, nil
}

// withPragmas appends the required PRAGMAs to a DSN, preserving an existing
// query string.
func withPragmas(dsn string) string {
	params := make([]string, 0, len(requiredPragmas))
	for _, pragma := range requiredPragmas {
		params = append(params, "_pragma="+pragma)
	}

	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + strings.Join(params, "&")
}

// isMemory reports whether the DSN names a private in-memory database.
func isMemory(dsn string) bool {
	return dsn == ":memory:" || strings.Contains(dsn, "mode=memory")
}
