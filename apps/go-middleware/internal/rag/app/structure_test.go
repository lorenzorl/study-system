package app_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// structureInstant is the fixed instant every section is stamped with, so the
// test reads the same value the use case would.
var structureInstant = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)

// printed is one entry of the printed index: the number beside it is a printed
// page.
func printed(level int, title string, page int) domain.Heading {
	return domain.Heading{Level: level, Title: title, Page: page}
}

// set is the same title as the page sets it: the page number is a PDF page.
func set(title string, page int) domain.Heading {
	return domain.Heading{Level: 1, Title: title, Page: page}
}

// buildSections runs the assembly over one document.
func buildSections(input app.StructureInput) ([]*domain.Section, app.StructureReport) {
	return app.BuildSections(input, structureInstant)
}

// byTitle indexes the sections so a test can ask about a section by its title
// instead of by a position the ordering rules decided.
func byTitle(sections []*domain.Section) map[string]*domain.Section {
	indexed := map[string]*domain.Section{}
	for _, section := range sections {
		indexed[section.Title] = section
	}
	return indexed
}

// dddTOC is the real book's shape: a part, its chapters, the sections inside a
// chapter, a closing section, and a second part.
func dddTOC() []domain.Heading {
	return []domain.Heading{
		printed(1, "Part I. Putting the Domain Model to Work", 3),
		printed(2, "Crunching Knowledge", 15),
		printed(3, "Listen to Language", 15),
		printed(3, "Scrutinize Awkwardness", 25),
		printed(2, "Communication and the Use of Language", 30),
		printed(3, "Summary", 40),
		printed(1, "Part II. Building of a Model-Driven", 41),
	}
}

// dddTypography is what the same pages set large, in PDF pages. The book prints
// its own page numbers, so the two agree at offset 0.
func dddTypography() []domain.Heading {
	return []domain.Heading{
		set("Part I. Putting the Domain Model to Work", 3),
		set("Crunching Knowledge", 15),
		set("Listen to Language", 15),
		set("Scrutinize Awkwardness", 25),
		set("Communication and the Use of Language", 30),
		set("Summary", 40),
		set("Part II. Building of a Model-Driven", 41),
	}
}

func TestBuildSections_BuildsTheRealBooksShape(t *testing.T) {
	sections, report := buildSections(app.StructureInput{
		DocumentID:  "document-1",
		PageCount:   359,
		TOC:         dddTOC(),
		Typographic: dddTypography(),
	})

	assert.Equal(t, 0, report.Offset, "the book numbers its own pages, so the offset is zero")
	assert.Equal(t, 2, report.ChaptersNamed)
	assert.Equal(t, 2, report.ChaptersConfirmed)
	assert.True(t, report.GatePassed)
	assert.Empty(t, report.Reason, "a document that produced sections needs no explanation")
	assert.Equal(t, 0, report.EntriesDropped)

	// Reading order: start page, then level. A section on the same page as its
	// chapter follows it.
	require.Len(t, sections, 7)
	assert.Equal(t, []string{
		"Part I. Putting the Domain Model to Work",
		"Crunching Knowledge",
		"Listen to Language",
		"Scrutinize Awkwardness",
		"Communication and the Use of Language",
		"Summary",
		"Part II. Building of a Model-Driven",
	}, titles(sections))

	indexed := byTitle(sections)
	assertRange(t, indexed["Part I. Putting the Domain Model to Work"], 3, 40, 1)
	assertRange(t, indexed["Crunching Knowledge"], 15, 29, 2)
	assertRange(t, indexed["Listen to Language"], 15, 24, 3)
	assertRange(t, indexed["Scrutinize Awkwardness"], 25, 29, 3)
	assertRange(t, indexed["Communication and the Use of Language"], 30, 40, 2)
	// The last section of a chapter never runs past its parent.
	assertRange(t, indexed["Summary"], 40, 40, 3)
	assertRange(t, indexed["Part II. Building of a Model-Driven"], 41, 359, 1)

	assert.Equal(t, "", indexed["Part I. Putting the Domain Model to Work"].ParentID,
		"a part has no parent")
	assert.Equal(t, indexed["Part I. Putting the Domain Model to Work"].ID,
		indexed["Crunching Knowledge"].ParentID)
	assert.Equal(t, indexed["Crunching Knowledge"].ID,
		indexed["Listen to Language"].ParentID)
	assert.Equal(t, indexed["Crunching Knowledge"].ID,
		indexed["Scrutinize Awkwardness"].ParentID)
	assert.Equal(t, indexed["Part I. Putting the Domain Model to Work"].ID,
		indexed["Communication and the Use of Language"].ParentID)
	assert.Equal(t, indexed["Communication and the Use of Language"].ID,
		indexed["Summary"].ParentID)

	identities := map[string]bool{}
	for _, section := range sections {
		assert.Regexp(t, `\A[0-9a-f]{8}-`, section.ID, "identity is a minted uuid")
		assert.False(t, identities[section.ID], "two sections share an identity")
		identities[section.ID] = true

		assert.Equal(t, "document-1", section.DocumentID)
		assert.Equal(t, structureInstant, section.CreatedAt)
		assert.Equal(t, domain.SourceMixed, section.Source,
			"a section the page's typography confirms is mixed")
	}
}

