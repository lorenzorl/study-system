package sqlite_test

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// sectionsRows counts stored sections directly, so an assertion about "the same
// rows" is about stored rows and not about what a query chose to return.
func sectionsRows(t *testing.T, sqlDB *sql.DB, documentID string) int {
	t.Helper()

	var count int
	require.NoError(t, sqlDB.QueryRow(
		`SELECT count(*) FROM document_sections WHERE document_id = ?`, documentID,
	).Scan(&count))
	return count
}

func mustSection(t *testing.T, draft domain.Section) *domain.Section {
	t.Helper()

	section, err := domain.NewSection(draft, instant)
	require.NoError(t, err)
	return section
}

// chapterTree is a part, a chapter under it and a section under that, at the
// pages a real book uses: the section starts inside the chapter, and the chapter
// inside the part.
func chapterTree(t *testing.T, documentID string) []*domain.Section {
	t.Helper()

	part := mustSection(t, domain.Section{
		ID:         uuid.NewString(),
		DocumentID: documentID,
		Level:      1,
		Title:      "Part I. Putting the Domain Model to Work",
		StartPage:  12,
		EndPage:    48,
		Source:     domain.SourceTOC,
	})
	chapter := mustSection(t, domain.Section{
		ID:         uuid.NewString(),
		DocumentID: documentID,
		ParentID:   part.ID,
		Level:      2,
		Title:      "Crunching Knowledge",
		StartPage:  15,
		EndPage:    23,
		Source:     domain.SourceTOC,
	})
	section := mustSection(t, domain.Section{
		ID:         uuid.NewString(),
		DocumentID: documentID,
		ParentID:   chapter.ID,
		Level:      3,
		Title:      "UBIQUITOUS LANGUAGE",
		StartPage:  20,
		EndPage:    23,
		Source:     domain.SourceMixed,
	})

	return []*domain.Section{part, chapter, section}
}

func sectionTitles(sections []*domain.Section) []string {
	titles := make([]string, 0, len(sections))
	for _, section := range sections {
		titles = append(titles, section.Title)
	}
	return titles
}

func TestSectionRepository_ReplaceStoresTheTreeAndListsItByPage(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()
	document := seedDocument(t, ctx, repos.documents, "ddd")

	require.NoError(t, repos.sections.ReplaceDocumentSections(ctx, document.ID, chapterTree(t, document.ID)))

	assert.Equal(t, 3, sectionsRows(t, sqlDB, document.ID))

	stored, err := repos.sections.ListByDocument(ctx, document.ID)
	require.NoError(t, err)
	require.Len(t, stored, 3)
	assert.Equal(t, []string{
		"Part I. Putting the Domain Model to Work",
		"Crunching Knowledge",
		"UBIQUITOUS LANGUAGE",
	}, sectionTitles(stored), "reading order is page order")

	chapter := stored[1]
	assert.Equal(t, 2, chapter.Level)
	assert.Equal(t, 15, chapter.StartPage)
	assert.Equal(t, 23, chapter.EndPage)
	assert.Equal(t, domain.SourceTOC, chapter.Source)
	assert.Equal(t, stored[0].ID, chapter.ParentID, "the parent link must survive the round trip")
	assert.True(t, instant.Equal(chapter.CreatedAt), "created_at must survive the round trip")

	section := stored[2]
	assert.Equal(t, domain.SourceMixed, section.Source)
	assert.Equal(t, chapter.ID, section.ParentID)

	assert.Empty(t, stored[0].ParentID, "a top-level section has no parent")
}

