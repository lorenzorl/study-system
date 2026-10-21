package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
)

// fakeHeadings is one reading of a document's structure: a fixed list, or a
// failure, and how many times the use case asked for it.
type fakeHeadings struct {
	headings []domain.Heading
	err      error
	calls    int
}

func (f *fakeHeadings) Headings(context.Context) ([]domain.Heading, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.headings, nil
}

// sourceFactory stands in for the pdf extractors: it builds the two readers for
// the path it is handed and remembers that path, because the payload the readers
// read is the caller's and never the use case's.
type sourceFactory struct {
	sources  app.StructureSources
	err      error
	attempts int
	paths    []string
}

func (f *sourceFactory) build(pdfPath string) (app.StructureSources, error) {
	f.attempts++
	f.paths = append(f.paths, pdfPath)

	if f.err != nil {
		return app.StructureSources{}, f.err
	}
	return f.sources, nil
}

// newStructure wires the structure stage over the harness's real repositories.
// The sections are the SQLite adapter over the same in-memory database the
// ingestion harness uses, so what a test reads back is what a later reader of the
// corpus would get.
func newStructure(
	t *testing.T, h *harness, factory *sourceFactory,
) (*app.StructureDocumentUseCase, domain.SectionRepository) {
	t.Helper()

	sections := ragsqlite.NewSectionRepository(h.sqlDB)

	return app.NewStructureDocumentUseCase(sections, h.jobs, h.pages, factory.build), sections
}

// indexRun executes one structure run for the harness's only document.
func indexRun(t *testing.T, h *harness, useCase *app.StructureDocumentUseCase) app.StructureDocumentReport {
	t.Helper()

	report, err := useCase.Execute(t.Context(), app.StructureRequest{
		DocumentID: h.onlyDocument(t).ID,
		PDFPath:    h.pdfPath,
	})
	require.NoError(t, err)

	return report
}

// indexedTOC is a small index in printed pages: a part, two chapters and a
// section inside the first chapter.
func indexedTOC() []domain.Heading {
	return []domain.Heading{
		printed(1, "Part I. Putting the Domain Model to Work", 3),
		printed(2, "Crunching Knowledge", 15),
		printed(3, "Listen to Language", 15),
		printed(2, "Communication and the Use of Language", 30),
	}
}

// confirmingTypography is what those pages set large: the part and the two
// chapters, and not the section inside the first one — which is what the real
// book does for most of its sections, and what makes provenance worth asserting.
func confirmingTypography() []domain.Heading {
	return []domain.Heading{
		set("Part I. Putting the Domain Model to Work", 3),
		set("Crunching Knowledge", 15),
		set("Communication and the Use of Language", 30),
	}
}

// paperPages are the recorded pages of a document with no index: its numbered
// headings are all the structure it prints, on one extracted page and one the
// model transcribed.
func paperPages() []domain.PageOutcome {
	return []domain.PageOutcome{
		{
			PageNumber: 1,
			Status:     domain.PageExtracted,
			Method:     domain.MethodExtract,
			Text:       "1. INTRODUCTION\n\nThe paper describes a survey of spectra.",
		},
		{
			PageNumber: 3,
			Status:     domain.PageTranscribed,
			Method:     domain.MethodVision,
			Text:       "2.1. TRES Spectra\n\nEach spectrum was reduced by hand.",
		},
	}
}

