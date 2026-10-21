package pdf_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/infra/pdf"
)

// The helpers below reproduce poppler's pdftohtml -xml shape: one element per
// line, a <page> per page, and one <text> run per styled fragment on a baseline.

// tocText renders one <text> run.
func tocText(top, left int, body string) string {
	return fmt.Sprintf(`<text top="%d" left="%d" width="90" height="15" font="0">%s</text>`, top, left, body)
}

// tocLeader is the run of dots that separates a title from its page number.
func tocLeader(top int) string {
	return tocText(top, 400, strings.Repeat(".", 14))
}

// tocRow renders one index row — title, leader dots, printed page — as the
// several runs poppler emits for it.
func tocRow(top, left int, title, page string) []string {
	return []string{tocText(top, left, title), tocLeader(top), tocText(top, 560, page)}
}

// tocRuns flattens the rows of one page into its runs.
func tocRuns(rows ...[]string) []string {
	runs := []string{}
	for _, row := range rows {
		runs = append(runs, row...)
	}
	return runs
}

// tocPage renders one <page> element.
func tocPage(number, height int, runs ...string) string {
	return fmt.Sprintf(
		`<page number="%d" position="absolute" top="0" left="0" height="%d" width="612">`+"\n%s\n</page>",
		number, height, strings.Join(runs, "\n"))
}