func TestBuildSections_RecoversTheOffsetOfRomanFrontMatter(t *testing.T) {
	// The front matter is numbered in roman numerals, so the print restarts its
	// arabic numbering five leaves into the PDF. Nothing in the index says so.
	sections, report := buildSections(app.StructureInput{
		DocumentID: "document-2",
		PageCount:  210,
		TOC: []domain.Heading{
			printed(1, "Part I. Foundations", 1),
			printed(2, "First Steps", 1),
			printed(2, "Second Steps", 12),
		},
		Typographic: []domain.Heading{
			set("Part I. Foundations", 6),
			set("First Steps", 6),
			set("Second Steps", 17),
		},
	})

	assert.Equal(t, 5, report.Offset, "the offset that puts both chapters on their exact page")
	assert.True(t, report.GatePassed)

	require.Len(t, sections, 3)
	indexed := byTitle(sections)
	assertRange(t, indexed["Part I. Foundations"], 6, 210, 1)
	assertRange(t, indexed["First Steps"], 6, 16, 2)
	assertRange(t, indexed["Second Steps"], 17, 210, 2)
}

func TestBuildSections_FailsTheGateWhenHalfTheChaptersConfirm(t *testing.T) {
	toc := []domain.Heading{}
	for chapter := 0; chapter < 8; chapter++ {
		toc = append(toc, printed(2, chapterTitle(chapter), 10+chapter))
	}

	// Only every other chapter is set large: the layout and the index disagree
	// about half the book, which is a misread index and not a document structure.
	typography := []domain.Heading{}
	for chapter := 0; chapter < 8; chapter += 2 {
		typography = append(typography, set(chapterTitle(chapter), 10+chapter))
	}

	sections, report := buildSections(app.StructureInput{
		DocumentID:  "document-3",
		PageCount:   100,
		TOC:         toc,
		Typographic: typography,
	})

	assert.NotNil(t, sections, "a document that fails the gate gets an empty list, never nil")
	assert.Empty(t, sections, "below the gate no section is stored (S4)")
	assert.False(t, report.GatePassed)
	assert.Equal(t, 8, report.ChaptersNamed)
	assert.Equal(t, 4, report.ChaptersConfirmed)
	assert.Contains(t, report.Reason, "95%", "the report says which gate was not cleared")
}

func TestBuildSections_ClampsASectionToThePagesItOwns(t *testing.T) {
	// The index names a section on a page this PDF does not have: a back-matter
	// row, or a row the index printed twice. It cannot own a page.
	sections, report := buildSections(app.StructureInput{
		DocumentID: "document-4",
		PageCount:  100,
		TOC: []domain.Heading{
			printed(2, "Chapter One", 10),
			printed(3, "Inside", 20),
			printed(2, "Chapter Two", 95),
			printed(3, "Beyond", 130),
			// Two sections on the page where the chapter changes: the closing
			// section of one chapter is printed on the page the next one opens.
			printed(3, "Bibliography", 100),
			printed(2, "Chapter Three", 100),
		},
		Typographic: []domain.Heading{
			set("Chapter One", 10),
			set("Inside", 20),
			set("Chapter Two", 95),
			set("Bibliography", 100),
			set("Chapter Three", 100),
		},
	})

	require.True(t, report.GatePassed)
	assert.Equal(t, 1, report.EntriesDropped, "the entry on page 130 is not a section")

	indexed := byTitle(sections)
	assert.NotContains(t, indexed, "Beyond", "an entry no page can hold is dropped, not invented")
	assertRange(t, indexed["Chapter Two"], 95, 99, 2)
	// A closing section printed where the next chapter opens ends on its own page
	// rather than on the page before it, which would be outside its chapter.
	assertRange(t, indexed["Bibliography"], 100, 100, 3)
	assertRange(t, indexed["Chapter Three"], 100, 100, 2)
}