func TestStructureDocumentUseCase_StoresTheSectionsTheIndexNames(t *testing.T) {
	h := newHarness(t, 359, []byte("book"))
	h.seedRecorded(t, 359, nil)
	document := h.onlyDocument(t)

	index := &fakeHeadings{headings: indexedTOC()}
	typographic := &fakeHeadings{headings: confirmingTypography()}
	factory := &sourceFactory{sources: app.StructureSources{Index: index, Typographic: typographic}}
	useCase, sections := newStructure(t, h, factory)

	report := indexRun(t, h, useCase)

	assert.Equal(t, []string{h.pdfPath}, factory.paths,
		"the readers are built over the payload the caller named")
	assert.Equal(t, 1, index.calls)
	assert.Equal(t, 1, typographic.calls, "the typography is read on every path: it confirms the index")

	assert.Equal(t, 4, report.Sections)
	require.NotNil(t, report.Structure)
	assert.True(t, report.Structure.GatePassed)
	assert.Equal(t, 2, report.Structure.ChaptersNamed)
	assert.Equal(t, 2, report.Structure.ChaptersConfirmed)
	assert.Empty(t, report.Reason, "a document with sections needs no explanation")

	stored, err := sections.ListByDocument(t.Context(), document.ID)
	require.NoError(t, err)
	require.Len(t, stored, 4, "the report counts what the document now has")

	indexed := byTitle(stored)
	assert.Equal(t, domain.SourceMixed, indexed["Part I. Putting the Domain Model to Work"].Source,
		"the index named it and a page set it large")
	assert.Equal(t, domain.SourceMixed, indexed["Crunching Knowledge"].Source)
	assert.Equal(t, domain.SourceTOC, indexed["Listen to Language"].Source,
		"no page confirms this one, so it is the index's claim alone")
	assert.Equal(t, indexed["Crunching Knowledge"].ID, indexed["Listen to Language"].ParentID,
		"the nesting the index prints survives the store")

	assert.Equal(t, 3, indexed["Part I. Putting the Domain Model to Work"].StartPage)
	assert.Equal(t, 359, indexed["Part I. Putting the Domain Model to Work"].EndPage)
}

func TestStructureDocumentUseCase_ReadsTheNumberedHeadingsWhenThereIsNoIndex(t *testing.T) {
	h := newHarness(t, 6, []byte("paper"))
	h.seedRecorded(t, 6, paperPages())
	document := h.onlyDocument(t)

	// The document prints no index at all: an empty list and no error.
	index := &fakeHeadings{}
	typographic := &fakeHeadings{headings: []domain.Heading{set("TRES Spectra", 3)}}
	factory := &sourceFactory{sources: app.StructureSources{Index: index, Typographic: typographic}}
	useCase, sections := newStructure(t, h, factory)

	report := indexRun(t, h, useCase)

	assert.Equal(t, 1, index.calls, "the index is asked first and falls through")
	assert.Equal(t, 2, report.Sections)
	require.NotNil(t, report.Structure)
	assert.True(t, report.Structure.GatePassed)
	assert.Empty(t, report.Reason)

	stored, err := sections.ListByDocument(t.Context(), document.ID)
	require.NoError(t, err)
	require.Len(t, stored, 2)

	indexed := byTitle(stored)
	assert.Contains(t, indexed, "INTRODUCTION")
	assert.Equal(t, 1, indexed["INTRODUCTION"].StartPage, "the page text names its own page")
	assert.Equal(t, domain.SourcePattern, indexed["INTRODUCTION"].Source,
		"the page text named it, so it must not claim a printed index")
	assert.Equal(t, 2, indexed["TRES Spectra"].Level, "the level is the printed numbering's")
	assert.Equal(t, domain.SourceMixed, indexed["TRES Spectra"].Source)
}

