package domain

import (
	"fmt"
	"strings"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
)

// Heading is one entry of a document's printed table of contents.
//
// It is the index's own claim and nothing more: a level as the index lays it
// out, a title as the index prints it, and the page number printed beside it.
// That page is the number the reader sees, not the number of the leaf it sits
// on, and binding the two is the confirmation step's job rather than this
// value's.
type Heading struct {
	// Level is 1 for a part, 2 for a chapter, 3 and beyond for a section,
	// read from the index's own indentation and numbering.
	Level int

	// Title is the entry's heading, normalised to single spaces.
	Title string

	// Page is the page number printed beside the entry.
	Page int
}

// NewHeading validates a heading read from an index.
//
// A row that cannot be read this way is not a heading: an entry with no title
// or no page would be attribution the document never printed, and this feature
// must never invent structure.
func NewHeading(heading Heading) (Heading, error) {
	heading.Title = strings.Join(strings.Fields(heading.Title), " ")

	switch {
	case heading.Title == "":
		return Heading{}, fmt.Errorf("heading has no title: %w", apperror.ErrInvalidInput)
	case heading.Level < 1:
		return Heading{}, fmt.Errorf("heading %q has level %d, want at least 1: %w",
			heading.Title, heading.Level, apperror.ErrInvalidInput)
	case heading.Page < 1:
		return Heading{}, fmt.Errorf("heading %q names page %d, want at least 1: %w",
			heading.Title, heading.Page, apperror.ErrInvalidInput)
	}

	return heading, nil
}