func TestSectionRepository_ReplaceIsReplaceAll(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()
	document := seedDocument(t, ctx, repos.documents, "ddd")

	require.NoError(t, repos.sections.ReplaceDocumentSections(ctx, document.ID, chapterTree(t, document.ID)))

	replacement := mustSection(t, domain.Section{
		ID:         uuid.NewString(),
		DocumentID: document.ID,
		Level:      2,
		Title:      "Supple Design",
		StartPage:  170,
		EndPage:    190,
		Source:     domain.SourceTOC,
	})
	require.NoError(t, repos.sections.ReplaceDocumentSections(ctx, document.ID, []*domain.Section{replacement}))

	assert.Equal(t, 1, sectionsRows(t, sqlDB, document.ID), "a replace leaves the previous tree behind")

	stored, err := repos.sections.ListByDocument(ctx, document.ID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, "Supple Design", stored[0].Title)
}

func TestSectionRepository_ReplaceWithNoSectionsClearsTheDocument(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()
	document := seedDocument(t, ctx, repos.documents, "ddd")
	require.NoError(t, repos.sections.ReplaceDocumentSections(ctx, document.ID, chapterTree(t, document.ID)))

	// The document that could not be structured: its sections are deleted, not
	// left behind from an earlier attempt.
	require.NoError(t, repos.sections.ReplaceDocumentSections(ctx, document.ID, nil))

	assert.Equal(t, 0, sectionsRows(t, sqlDB, document.ID))

	stored, err := repos.sections.ListByDocument(ctx, document.ID)
	require.NoError(t, err)
	assert.Empty(t, stored)
	assert.NotNil(t, stored, "an empty list, never nil: callers range over it")
}

func TestSectionRepository_ReplaceAcceptsChildrenBeforeTheirParents(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()
	document := seedDocument(t, ctx, repos.documents, "ddd")

	tree := chapterTree(t, document.ID)
	reversed := []*domain.Section{tree[2], tree[1], tree[0]}

	// Insertion order is a storage concern: the foreign key would reject a child
	// written before its parent, so the adapter orders by level and the caller
	// does not have to.
	require.NoError(t, repos.sections.ReplaceDocumentSections(ctx, document.ID, reversed))
	assert.Equal(t, 3, sectionsRows(t, sqlDB, document.ID))
}