func TestStructureDocumentUseCase_AnExtractionFailureIsReportedAndNeverFatal(t *testing.T) {
	t.Run("the readers cannot be built", func(t *testing.T) {
		h := newHarness(t, 10, []byte("book"))
		h.seedRecorded(t, 10, nil)
		document := h.onlyDocument(t)

		factory := &sourceFactory{err: errors.New("pdftohtml is not installed")}
		useCase, sections := newStructure(t, h, factory)

		report, err := useCase.Execute(t.Context(), app.StructureRequest{
			DocumentID: document.ID,
			PDFPath:    h.pdfPath,
		})

		require.NoError(t, err, "a missing extractor never fails the document (S5)")
		assert.Equal(t, 0, report.Sections)
		assert.Contains(t, report.Reason, "pdftohtml")
		assert.Nil(t, report.Structure, "no reading happened, so no decision is reported")

		stored, err := sections.ListByDocument(t.Context(), document.ID)
		require.NoError(t, err)
		assert.Empty(t, stored)
	})

	t.Run("the readers fail", func(t *testing.T) {
		h := newHarness(t, 10, []byte("book"))
		h.seedRecorded(t, 10, nil)
		document := h.onlyDocument(t)

		broken := errors.New("cannot parse the payload")
		factory := &sourceFactory{sources: app.StructureSources{
			Index:       &fakeHeadings{err: broken},
			Typographic: &fakeHeadings{err: broken},
		}}
		useCase, sections := newStructure(t, h, factory)

		report, err := useCase.Execute(t.Context(), app.StructureRequest{
			DocumentID: document.ID,
			PDFPath:    h.pdfPath,
		})

		require.NoError(t, err)
		assert.Equal(t, 1, factory.attempts)
		assert.Equal(t, 0, report.Sections)
		assert.Contains(t, report.Reason, "cannot parse the payload")
		assert.Nil(t, report.Structure)

		stored, err := sections.ListByDocument(t.Context(), document.ID)
		require.NoError(t, err)
		assert.Empty(t, stored)
	})

	t.Run("a document whose sections were already read keeps them", func(t *testing.T) {
		h := newHarness(t, 359, []byte("book"))
		h.seedRecorded(t, 359, nil)
		document := h.onlyDocument(t)

		factory := &sourceFactory{sources: app.StructureSources{
			Index:       &fakeHeadings{headings: indexedTOC()},
			Typographic: &fakeHeadings{headings: confirmingTypography()},
		}}
		useCase, sections := newStructure(t, h, factory)

		first := indexRun(t, h, useCase)
		require.Equal(t, 4, first.Sections)

		// The tool disappears between two runs of the same document.
		factory.err = errors.New("pdftohtml is not installed")
		second := indexRun(t, h, useCase)

		assert.Equal(t, 0, second.Sections)
		assert.Contains(t, second.Reason, "pdftohtml")

		stored, err := sections.ListByDocument(t.Context(), document.ID)
		require.NoError(t, err)
		assert.Len(t, stored, 4,
			"a reading that never happened cannot say the document has no structure")
	})
}

func TestStructureDocumentUseCase_ARunReplacesRatherThanAppends(t *testing.T) {
	h := newHarness(t, 359, []byte("book"))
	h.seedRecorded(t, 359, nil)
	document := h.onlyDocument(t)

	factory := &sourceFactory{sources: app.StructureSources{
		Index:       &fakeHeadings{headings: indexedTOC()},
		Typographic: &fakeHeadings{headings: confirmingTypography()},
	}}
	useCase, sections := newStructure(t, h, factory)

	first := indexRun(t, h, useCase)
	second := indexRun(t, h, useCase)

	assert.Equal(t, 4, first.Sections)
	assert.Equal(t, 4, second.Sections)

	stored, err := sections.ListByDocument(t.Context(), document.ID)
	require.NoError(t, err)
	assert.Len(t, stored, 4, "a second run replaces the rows instead of adding a second copy")
}

func TestStructureDocumentUseCase_AGateThatFailsStoresNoSections(t *testing.T) {
	h := newHarness(t, 359, []byte("book"))
	h.seedRecorded(t, 359, nil)
	document := h.onlyDocument(t)

	factory := &sourceFactory{sources: app.StructureSources{
		Index:       &fakeHeadings{headings: indexedTOC()},
		Typographic: &fakeHeadings{headings: confirmingTypography()},
	}}
	useCase, sections := newStructure(t, h, factory)

	first := indexRun(t, h, useCase)
	require.Equal(t, 4, first.Sections)

	// The same index, but no page sets its chapters large: exactly the book the
	// gate exists to refuse.
	factory.sources = app.StructureSources{
		Index:       &fakeHeadings{headings: indexedTOC()},
		Typographic: &fakeHeadings{},
	}

	report := indexRun(t, h, useCase)

	assert.Equal(t, 0, report.Sections)
	require.NotNil(t, report.Structure)
	assert.False(t, report.Structure.GatePassed)
	assert.Contains(t, report.Reason, "95%")

	stored, err := sections.ListByDocument(t.Context(), document.ID)
	require.NoError(t, err)
	assert.Empty(t, stored, "the gate's zero is what the database holds (S4)")
}