// tocDocument wraps pages in the document pdftohtml writes.
func tocDocument(pages ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE pdf2xml SYSTEM "pdf2xml.dtd">

<pdf2xml producer="poppler" version="25.11.0">
<fontspec id="0" size="15" family="Times" color="#000000"/>
<fontspec id="1" size="27" family="Times-Bold" color="#000000"/>
` + strings.Join(pages, "\n") + `
</pdf2xml>
`
}

// tocHeadings runs the extractor over canned XML through a fake runner.
func tocHeadings(t *testing.T, document string) []domain.Heading {
	t.Helper()

	stub := &stubRunner{fn: func(string, []string) ([]byte, error) { return []byte(document), nil }}

	extractor, err := pdf.NewTOCExtractor("book.pdf", stub.runner())
	require.NoError(t, err)

	headings, err := extractor.Headings(t.Context())
	require.NoError(t, err)
	return headings
}

func TestTOCExtractor_ReadsAOnePageIndexInReadingOrder(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte(tocDocument(tocPage(3, 792, tocRuns(
			[]string{tocText(100, 200, "Table of Contents")},
			tocRow(200, 72, "Part I.Putting the Domain Model", "12"),
			tocRow(220, 72, "Preface", "7"),
			tocRow(240, 150, "1.Crunching Knowledge", "15"),
			tocRow(260, 180, "Breaking Through", "136"),
		)...))), nil
	}}

	extractor, err := pdf.NewTOCExtractor("book.pdf", stub.runner())
	require.NoError(t, err)

	headings, err := extractor.Headings(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []domain.Heading{
		// A part keeps its "Part" prefix; a page marker in the far-left label
		// column is a part too.
		{Level: 1, Title: "Part I.Putting the Domain Model", Page: 12},
		{Level: 1, Title: "Preface", Page: 7},
		// A numbered chapter loses its number.
		{Level: 2, Title: "Crunching Knowledge", Page: 15},
		{Level: 3, Title: "Breaking Through", Page: 136},
	}, headings)

	assert.Equal(t, []string{"pdftohtml -xml -stdout -i book.pdf"}, stub.calls)
}

func TestTOCExtractor_ReadsAnIndexThatLosesItsTitleOnTheNextPage(t *testing.T) {
	// An index that spans pages prints "Table of Contents" on the first page
	// only. Detecting the index by its title would keep this page and drop the
	// next one, which carries most of the entries.
	document := tocDocument(
		tocPage(3, 792, tocRuns(
			[]string{tocText(100, 200, "Table of Contents")},
			tocRow(200, 150, "1.Crunching Knowledge", "15"),
			tocRow(220, 180, "Listen to Language", "145"),
			tocRow(240, 180, "Scrutinize Awkwardness", "148"),
		)...),
		tocPage(4, 792, tocRuns(
			tocRow(100, 150, "2.Contemplate Contradictions", "152"),
			tocRow(120, 180, "Read the Book", "153"),
			tocRow(140, 180, "Try, Try Again", "154"),
		)...),
	)

	headings := tocHeadings(t, document)

	assert.Equal(t, []domain.Heading{
		{Level: 2, Title: "Crunching Knowledge", Page: 15},
		{Level: 3, Title: "Listen to Language", Page: 145},
		{Level: 3, Title: "Scrutinize Awkwardness", Page: 148},
		{Level: 2, Title: "Contemplate Contradictions", Page: 152},
		{Level: 3, Title: "Read the Book", Page: 153},
		{Level: 3, Title: "Try, Try Again", Page: 154},
	}, headings)
}

func TestTOCExtractor_KeepsADropCapWordOnOneLine(t *testing.T) {
	// The drop cap sits two points above the word it belongs to. Anchoring the
	// line anywhere but on its first run splits the "C" off, and the row then
	// reads "runching Knowledge".
	document := tocDocument(tocPage(5, 792, tocRuns(
		[]string{
			tocText(199, 180, "C"),
			tocText(201, 196, "runching Knowledge"),
			tocLeader(201),
			tocText(201, 560, "15"),
		},
		tocRow(220, 180, "Listen to Language", "145"),
		tocRow(240, 180, "Scrutinize Awkwardness", "148"),
	)...))

	headings := tocHeadings(t, document)

	require.Len(t, headings, 3)
	assert.Equal(t, domain.Heading{Level: 3, Title: "Crunching Knowledge", Page: 15}, headings[0])
}

func TestTOCExtractor_IgnoresWhitespaceOnlySpacers(t *testing.T) {
	// poppler emits the page's invisible margin spacers as whitespace-only runs.
	// A spacer must never join a line: one sits at the far-left margin, so a line
	// that took its left edge would read as a part.
	document := tocDocument(tocPage(6, 792, tocRuns(
		[]string{
			tocText(196, 20, "   "),
			tocText(201, 20, "   "),
			tocText(201, 180, "Crunching Knowledge"),
			tocLeader(201),
			tocText(201, 560, "15"),
		},
		tocRow(220, 180, "Listen to Language", "145"),
		tocRow(240, 180, "Scrutinize Awkwardness", "148"),
	)...))

	headings := tocHeadings(t, document)

	assert.Equal(t, []domain.Heading{
		{Level: 3, Title: "Crunching Knowledge", Page: 15},
		{Level: 3, Title: "Listen to Language", Page: 145},
		{Level: 3, Title: "Scrutinize Awkwardness", Page: 148},
	}, headings)
}

func TestTOCExtractor_JoinsALetterSpacedPageNumber(t *testing.T) {
	document := tocDocument(tocPage(7, 792, tocRuns(
		tocRow(200, 180, "Crunching Knowledge", "1 2"),
		tocRow(220, 180, "Listen to Language", "2 5"),
		tocRow(240, 180, "Try, Try Again", "3 0 7"),
	)...))

	headings := tocHeadings(t, document)

	// The print spaces a page number's digits: "1 2" is page 12, not page 1.
	assert.Equal(t, []domain.Heading{
		{Level: 3, Title: "Crunching Knowledge", Page: 12},
		{Level: 3, Title: "Listen to Language", Page: 25},
		{Level: 3, Title: "Try, Try Again", Page: 307},
	}, headings)
}

func TestTOCExtractor_DespacesALetterSpacedTitle(t *testing.T) {
	// A letter-spaced title arrives one glyph at a time, with a single space
	// inside a word and a double space between words.
	document := tocDocument(tocPage(8, 792, tocRuns(
		tocRow(200, 180, "P u t t in g  t h e  Do m a in", "12"),
		tocRow(220, 180, "Listen to Language", "145"),
		tocRow(240, 180, "Try, Try Again", "154"),
	)...))

	headings := tocHeadings(t, document)

	require.Len(t, headings, 3)
	assert.Equal(t, "Putting the Domain", headings[0].Title)
}

func TestTOCExtractor_JoinsASmallCapsTitle(t *testing.T) {
	// Small caps arrive as a first-letter run at one size and the remainder at
	// another; the two must join into one word.
	document := tocDocument(tocPage(9, 792, tocRuns(
		[]string{
			tocText(200, 180, "U"),
			tocText(200, 196, "BIQUITOUS "),
			tocText(200, 258, "L"),
			tocText(200, 270, "ANGUAGE"),
			tocLeader(200),
			tocText(200, 560, "25"),
		},
		tocRow(220, 180, "Listen to Language", "145"),
		tocRow(240, 180, "Try, Try Again", "154"),
	)...))

	headings := tocHeadings(t, document)

	require.Len(t, headings, 3)
	assert.Equal(t, domain.Heading{Level: 3, Title: "UBIQUITOUS LANGUAGE", Page: 25}, headings[0])
}

func TestTOCExtractor_ReturnsNoEntriesForADocumentWithoutAnIndex(t *testing.T) {
	document := tocDocument(tocPage(1, 792,
		tocText(200, 72, "TRES Spectra: A Field Study"),
		tocText(230, 72, "1. INTRODUCTION"),
	))

	headings := tocHeadings(t, document)

	assert.NotNil(t, headings, "a document with no index gets an empty list, never nil")
	assert.Empty(t, headings, "nothing is invented when there is no index")
}

func TestTOCExtractor_DropsARowThatNamesNoPage(t *testing.T) {
	// A row whose page number never arrives names no page, so it is not an
	// entry: keeping it would need a page the index never printed.
	document := tocDocument(tocPage(10, 792, tocRuns(
		[]string{tocText(180, 180, "Acknowledgements"), tocLeader(180)},
		tocRow(200, 180, "Preface", "7"),
		tocRow(220, 180, "Glossary", "353"),
	)...))

	headings := tocHeadings(t, document)

	assert.Equal(t, []domain.Heading{
		{Level: 3, Title: "Preface", Page: 7},
		{Level: 3, Title: "Glossary", Page: 353},
	}, headings)
}

func TestTOCExtractor_IgnoresTheRunningFooter(t *testing.T) {
	// A running footer sits below the text block. It is not an index row even
	// when it carries leader dots.
	document := tocDocument(tocPage(11, 792, tocRuns(
		tocRow(200, 180, "Preface", "7"),
		tocRow(220, 180, "Glossary", "353"),
		tocRow(240, 180, "References", "356"),
		tocRow(760, 180, "Index of terms", "400"),
	)...))

	headings := tocHeadings(t, document)

	assert.Equal(t, []domain.Heading{
		{Level: 3, Title: "Preface", Page: 7},
		{Level: 3, Title: "Glossary", Page: 353},
		{Level: 3, Title: "References", Page: 356},
	}, headings)
}

func TestTOCExtractor_KeepsAnEntryPrintedOnce(t *testing.T) {
	// A title the index prints twice is one section, not two.
	document := tocDocument(
		tocPage(12, 792, tocRuns(
			tocRow(200, 180, "Glossary", "353"),
			tocRow(220, 180, "References", "356"),
			tocRow(240, 180, "Glossary", "353"),
		)...),
	)

	headings := tocHeadings(t, document)

	assert.Equal(t, []domain.Heading{
		{Level: 3, Title: "Glossary", Page: 353},
		{Level: 3, Title: "References", Page: 356},
	}, headings)
}

func TestTOCExtractor_SurfacesProcessFailure(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return nil, errors.New("pdftohtml: exit status 1: Couldn't find trailer dictionary")
	}}

	extractor, err := pdf.NewTOCExtractor("book.pdf", stub.runner())
	require.NoError(t, err)

	_, err = extractor.Headings(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailer dictionary", "the process message must survive the wrap")
}

func TestTOCExtractor_RejectsUnparseableCoordinates(t *testing.T) {
	document := tocDocument(tocPage(13, 792,
		`<text top="many" left="180" width="90" height="15" font="0">Glossary</text>`,
	))

	stub := &stubRunner{fn: func(string, []string) ([]byte, error) { return []byte(document), nil }}
	extractor, err := pdf.NewTOCExtractor("book.pdf", stub.runner())
	require.NoError(t, err)

	_, err = extractor.Headings(t.Context())
	require.Error(t, err, "a run that cannot be placed must be reported, never silently dropped")
	assert.Contains(t, err.Error(), "many")
}

// The same section title closes most chapters of a real book. Collapsing those
// rows by title alone would keep one "Summary" and silently lose the rest of the
// document's structure.
func TestTOCExtractor_KeepsTheSameTitleOnDifferentPages(t *testing.T) {
	document := tocDocument(tocPage(3, 1188,
		tocRuns(
			tocRow(100, 150, "1. Crunching Knowledge", "15"),
			tocRow(120, 200, "Summary", "23"),
			tocRow(140, 150, "2. Communication and the Use of Language", "24"),
			tocRow(160, 200, "Summary", "40"),
		)...,
	))

	headings := tocHeadings(t, document)
	require.Len(t, headings, 4, "two chapters and the two Summaries that close them")

	assert.Equal(t, "Summary", headings[1].Title)
	assert.Equal(t, 23, headings[1].Page)
	assert.Equal(t, "Summary", headings[3].Title)
	assert.Equal(t, 40, headings[3].Page)
}

// A letter-spaced word is not a run of short words: the glyphs of one word can be
// longer than two characters ("ild", "l-Dr") and they still belong together.
func TestTOCExtractor_JoinsAWordWhoseGlyphsAreLongerThanTwoCharacters(t *testing.T) {
	document := tocDocument(tocPage(3, 1188,
		tocRuns(
			tocRow(100, 150, "P a r t  II.", "49"),
			tocRow(120, 150, "B u ild in g  o f  a  Mo d e l-Dr iv e n", "49"),
			tocRow(140, 200, "Glossary", "353"),
		)...,
	))

	headings := tocHeadings(t, document)
	require.Len(t, headings, 3)

	assert.Equal(t, "Building of a Model-Driven", headings[1].Title)
}

// The rule above must not fire on a title that is set normally: joining it would
// weld three words into one.
func TestTOCExtractor_KeepsTheSpacesOfANormallySetTitle(t *testing.T) {
	document := tocDocument(tocPage(3, 1188,
		tocRuns(
			tocRow(100, 200, "Read the Book", "153"),
			tocRow(120, 200, "Glossary", "353"),
			tocRow(140, 200, "References", "356"),
		)...,
	))

	headings := tocHeadings(t, document)
	require.Len(t, headings, 3)

	assert.Equal(t, "Read the Book", headings[0].Title)
}
