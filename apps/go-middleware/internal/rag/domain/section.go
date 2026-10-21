package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
)

// SectionSource names how a section was recovered, so a stored row can always
// say where it came from.
//
// The vocabulary is validated here and not by a CHECK constraint, for the same
// reason ingestion_jobs.status is not constrained: it may grow, and SQLite would
// require rebuilding the table to widen a CHECK.
type SectionSource string

const (
	// SourceOutline means the document's own bookmarks supplied the entry.
	SourceOutline SectionSource = "outline"

	// SourceTOC means the printed table of contents supplied the entry.
	SourceTOC SectionSource = "toc"

	// SourceTypography means the page's own typography supplied the entry,
	// without a table of contents to confirm it.
	SourceTypography SectionSource = "typography"

	// SourcePattern means a numbered heading in the text supplied the entry,
	// which is what a document with no table of contents offers.
	SourcePattern SectionSource = "pattern"

	// SourceMixed means the entry was read from one source and confirmed by
	// another: the state every confident section is in.
	SourceMixed SectionSource = "mixed"
)

// valid reports whether the source is one this system can produce.
func (s SectionSource) valid() bool {
	switch s {
	case SourceOutline, SourceTOC, SourceTypography, SourcePattern, SourceMixed:
		return true
	default:
		return false
	}
}

// Section is one node of a document's structure — a part, a chapter or a
// section — together with the page range it owns.
//
// The range is what makes the row useful: a chunk knows its page, so a section
// that owns a range can answer for it without reading a byte of the text. A
// section's range never crosses another section that starts inside it, and it
// says nothing about parent/child page containment, which the extractor is
// responsible for.
type Section struct {
	// ID is the row's identity. The caller supplies it because ParentID has to
	// name it before anything is written.
	ID string

	// DocumentID is the document this section belongs to.
	DocumentID string

	// ParentID is the enclosing section, empty for a top-level one.
	ParentID string

	// Level is 1 for a part, 2 for a chapter, 3 and beyond for sections. The
	// numbering is the document's own nesting, not a fixed vocabulary: a paper
	// with no parts starts its first section at 1.
	Level int

	// Title is the section's heading, normalised to single spaces.
	Title string

	// StartPage is the 1-based page the section begins on.
	StartPage int

	// EndPage is the 1-based page it ends on, inclusive. A section that owns one
	// page has StartPage == EndPage.
	EndPage int

	// Source is how the section was recovered.
	Source SectionSource

	// CreatedAt is when the row was produced.
	CreatedAt time.Time
}

// NewSection validates a section and stamps it.
//
// The instance is not a place to put a guess: an impossible range or an unknown
// provenance is rejected here rather than stored, because a wrong section hides
// content with the appearance of authority.
func NewSection(section Section, now time.Time) (*Section, error) {
	section.ID = strings.TrimSpace(section.ID)
	section.DocumentID = strings.TrimSpace(section.DocumentID)
	section.ParentID = strings.TrimSpace(section.ParentID)
	section.Title = strings.Join(strings.Fields(section.Title), " ")

	switch {
	case section.ID == "":
		return nil, fmt.Errorf("section id is required: %w", apperror.ErrInvalidInput)
	case section.DocumentID == "":
		return nil, fmt.Errorf("section %s has no document: %w", section.ID, apperror.ErrInvalidInput)
	case section.Title == "":
		return nil, fmt.Errorf("section %s has no title: %w", section.ID, apperror.ErrInvalidInput)
	case section.Level < 1:
		return nil, fmt.Errorf("section %s has level %d, want at least 1: %w",
			section.ID, section.Level, apperror.ErrInvalidInput)
	case section.StartPage < 1:
		return nil, fmt.Errorf("section %s starts on page %d, want at least 1: %w",
			section.ID, section.StartPage, apperror.ErrInvalidInput)
	case section.EndPage < section.StartPage:
		return nil, fmt.Errorf("section %s ends on page %d before it starts on %d: %w",
			section.ID, section.EndPage, section.StartPage, apperror.ErrInvalidInput)
	case section.ParentID == section.ID:
		return nil, fmt.Errorf("section %s cannot be its own parent: %w", section.ID, apperror.ErrInvalidInput)
	case !section.Source.valid():
		return nil, fmt.Errorf("section %s has source %q: %w",
			section.ID, section.Source, apperror.ErrInvalidInput)
	}

	section.CreatedAt = now.UTC()
	return &section, nil
}
