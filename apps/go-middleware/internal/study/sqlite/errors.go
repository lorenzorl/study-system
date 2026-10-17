package sqlite

import "strings"

// isUniqueConstraintError returns true if err indicates a SQLite UNIQUE constraint violation.
// modernc.org/sqlite surfaces this as a string-based error message.
func isUniqueConstraintError(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
