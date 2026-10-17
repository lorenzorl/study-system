package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// schemaVersionTable records which migrations have been applied.
const schemaVersionTable = `
CREATE TABLE IF NOT EXISTS schema_version (
	version     INTEGER PRIMARY KEY,
	description TEXT NOT NULL,
	applied_at  TEXT NOT NULL
);`

// Migration is one forward-only schema change.
//
// Up runs inside a transaction that also records the version, so a migration
// either applies completely and is recorded, or leaves nothing behind.
type Migration struct {
	// Version orders migrations. It must be positive and unique.
	Version int

	// Description is stored alongside the version for auditing.
	Description string

	// Up performs the change. DDL is transactional in SQLite.
	Up func(ctx context.Context, tx *sql.Tx) error
}

// Migrate applies every pending migration in ascending version order.
//
// It is idempotent: migrations at or below the recorded version are skipped, so
// calling it on every boot is safe. Adopting an existing database is supported
// as long as the first migration is written to tolerate an already-present
// schema (use CREATE TABLE IF NOT EXISTS in a baseline).
func Migrate(ctx context.Context, sqlDB *sql.DB, migrations []Migration) error {
	ordered, err := validate(migrations)
	if err != nil {
		return err
	}

	if _, err := sqlDB.ExecContext(ctx, schemaVersionTable); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	current, err := currentVersion(ctx, sqlDB)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for _, migration := range ordered {
		if migration.Version <= current {
			continue
		}
		if err := apply(ctx, sqlDB, migration); err != nil {
			return err
		}
	}

	return nil
}

// CurrentVersion reports the highest applied migration version, or 0 when the
// schema_version table is absent or empty.
func CurrentVersion(ctx context.Context, sqlDB *sql.DB) (int, error) {
	if _, err := sqlDB.ExecContext(ctx, schemaVersionTable); err != nil {
		return 0, fmt.Errorf("create schema_version: %w", err)
	}

	version, err := currentVersion(ctx, sqlDB)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

// validate sorts migrations by version and rejects duplicates and non-positive
// versions. A duplicate version is a programming error, not a runtime condition.
func validate(migrations []Migration) ([]Migration, error) {
	ordered := make([]Migration, len(migrations))
	copy(ordered, migrations)

	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Version < ordered[j].Version
	})

	seen := make(map[int]bool, len(ordered))
	for _, migration := range ordered {
		if migration.Version < 1 {
			return nil, fmt.Errorf("migration version must be positive, got %d", migration.Version)
		}
		if seen[migration.Version] {
			return nil, fmt.Errorf("duplicate migration version %d", migration.Version)
		}
		if migration.Up == nil {
			return nil, fmt.Errorf("migration %d has no Up function", migration.Version)
		}
		seen[migration.Version] = true
	}

	return ordered, nil
}

func currentVersion(ctx context.Context, sqlDB *sql.DB) (int, error) {
	var version sql.NullInt64
	err := sqlDB.QueryRowContext(ctx, `SELECT max(version) FROM schema_version`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("read current schema version: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}

// apply runs one migration and records it, in a single transaction.
func apply(ctx context.Context, sqlDB *sql.DB, migration Migration) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", migration.Version, err)
	}
	defer tx.Rollback()

	if err := migration.Up(ctx, tx); err != nil {
		return fmt.Errorf("migration %d (%s): %w", migration.Version, migration.Description, err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO schema_version (version, description, applied_at) VALUES (?, ?, ?)`,
		migration.Version, migration.Description, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("record migration %d: %w", migration.Version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", migration.Version, err)
	}
	return nil
}
