// Package composition turns configuration and a database into the application
// objects a process runs.
//
// It is the one place that knows every bounded context, and the one place that
// decides how a context is wired. Before it existed, each command repeated that
// wiring inline, so a third entrypoint would have been a third copy of the same
// migration list, the same open/migrate rules, and the same lexical-versus-hybrid
// search decision.
//
// One rule keeps the package honest: **composition builds, the entrypoint
// mounts.** Nothing here registers a route, parses a flag, or starts a server.
// Each entrypoint keeps its own transport, its own policy, and its own shutdown.
// That is why there is no Router here: the RAG router needs the upload handler,
// which needs the store and "is a key configured" — entrypoint policy, not
// wiring.
//
// The dependency direction is outward only. This package imports every context;
// no context imports it.
package composition

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
	studysqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// Migrations returns every context's migrations in one list.
//
// This is the single definition of what this database's schema is. It matters
// more than it looks: platform/db keeps ONE schema_version table for the whole
// file and treats every migration at or below max(version) as applied, so a
// caller that passes only one context's list does not fail — it silently skips
// the other context's earlier versions. Each context therefore owns a disjoint
// version block (study 1-2, rag 100-199), and this function is what puts both
// blocks in one run.
func Migrations() []db.Migration {
	return slices.Concat(studysqlite.Migrations(), ragsqlite.Migrations())
}

// OpenDatabase opens the database read-write and applies every pending
// migration. It is what a process that may write uses, and it is safe to call on
// every boot: migrations at or below the recorded version are skipped.
//
// The error messages are the ones the commands printed before this package
// existed, so a failure reads the same from either binary.
func OpenDatabase(ctx context.Context, cfg *config.Config) (*sql.DB, error) {
	sqlDB, err := db.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Migrate(ctx, sqlDB, Migrations()); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return sqlDB, nil
}

// OpenReadOnlyDatabase opens an existing database without applying migrations.
//
// It is what a read-only command uses: search reads the index and must not be
// the thing that upgrades a schema, and it must report a missing file instead of
// creating one. An in-memory DSN is returned unchanged, because private memory
// has no "read-only" mode to ask for.
func OpenReadOnlyDatabase(ctx context.Context, cfg *config.Config) (*sql.DB, error) {
	return db.Open(ctx, readOnlyDSN(cfg.DatabaseDSN))
}

// OpenPlanningDatabase opens the database a dry run is allowed to see: the real
// file when it exists, read-only, and otherwise an empty in-memory schema.
//
// The fallback is the point. A dry run must not write, and it must still answer
// the planning questions; an empty migrated schema answers every lookup with "not
// found", exactly as a database that has never seen those bytes does, and writes
// nothing to disk.
func OpenPlanningDatabase(ctx context.Context, cfg *config.Config) (*sql.DB, error) {
	if sqlDB, err := db.Open(ctx, readOnlyDSN(cfg.DatabaseDSN)); err == nil {
		return sqlDB, nil
	}

	sqlDB, err := db.Open(ctx, ":memory:")
	if err != nil {
		return nil, fmt.Errorf("open planning database: %w", err)
	}

	if err := db.Migrate(ctx, sqlDB, Migrations()); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("prepare planning database: %w", err)
	}

	return sqlDB, nil
}

// readOnlyDSN appends SQLite's read-only mode to a file DSN, preserving any
// query string it already carries. An in-memory DSN is returned unchanged.
func readOnlyDSN(dsn string) string {
	if strings.Contains(dsn, ":memory:") {
		return dsn
	}
	if strings.Contains(dsn, "?") {
		return dsn + "&mode=ro"
	}
	return dsn + "?mode=ro"
}
