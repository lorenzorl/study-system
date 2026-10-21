package pdf_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/infra/pdf"
)

// The font ids headingDocument declares: the body size, the two sizes that
// straddle the heading threshold, and a display size.
const (
	bodyFont    = 0 // 15: the size most of a page is set in
	underFont   = 1 // 18: body + 3, not a heading
	atFont      = 2 // 19: body + 4, the smallest heading
	displayFont = 3 // 27: a part or a chapter title
	smallFont   = 4 // 12: a footnote or a caption
)

// headingText renders one <text> run set in the font with the given id.
func headingText(top, left, font int, body string) string {
	return fmt.Sprintf(`<text top="%d" left="%d" width="90" height="15" font="%d">%s</text>`,
		top, left, font, body)
}

// bodyLine renders one line of body text, long enough that the body size wins the
// character count even in a fixture this small.
func bodyLine(top int) string {
	return headingText(top, 72, bodyFont,
		"The body of the page is set in the document's body size, and that size is what most of the book is printed in.")
}

// headingDocument wraps pages in the document pdftohtml writes, declaring one
// font size per id so the extractor can resolve a run's size.
func headingDocument(pages ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE pdf2xml SYSTEM "pdf2xml.dtd">

<pdf2xml producer="poppler" version="25.11.0">
<fontspec id="0" size="15" family="Times" color="#000000"/>
<fontspec id="1" size="18" family="Times-Bold" color="#000000"/>
<fontspec id="2" size="19" family="Times-Bold" color="#000000"/>
<fontspec id="3" size="27" family="Times-Bold" color="#000000"/>
<fontspec id="4" size="12" family="Times-Roman" color="#000000"/>
` + strings.Join(pages, "\n") + `
</pdf2xml>
`
}

// typographicHeadings runs the extractor over canned XML through a fake runner.
func typographicHeadings(t *testing.T, document string) []domain.Heading {
	t.Helper()

	stub := &stubRunner{fn: func(string, []string) ([]byte, error) { return []byte(document), nil }}

	extractor, err := pdf.NewTypographicHeadings("book.pdf", stub.runner())
	require.NoError(t, err)

	headings, err := extractor.Headings(t.Context())
	require.NoError(t, err)
	return headings
}

func TestTypographicHeadings_FindsTheLinesSetLargerThanTheBody(t *testing.T) {
	document := headingDocument(
		tocPage(3, 792,
			bodyLine(100), bodyLine(120), bodyLine(140),
			headingText(200, 72, displayFont, "Crunching Knowledge"),
			bodyLine(240), bodyLine(260),
		),
		tocPage(7, 792,
			bodyLine(100), bodyLine(120),
			headingText(200, 72, atFont, "Strategy"),
			bodyLine(240),
		),
	)

	headings := typographicHeadings(t, document)

	// The page is the PDF page the line sits on, not a number the index printed.
	// The level is 1 because typography cannot declare nesting: the index supplies
	// the levels and this list only confirms titles on pages.
	assert.Equal(t, []domain.Heading{
		{Level: 1, Title: "Crunching Knowledge", Page: 3},
		{Level: 1, Title: "Strategy", Page: 7},
	}, headings)
}

func TestTypographicHeadings_ComputesTheBodySizeForTheWholeDocument(t *testing.T) {
	// The body size is the document's most-printed size, not each page's. A page
	// set almost entirely in a smaller size — a footnote page, a caption block —
	// would otherwise report its own size as the body and promote the document's
	// real body text to a heading.
	document := headingDocument(
		tocPage(3, 792,
			bodyLine(100), bodyLine(120), bodyLine(140),
			bodyLine(160), bodyLine(180), bodyLine(200),
		),
		tocPage(4, 792,
			headingText(100, 72, smallFont, "A caption set small, long enough to outweigh the three body lines beside it, because the page's own text is short."),
			headingText(120, 72, smallFont, "Another caption set small, and also long enough to win a page-local character count quite easily."),
			headingText(140, 72, smallFont, "A third caption set small, so that a page-local answer would call this page's body 12 points."),
			headingText(200, 72, underFont, "Not A Heading In This Document"),
		),
	)

	headings := typographicHeadings(t, document)

	// 18 is body + 3 for the document's body of 15, and body + 6 for this page's
	// 12. It is only a heading if the body were measured per page.
	assert.Empty(t, headings)
}

func TestTypographicHeadings_RejectsALineThreePointsAboveTheBody(t *testing.T) {
	document := headingDocument(tocPage(5, 792,
		bodyLine(100), bodyLine(120), bodyLine(140),
		headingText(200, 72, underFont, "Almost A Heading"),
		headingText(240, 72, atFont, "Actually A Heading"),
	))

	headings := typographicHeadings(t, document)

	require.Len(t, headings, 1)
	assert.Equal(t, "Actually A Heading", headings[0].Title)
}

func TestTypographicHeadings_IgnoresTheRunningFooter(t *testing.T) {
	// A running footer sits below the text block, and on this book it is set
	// large. Counting it would make the footer size the document's body and leave
	// nothing above the threshold; accepting it would report a page number as a
	// heading.
	footer := strings.Repeat("A running footer set in the display size, printed on every page of this fixture. ", 3)

	document := headingDocument(
		tocPage(6, 792,
			bodyLine(100), bodyLine(120),
			headingText(200, 72, atFont, "Real Heading"),
			headingText(760, 72, displayFont, footer),
		),
		tocPage(8, 792,
			bodyLine(100), bodyLine(120),
			headingText(760, 72, displayFont, footer),
		),
	)

	headings := typographicHeadings(t, document)

	assert.Equal(t, []domain.Heading{
		{Level: 1, Title: "Real Heading", Page: 6},
	}, headings)
}

func TestTypographicHeadings_DespacesALetterSpacedTitle(t *testing.T) {
	// A letter-spaced title arrives one glyph per run. Its runs are one line, and
	// its title is one title — welded back into words, not left as glyphs.
	document := headingDocument(tocPage(9, 792,
		bodyLine(100), bodyLine(120),
		headingText(200, 72, displayFont, "P a r t  I .  P u t t in g"),
		bodyLine(240),
	))

	headings := typographicHeadings(t, document)

	require.Len(t, headings, 1)
	assert.Equal(t, "Part I. Putting", headings[0].Title)
}

func TestTypographicHeadings_RejectsADisplayLineThatIsNotATitle(t *testing.T) {
	// A figure axis and a table row can be set above the body size. Neither names
	// anything a reader could cite, and both are what makes typography a
	// confirmer rather than a source.
	document := headingDocument(tocPage(10, 792,
		bodyLine(100), bodyLine(120),
		headingText(200, 72, displayFont, "0.0 0.1 0.2 0.3 0.4"),
		headingText(240, 72, displayFont, strings.Repeat("A title nobody would print in one line, ", 4)),
		headingText(280, 72, displayFont, "A Real Title"),
	))

	headings := typographicHeadings(t, document)

	assert.Equal(t, []domain.Heading{
		{Level: 1, Title: "A Real Title", Page: 10},
	}, headings)
}

func TestTypographicHeadings_ReturnsNoHeadingsForADocumentWithoutThem(t *testing.T) {
	document := headingDocument(
		tocPage(1, 792, bodyLine(100), bodyLine(120), bodyLine(140)),
		tocPage(2, 792, bodyLine(100), bodyLine(120)),
	)

	headings := typographicHeadings(t, document)

	assert.NotNil(t, headings, "a document with no heading gets an empty list, never nil")
	assert.Empty(t, headings, "nothing is invented for a document set in one size")

	empty := typographicHeadings(t, headingDocument())
	assert.NotNil(t, empty, "a document with no pages at all is still an empty list")
	assert.Empty(t, empty)
}

func TestTypographicHeadings_RunsPdfToHtmlOnTheDocument(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte(headingDocument(tocPage(1, 792, bodyLine(100)))), nil
	}}

	extractor, err := pdf.NewTypographicHeadings("book.pdf", stub.runner())
	require.NoError(t, err)

	_, err = extractor.Headings(t.Context())
	require.NoError(t, err)

	// The same invocation the table-of-contents extractor makes: one process, one
	// reading of the layout.
	assert.Equal(t, []string{"pdftohtml -xml -stdout -i book.pdf"}, stub.calls)
}

func TestTypographicHeadings_SurfacesProcessFailure(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return nil, fmt.Errorf("pdftohtml: exit status 1: Couldn't find trailer dictionary")
	}}

	extractor, err := pdf.NewTypographicHeadings("book.pdf", stub.runner())
	require.NoError(t, err)

	_, err = extractor.Headings(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailer dictionary", "the process message must survive the wrap")
}
