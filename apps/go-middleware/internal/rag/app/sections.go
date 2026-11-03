package app

import (
	"context"
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// SectionListRepository is the slice of the section store this read needs: every
// section of one document, already in reading order.
//
// Order belongs to the port and not to the caller. The store sorts by start page,
// level and id (sqlite.SectionRepository.ListByDocument), which is the order a
// table of contents is read in, and repeating that sort here would give one rule
// two owners that can disagree.
type SectionListRepository interface {
	// ListByDocument returns the document's sections, ascending by start page. A
	// document with no extracted structure returns an empty slice, never an error:
	// having no chapters is a normal document, not a failed read.
	ListByDocument(ctx context.Context, documentID string) ([]*domain.Section, error)
}

// Section is one entry of the answer: the chapter table as the store keeps it.
//
// It is a projection and not the domain type, because CreatedAt is deliberately
// absent. A section's row instant is when the structure stage ran, which is
// evidence about this server, not about the book; a caller citing a chapter needs
// where it starts and ends, not when the row was written.
type Section struct {
	// ID is the section's row identity, which ParentID points at.
	ID string

	// ParentID is the enclosing section, empty for a top-level one.
	ParentID string

	// Level is 1 for a part, 2 for a chapter, 3 and beyond for sections.
	Level int

	// Title is the section's heading.
	Title string

	// StartPage is the 1-based page the section begins on.
	StartPage int

	// EndPage is the 1-based page it ends on, inclusive.
	EndPage int

	// Source is how the structure stage recovered the entry: the document's own
	// outline, its printed table of contents, its typography, a numbered heading,
	// or one of those confirmed by another.
	Source domain.SectionSource
}

// DocumentSectionsUseCase answers the structure already stored for a document.
//
// It exists because the chapter table was built, validated and stored, and no
// endpoint exposed it: the only way to read the chapters of a book was to
// reassemble them from the printed table of contents, which is exactly the text
// dot leaders mangle. The stored table is the validated answer to that question.
type DocumentSectionsUseCase struct {
	documents domain.DocumentRepository
	sections  SectionListRepository
}

// NewDocumentSectionsUseCase wires the structure read.
func NewDocumentSectionsUseCase(
	documents domain.DocumentRepository,
	sections SectionListRepository,
) *DocumentSectionsUseCase {
	return &DocumentSectionsUseCase{documents: documents, sections: sections}
}

// Execute returns the document's sections in reading order.
//
// The document is looked up first, for the reason this read has two silences worth
// telling apart: an unknown document is apperror.ErrNotFound, while a document
// that exists and has no structure — because the stage has not run yet, or because
// it found nothing — is an empty list. Collapsing the first into the second would
// make a typo in a document id look like a book with no chapters.
func (uc *DocumentSectionsUseCase) Execute(
	ctx context.Context,
	documentID string,
) ([]Section, error) {
	if _, err := uc.documents.FindByID(ctx, documentID); err != nil {
		return nil, err
	}

	stored, err := uc.sections.ListByDocument(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("read the sections of document %s: %w", documentID, err)
	}

	// make with len guarantees an empty structure encodes as [] and never as null:
	// a caller ranges over this list, and "this book has no chapters" must not be a
	// case it has to special-case.
	sections := make([]Section, 0, len(stored))
	for _, section := range stored {
		sections = append(sections, Section{
			ID:        section.ID,
			ParentID:  section.ParentID,
			Level:     section.Level,
			Title:     section.Title,
			StartPage: section.StartPage,
			EndPage:   section.EndPage,
			Source:    section.Source,
		})
	}

	return sections, nil
}
