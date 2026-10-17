package sqlite

import (
	"database/sql"
	"fmt"
	"time"
)

// Stored instants are UTC, RFC3339, whole seconds, which is the precision
// domain.Now produces. Every timestamp in this context is written and read
// through these helpers so no call site can decide its own format.

// formatInstant encodes an instant for storage.
func formatInstant(instant time.Time) string {
	return instant.UTC().Format(time.RFC3339)
}

// nullableInstant encodes an optional instant, as NULL when absent.
func nullableInstant(instant *time.Time) any {
	if instant == nil {
		return nil
	}
	return formatInstant(*instant)
}

// parseInstant decodes a stored instant.
//
// The error is returned rather than discarded: a timestamp that fails to parse
// would otherwise become the zero time, which sorts before every real instant —
// and "the most recent job for this document" would then quietly return the wrong
// job. Corruption must be loud.
func parseInstant(value string) (time.Time, error) {
	instant, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse instant %q: %w", value, err)
	}
	return instant.UTC(), nil
}

// parseNullableInstant decodes an optional instant, as nil when NULL.
func parseNullableInstant(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	instant, err := parseInstant(value.String)
	if err != nil {
		return nil, err
	}
	return &instant, nil
}
