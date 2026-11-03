package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// fakeSectionList answers ListByDocument and records what it was asked, so a
// refused read can prove the query was never issued.
type fakeSectionList struct {
	sections []*domain.Section
	err      error
	calls    int
	gotID    string
}

func (f *fakeSectionList) ListByDocument(
	_ context.Context, documentID string,
) ([]*domain.Section, error) {
	f.calls++
	f.gotID = documentID
	if f.err != nil {
		return nil, f.err
	}
	return f.sections, nil
}

// sectionRead is one wired use case over the document and section fakes.
type sectionRead struct {
	useCase  *app.DocumentSectionsUseCase
	sections *fakeSectionList
}

// newSectionRead wires the read over a corpus holding exactly the given
// documents, so a test says which documents exist and nothing else.
func newSectionRead(sections *fakeSectionList, documentIDs ...string) sectionRead {
	documents := fakePageDocuments{}
	for _, id := range documentIDs {
		documents[id] = &domain.Document{ID: id, Title: id + ".pdf"}
	}
	return sectionRead{
		useCase:  app.NewDocumentSectionsUseCase(documents, sections),
		sections: sections,
	}
}

// TestDocumentSectionsUseCase_ReturnsEveryStoredSection pins the projection: the
// chapter table's own fields, in the order the store returned them.
//
// Order is not re-derived here. The store orders by start page, level and id
// (section_repo.go), and that contract is pinned where it lives; sorting again in
// the use case would be two owners of one rule.
func TestDocumentSectionsUseCase_ReturnsEveryStoredSection(t *testing.T) {
	read := newSectionRead(&fakeSectionList{sections: []*domain.Section{
		{
			ID: "part-1", DocumentID: "doc-1", Level: 1, Title: "Part I",
			StartPage: 3, EndPage: 120, Source: domain.SourceTOC,
		},
		{
			ID: "ch-1", DocumentID: "doc-1", ParentID: "part-1", Level: 2,
			Title: "Chapter 1", StartPage: 3, EndPage: 40, Source: domain.SourceMixed,
		},
	}}, "doc-1")

	sections, err := read.useCase.Execute(t.Context(), "doc-1")
	require.NoError(t, err)

	assert.Equal(t, "doc-1", read.sections.gotID)
	require.Len(t, sections, 2)

	assert.Equal(t, app.Section{
		ID: "part-1", ParentID: "", Level: 1, Title: "Part I",
		StartPage: 3, EndPage: 120, Source: domain.SourceTOC,
	}, sections[0])
	assert.Equal(t, app.Section{
		ID: "ch-1", ParentID: "part-1", Level: 2, Title: "Chapter 1",
		StartPage: 3, EndPage: 40, Source: domain.SourceMixed,
	}, sections[1])
}

// TestDocumentSectionsUseCase_DocumentWithNoStructureIsAnEmptyList pins the rule
// that separates the two silences: a document that exists and has no chapters is
// a normal document, not a missing one.
func TestDocumentSectionsUseCase_DocumentWithNoStructureIsAnEmptyList(t *testing.T) {
	read := newSectionRead(&fakeSectionList{sections: nil}, "doc-1")

	sections, err := read.useCase.Execute(t.Context(), "doc-1")
	require.NoError(t, err)

	require.NotNil(t, sections)
	assert.Empty(t, sections)
	assert.Equal(t, 1, read.sections.calls)
}

// TestDocumentSectionsUseCase_UnknownDocumentIsNotFound pins the other silence,
// and that it costs no section query: an id nobody registered is answered from
// the document lookup alone.
func TestDocumentSectionsUseCase_UnknownDocumentIsNotFound(t *testing.T) {
	read := newSectionRead(&fakeSectionList{})

	sections, err := read.useCase.Execute(t.Context(), "missing")

	require.ErrorIs(t, err, apperror.ErrNotFound)
	assert.Nil(t, sections)
	assert.Zero(t, read.sections.calls)
}

// TestDocumentSectionsUseCase_ReadFailureIsReported pins that a store failure is
// not flattened into an empty chapter list, which a client would read as "this
// book has no structure".
func TestDocumentSectionsUseCase_ReadFailureIsReported(t *testing.T) {
	cause := errors.New("the sections disk is gone")
	read := newSectionRead(&fakeSectionList{err: cause}, "doc-1")

	sections, err := read.useCase.Execute(t.Context(), "doc-1")

	require.ErrorIs(t, err, cause)
	assert.Nil(t, sections)
}