func TestBuildSections_StopsAChildAtItsParentsEnd(t *testing.T) {
	// The section's own end — the page before the next section — is past the
	// parent part, which ends where the next part opens. A child range outside its
	// parent would attribute pages the parent never owned.
	sections, report := buildSections(app.StructureInput{
		DocumentID: "document-5",
		PageCount:  200,
		TOC: []domain.Heading{
			printed(1, "Part I. Foundations", 10),
			printed(2, "Chapter A", 12),
			printed(3, "Deep", 15),
			printed(1, "Part II. Practice", 18),
		},
		Typographic: []domain.Heading{
			set("Part I. Foundations", 10),
			set("Chapter A", 12),
			set("Deep", 15),
			set("Part II. Practice", 18),
		},
	})

	require.True(t, report.GatePassed)

	indexed := byTitle(sections)
	assertRange(t, indexed["Part I. Foundations"], 10, 17, 1)
	assertRange(t, indexed["Chapter A"], 12, 17, 2)
	assertRange(t, indexed["Deep"], 15, 17, 3)
	assertRange(t, indexed["Part II. Practice"], 18, 200, 1)
}

func TestBuildSections_ReturnsNothingForADocumentWithoutChapters(t *testing.T) {
	// A part is not a chapter: there is nothing to confirm the offset against, and
	// a range computed from an unconfirmed offset is a guess with the appearance of
	// a citation.
	sections, report := buildSections(app.StructureInput{
		DocumentID: "document-6",
		PageCount:  120,
		TOC: []domain.Heading{
			printed(1, "Part I. Foundations", 3),
			printed(3, "A Section That Is Not A Chapter", 9),
		},
		Typographic: []domain.Heading{
			set("Part I. Foundations", 3),
			set("A Section That Is Not A Chapter", 9),
		},
	})

	assert.Empty(t, sections)
	assert.False(t, report.GatePassed)
	assert.Equal(t, 0, report.ChaptersNamed)
	assert.NotEmpty(t, report.Reason, "the report says there was no chapter to confirm")

	empty, report := buildSections(app.StructureInput{DocumentID: "document-7", PageCount: 10})
	assert.NotNil(t, empty, "no index at all is an empty list, never nil")
	assert.Empty(t, empty)
	assert.False(t, report.GatePassed)
	assert.NotEmpty(t, report.Reason)
}

func TestBuildSections_KeepsAnEntryOnlyTheIndexNames(t *testing.T) {
	// Sections are only where the index names them (S4), and every section says
	// where it came from: an entry no page confirms is still the index's claim.
	sections, report := buildSections(app.StructureInput{
		DocumentID: "document-8",
		PageCount:  100,
		TOC: []domain.Heading{
			printed(1, "Part I. Foundations", 3),
			printed(2, "First Steps", 10),
			printed(2, "Second Steps", 40),
			printed(3, "A Section The Pages Do Not Set Large", 50),
		},
		Typographic: []domain.Heading{
			set("Part I. Foundations", 3),
			set("First Steps", 10),
			set("Second Steps", 40),
		},
	})

	require.True(t, report.GatePassed)
	assert.Equal(t, 2, report.ChaptersConfirmed)

	indexed := byTitle(sections)
	require.Len(t, sections, 4)
	assert.Equal(t, domain.SourceMixed, indexed["First Steps"].Source)
	assert.Equal(t, domain.SourceTOC, indexed["A Section The Pages Do Not Set Large"].Source,
		"an unconfirmed entry keeps its provenance instead of pretending to be confirmed")
	assertRange(t, indexed["A Section The Pages Do Not Set Large"], 50, 100, 3)

	for _, section := range sections {
		assert.NotEmpty(t, section.Source, "no section is stored without a provenance (S4)")
	}
}

func TestBuildSections_BreaksAnOffsetTieTowardsTheSmallerOffset(t *testing.T) {
	// Both chapters sit on a page and on the page four leaves later, so offset -2
	// and offset +2 confirm exactly two entries each. The tie has to resolve the
	// same way every run, or two ingests of one book produce two structures.
	input := app.StructureInput{
		DocumentID: "document-9",
		PageCount:  100,
		TOC: []domain.Heading{
			printed(2, "Alpha Chapter", 10),
			printed(2, "Beta Chapter", 20),
		},
		Typographic: []domain.Heading{
			set("Alpha Chapter", 8),
			set("Alpha Chapter", 12),
			set("Beta Chapter", 18),
			set("Beta Chapter", 22),
		},
	}

	for attempt := 0; attempt < 3; attempt++ {
		sections, report := buildSections(input)

		assert.True(t, report.GatePassed)
		assert.Equal(t, -2, report.Offset,
			"equally distant offsets resolve to the smaller one")

		indexed := byTitle(sections)
		require.Len(t, sections, 2)
		assert.Equal(t, 8, indexed["Alpha Chapter"].StartPage)
		assert.Equal(t, 18, indexed["Beta Chapter"].StartPage)
	}
}