func TestSectionRepository_ReplaceForAnUnknownDocumentIsNotFound(t *testing.T) {
	_, repos := openRepos(t)

	err := repos.sections.ReplaceDocumentSections(t.Context(), uuid.NewString(),
		chapterTree(t, uuid.NewString()))
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

func TestSectionRepository_ReplaceWithNoSectionsForAnUnknownDocumentIsNotFound(t *testing.T) {
	_, repos := openRepos(t)

	// The empty replace only deletes rows, so nothing trips the foreign key. Without
	// an explicit lookup the missing document would look like a successful no-op.
	err := repos.sections.ReplaceDocumentSections(t.Context(), uuid.NewString(), nil)
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

func TestSectionRepository_ListByDocumentOfAnUnknownDocumentIsEmpty(t *testing.T) {
	_, repos := openRepos(t)

	stored, err := repos.sections.ListByDocument(t.Context(), uuid.NewString())
	require.NoError(t, err)
	assert.Empty(t, stored)
	assert.NotNil(t, stored)
}

func TestSectionRepository_SectionAtReturnsTheDeepestSectionOwningThePage(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()
	document := seedDocument(t, ctx, repos.documents, "ddd")
	tree := chapterTree(t, document.ID)
	require.NoError(t, repos.sections.ReplaceDocumentSections(ctx, document.ID, tree))

	tests := []struct {
		name  string
		page  int
		level int
		title string
	}{
		{"inside the section", 21, 3, "UBIQUITOUS LANGUAGE"},
		{"inside the chapter only", 16, 2, "Crunching Knowledge"},
		{"inside the part only", 13, 1, "Part I. Putting the Domain Model to Work"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			found, err := repos.sections.SectionAt(ctx, document.ID, test.page)
			require.NoError(t, err)
			assert.Equal(t, test.level, found.Level)
			assert.Equal(t, test.title, found.Title)
		})
	}
}

func TestSectionRepository_SectionAtStopsAtTheEndOfTheRange(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()
	document := seedDocument(t, ctx, repos.documents, "ddd")
	tree := chapterTree(t, document.ID)

	// A sibling chapter that starts exactly where the previous one ends, which is
	// how a book is paginated: one section's last page and the next one's first
	// page are consecutive.
	next := mustSection(t, domain.Section{
		ID:         uuid.NewString(),
		DocumentID: document.ID,
		ParentID:   tree[0].ID,
		Level:      2,
		Title:      "Communication and the Use of Language",
		StartPage:  24,
		EndPage:    36,
		Source:     domain.SourceTOC,
	})
	require.NoError(t, repos.sections.ReplaceDocumentSections(ctx, document.ID, append(tree, next)))

	found, err := repos.sections.SectionAt(ctx, document.ID, 24)
	require.NoError(t, err)
	assert.Equal(t, "Communication and the Use of Language", found.Title,
		"the boundary page belongs to the section that starts there, not to the one that ended")

	// A page inside the part but outside every chapter is the part's, and a page
	// inside nothing is the null attribution rather than an error-free guess.
	found, err = repos.sections.SectionAt(ctx, document.ID, 13)
	require.NoError(t, err)
	assert.Equal(t, 1, found.Level)

	_, err = repos.sections.SectionAt(ctx, document.ID, 400)
	assert.ErrorIs(t, err, apperror.ErrNotFound)
}

func TestSectionRepository_SectionAtIsScopedToOneDocument(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()
	ddd := seedDocument(t, ctx, repos.documents, "ddd")
	other := seedDocument(t, ctx, repos.documents, "other")
	require.NoError(t, repos.sections.ReplaceDocumentSections(ctx, ddd.ID, chapterTree(t, ddd.ID)))

	_, err := repos.sections.SectionAt(ctx, other.ID, 21)
	assert.ErrorIs(t, err, apperror.ErrNotFound, "one book's sections must never answer for another")
}

func TestSectionRepository_SectionAtRejectsAPageBelowOne(t *testing.T) {
	_, repos := openRepos(t)
	ctx := t.Context()
	document := seedDocument(t, ctx, repos.documents, "ddd")

	_, err := repos.sections.SectionAt(ctx, document.ID, 0)
	assert.ErrorIs(t, err, apperror.ErrInvalidInput)
}

// Reindexing rebuilds a document's chunks. The sections are a different table and
// must survive that: a citation is resolved by page at query time, so a reindex
// that swept them away would silently downgrade every result to a bare page
// number. This guards the foreign keys from a future cascade.
func TestSectionRepository_ChunksBeingReplacedLeavesTheSectionsAlone(t *testing.T) {
	sqlDB, repos := openRepos(t)
	ctx := t.Context()
	document := seedDocument(t, ctx, repos.documents, "ddd")
	tree := chapterTree(t, document.ID)
	require.NoError(t, repos.sections.ReplaceDocumentSections(ctx, document.ID, tree))

	job := seedJob(t, ctx, repos, document.ID, 24, instant)
	require.NoError(t, repos.chunks.ReplaceDocumentChunks(ctx, document.ID, job.ID,
		[]domain.Chunk{{PageNumber: 21, Ordinal: 0, Text: "el obstáculo del diseño"}}, instant))
	// A reindex replaces again; the second write is what a later run does.
	require.NoError(t, repos.chunks.ReplaceDocumentChunks(ctx, document.ID, job.ID,
		[]domain.Chunk{{PageNumber: 21, Ordinal: 0, Text: "el obstáculo, reformulado"}}, instant))

	assert.Equal(t, 3, sectionsRows(t, sqlDB, document.ID), "reindexing must not touch the sections")

	found, err := repos.sections.SectionAt(ctx, document.ID, 21)
	require.NoError(t, err)
	assert.Equal(t, "UBIQUITOUS LANGUAGE", found.Title, "the citation still resolves after a reindex")
}