func TestStructureDocumentUseCase_AMissingPayloadIsReportedAndStoresNothing(t *testing.T) {
	h := newHarness(t, 10, []byte("book"))
	h.seedRecorded(t, 10, nil)
	document := h.onlyDocument(t)

	factory := &sourceFactory{sources: app.StructureSources{
		Index:       &fakeHeadings{headings: indexedTOC()},
		Typographic: &fakeHeadings{headings: confirmingTypography()},
	}}
	useCase, sections := newStructure(t, h, factory)

	report, err := useCase.Execute(t.Context(), app.StructureRequest{DocumentID: document.ID})

	require.NoError(t, err)
	assert.Equal(t, 0, factory.attempts, "no reader is built for a payload that is not there")
	assert.Equal(t, 0, report.Sections)
	assert.Contains(t, report.Reason, "payload")

	stored, err := sections.ListByDocument(t.Context(), document.ID)
	require.NoError(t, err)
	assert.Empty(t, stored)
}

func TestProcessDocumentUseCase_RunsTheStructureStageAfterTheEmbeddings(t *testing.T) {
	t.Run("a failing structure stage leaves the book searchable", func(t *testing.T) {
		h, process, _ := newPipeline(t, 3, []byte("text-only book"))
		document := seedUploadedDocument(t, h, "Test Book")

		sections := ragsqlite.NewSectionRepository(h.sqlDB)
		structure := app.NewStructureDocumentUseCase(sections, h.jobs, h.pages,
			func(string) (app.StructureSources, error) {
				return app.StructureSources{}, errors.New("pdftohtml is not installed")
			})

		report, err := process.WithStructure(structure).Execute(t.Context(), document.ID)

		require.NoError(t, err, "a structure problem never fails the document (S5)")
		assert.Equal(t, 0, report.Sections)
		assert.Greater(t, report.Chunks, 0, "the book is still chunked")
		assert.Equal(t, report.Chunks, report.Embeddings, "and still embedded")

		stored, err := sections.ListByDocument(t.Context(), document.ID)
		require.NoError(t, err)
		assert.Empty(t, stored)
	})

	t.Run("the section count reaches the report", func(t *testing.T) {
		h, process, _ := newPipeline(t, 3, []byte("text-only book"))
		document := seedUploadedDocument(t, h, "Test Book")

		sections := ragsqlite.NewSectionRepository(h.sqlDB)
		structure := app.NewStructureDocumentUseCase(sections, h.jobs, h.pages,
			func(string) (app.StructureSources, error) {
				return app.StructureSources{
					Index: &fakeHeadings{headings: []domain.Heading{
						printed(1, "Part One", 1),
						printed(2, "Chapter One", 2),
					}},
					Typographic: &fakeHeadings{headings: []domain.Heading{
						set("Part One", 1),
						set("Chapter One", 2),
					}},
				}, nil
			})

		report, err := process.WithStructure(structure).Execute(t.Context(), document.ID)

		require.NoError(t, err)
		assert.Equal(t, 2, report.Sections)
	})
}

// An index that names no chapter is not an index. A table page's leader-dot rows
// look exactly like one, and accepting them as the document's structure would
// leave the document with none: the real figures paper has one such row.
func TestStructureDocumentUseCase_FallsBackWhenTheIndexNamesNoChapter(t *testing.T) {
	h := newHarness(t, 6, []byte("paper"))
	h.seedRecorded(t, 6, paperPages())
	document := h.onlyDocument(t)

	// One section-level row and no chapter: what a table of values looks like.
	index := &fakeHeadings{headings: []domain.Heading{printed(3, "Characteristic", 8)}}
	typographic := &fakeHeadings{headings: []domain.Heading{set("TRES Spectra", 3)}}
	factory := &sourceFactory{sources: app.StructureSources{Index: index, Typographic: typographic}}
	useCase, sections := newStructure(t, h, factory)

	report := indexRun(t, h, useCase)

	assert.Equal(t, 1, index.calls, "the index is asked first and rejected as one")
	require.NotNil(t, report.Structure)
	assert.True(t, report.Structure.GatePassed)

	stored, err := sections.ListByDocument(t.Context(), document.ID)
	require.NoError(t, err)
	require.Len(t, stored, 2, "the page text supplied the structure instead")

	indexed := byTitle(stored)
	assert.Contains(t, indexed, "INTRODUCTION")
	assert.NotContains(t, indexed, "Characteristic", "the rejected index contributes nothing")
	assert.Equal(t, domain.SourcePattern, indexed["INTRODUCTION"].Source)
}
