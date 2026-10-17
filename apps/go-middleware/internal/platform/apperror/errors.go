package apperror

import "errors"

// Sentinel errors for classifying failures across layers.
//
// They live in this package, and not inside a context, because the HTTP
// transport maps them to status codes without importing any single context, and
// because contexts must never depend on each other.
var (
	// ErrNotFound means the requested entity does not exist. Maps to HTTP 404.
	ErrNotFound = errors.New("not found")

	// ErrConflict means the write violates a uniqueness invariant. Maps to HTTP 409.
	ErrConflict = errors.New("conflict")

	// ErrInvalidInput means the caller supplied a value the domain rejects.
	// Maps to HTTP 400.
	ErrInvalidInput = errors.New("invalid input")

	// ErrTooLarge means the payload exceeds a configured limit. Maps to HTTP 413.
	ErrTooLarge = errors.New("too large")
)
