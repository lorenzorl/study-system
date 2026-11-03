package domain

import "time"

// ReadingState is whether the reader has read one page range.
//
// Two states, and nothing else: `unread` and `read`. A third ("reading",
// "skimmed") would be a claim nobody can check and a value every consumer would
// have to interpret, while the plan asks one question — did this block get done —
// and these two answer it.
//
// The stored column is TEXT with no CHECK, following ingestion_jobs.status and
// range_gaps.state, so a vocabulary that grows does not need a migration for
// every new word.
type ReadingState string

const (
	// ReadingStateUnread is a range the reader has not read, stated explicitly.
	// It is stored, and not represented by the absence of a row, so "not read" and
	// "never looked at" stay different answers.
	ReadingStateUnread ReadingState = "unread"

	// ReadingStateRead is a range the reader says they read. It is a claim, not a
	// proof: nothing here verifies it, and nothing should.
	ReadingStateRead ReadingState = "read"
)

// ParseReadingState maps a wire value to a state, and reports whether it is one
// this application knows.
func ParseReadingState(value string) (ReadingState, bool) {
	switch ReadingState(value) {
	case ReadingStateUnread:
		return ReadingStateUnread, true
	case ReadingStateRead:
		return ReadingStateRead, true
	}

	return "", false
}

// ReadingProgress is one page range of one document, and whether it was read.
//
// The unit is the range the plan recommends, not the page: after two pages of a
// thirteen-page chapter a page counter says 2/13 and the chapter is neither read
// nor not read, while a range with two states answers the only question the plan
// asks. It is keyed to pages and not to document_sections ids because sections are
// derived data rebuilt on reprocess, and it declares no foreign key to documents
// because the two contexts share a database file, not a schema.
type ReadingProgress struct {
	ID         string
	DocumentID string

	// FromPage and ToPage are 1-based and inclusive, as the plan names them.
	FromPage int
	ToPage   int

	State ReadingState

	// CreatedAt belongs to the first marking of the range and survives every
	// later one; UpdatedAt moves with each write.
	CreatedAt time.Time
	UpdatedAt time.Time
}
