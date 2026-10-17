package sqlite

import "strings"

// The modernc.org/sqlite driver reports constraint violations as message text
// rather than as typed errors, so these helpers match on that text. The study
// context's adapter keeps its own copy of the UNIQUE check for the same reason a
// context never imports another context; consolidating both into platform/db is
// worth doing when a third adapter needs them (see the backlog).

// isUniqueConstraintError reports a UNIQUE constraint violation, which includes
// a duplicate primary key in SQLite.
func isUniqueConstraintError(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// isForeignKeyConstraintError reports a violated foreign key. It is mapped to
// apperror.ErrNotFound rather than surfaced raw, because the caller's real
// mistake is naming a row that does not exist: an outcome for an unknown job, or
// a job for an unknown document. The raw message names a constraint, which tells
// a caller nothing it can act on.
func isForeignKeyConstraintError(err error) bool {
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