// chapterTitle names the nth chapter of the generated fixtures.
func chapterTitle(chapter int) string {
	return "Chapter " + string(rune('A'+chapter)) + " Of The Book"
}

// titles lists the sections' titles in the order the assembly returned them.
func titles(sections []*domain.Section) []string {
	listed := make([]string, 0, len(sections))
	for _, section := range sections {
		listed = append(listed, section.Title)
	}
	return listed
}

// assertRange checks one section's level and page range.
func assertRange(t *testing.T, section *domain.Section, start, end, level int) {
	t.Helper()

	require.NotNil(t, section)
	assert.Equal(t, level, section.Level, "%s: level", section.Title)
	assert.Equal(t, start, section.StartPage, "%s: start page", section.Title)
	assert.Equal(t, end, section.EndPage, "%s: end page", section.Title)
}

// A letter-spaced index row cannot report its own word boundaries ("P u t t in g
//
//	t h e  Do m a in"), so a printed part title can arrive welded ("Modelto",
//
// "ofa"). When the page sets the same letters, the print is the better title and
// the citation reads correctly; the letters are what is compared, so this cannot
// rename a section.
func TestBuildSections_TakesThePrintSpacingOfAConfirmedTitle(t *testing.T) {
	input := app.StructureInput{
		DocumentID: "doc",
		PageCount:  60,
		TOC: []domain.Heading{
			printed(1, "Part II. Building Blocks ofa Model-Driven Design", 1),
			printed(2, "Crunching Knowledge", 10),
			printed(2, "Communication and the Use of Language", 30),
		},
		Typographic: []domain.Heading{
			set("Part II. Building Blocks of a Model-Driven Design", 1),
			set("Crunching Knowledge", 10),
			set("Communication and the Use of Language", 30),
		},
	}

	sections, report := buildSections(input)
	require.True(t, report.GatePassed)

	assert.Equal(t, "Part II. Building Blocks of a Model-Driven Design", sections[0].Title)
}

// The rule above is about spacing and punctuation only: a page that sets
// different words confirms the page, never the title.
func TestBuildSections_KeepsTheIndexTitleWhenThePrintSaysSomethingElse(t *testing.T) {
	input := app.StructureInput{
		DocumentID: "doc",
		PageCount:  60,
		TOC:        []domain.Heading{printed(2, "Crunching Knowledge", 10)},
		Typographic: []domain.Heading{
			set("Crunching Knowledge and What It Costs", 10),
		},
	}

	sections, report := buildSections(input)
	require.True(t, report.GatePassed)
	require.Len(t, sections, 1)

	assert.Equal(t, "Crunching Knowledge", sections[0].Title, "the index names the section")
}

// A paper prints its structure in its page text and sets its headings no larger
// than the body, so the typography cannot confirm anything. The gate for that
// source is the shape of the reading, not a confirmation: an entry read from a
// page is already on that page.
func TestBuildSections_ReadsAPaperFromItsPageText(t *testing.T) {
	input := app.StructureInput{
		DocumentID: "paper",
		PageCount:  18,
		Source:     domain.SourcePattern,
		TOC: []domain.Heading{
			printed(1, "INTRODUCTION", 1),
			printed(2, "TRES Spectra", 3),
			printed(3, "Wavelength coverage", 3),
		},
		Typographic: []domain.Heading{set("TRES Spectra", 3)},
	}

	sections, report := buildSections(input)
	require.True(t, report.GatePassed, "the reading names a first level and more than one entry")
	require.Len(t, sections, 2, "a page-text reading is trusted to two levels, no deeper")

	assert.Equal(t, "INTRODUCTION", sections[0].Title)
	assert.Equal(t, domain.SourcePattern, sections[0].Source, "no page set that title large")
	assert.Equal(t, "TRES Spectra", sections[1].Title)
	assert.Equal(t, domain.SourceMixed, sections[1].Source, "the page did set that title large")
	assert.Equal(t, domain.SourcePattern, sections[0].Source)
}

func TestBuildSections_RefusesToCallOneHeadingAStructure(t *testing.T) {
	input := app.StructureInput{
		DocumentID: "paper",
		PageCount:  8,
		Source:     domain.SourcePattern,
		TOC:        []domain.Heading{printed(1, "Introduction", 1)},
	}

	sections, report := buildSections(input)
	assert.False(t, report.GatePassed)
	assert.Empty(t, sections)
	assert.Contains(t, report.Reason, "too thin")
}
