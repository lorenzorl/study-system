package pdf

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// TypographicHeadings reads the lines a document sets larger than its body text.
//
// It is a confirmer, not a source: on the real book it reports 100 candidates in
// 63 pages, of which 55 are entries the index also prints. The rest is a title
// page, the index page itself and display text inside figures. So a candidate
// here is never an attribution on its own — it is what tells the printed index
// where its own page numbers land.
type TypographicHeadings struct {
	pdfPath string
	runner  Runner
}

// NewTypographicHeadings builds a typographic reader over pdfPath.
//
// A nil runner selects the real poppler binaries and verifies pdftohtml is
// installed, exactly as the table-of-contents extractor does: one reading of the
// layout, one tool.
func NewTypographicHeadings(pdfPath string, runner Runner) (*TypographicHeadings, error) {
	resolved, err := resolveRunner(runner, "pdftohtml")
	if err != nil {
		return nil, err
	}
	return &TypographicHeadings{pdfPath: pdfPath, runner: resolved}, nil
}

// Headings returns every line the document sets above its body size, in reading
// order.
//
// Each heading carries the PDF page it was found on, because that is the number
// this layer knows: the printed number beside an index entry is a different
// claim, and binding the two is the confirmation step's job.
func (t *TypographicHeadings) Headings(ctx context.Context) ([]domain.Heading, error) {
	output, err := t.runner(ctx, "pdftohtml", "-xml", "-stdout", "-i", t.pdfPath)
	if err != nil {
		return nil, fmt.Errorf("read the typography of %q: %w", t.pdfPath, err)
	}

	headings, err := parseTypographicHeadings(output)
	if err != nil {
		return nil, fmt.Errorf("read the typography of %q: %w", t.pdfPath, err)
	}
	return headings, nil
}

const (
	// headingDelta is how many points above the body size a line must be set to
	// be a heading. Measured on the real book: the body is 15, parts and chapters
	// land at 21, 27 and 30, and a body line never clears 19.
	headingDelta = 4

	// maxHeadingRunes bounds a title. A display line longer than this is a line of
	// body text set large — a lead paragraph, a figure caption — and not something
	// a reader could cite as a section.
	maxHeadingRunes = 120
)

// parseTypographicHeadings reads pdftohtml's XML into the lines set above the
// document's body size.
func parseTypographicHeadings(output []byte) ([]domain.Heading, error) {
	pages, err := parseTOCPages(output)
	if err != nil {
		return nil, err
	}

	body := bodySize(pages)
	headings := []domain.Heading{}

	for _, page := range pages {
		for _, line := range clusterRuns(page.runs) {
			if !isHeadingLine(line, page.height, body) {
				continue
			}

			// Level 1 for every candidate: typography declares a size, never a
			// nesting. The index supplies the levels and this list only confirms
			// titles on pages, so a level here would be a guess.
			heading, err := domain.NewHeading(domain.Heading{
				Level: 1,
				Title: despaced(strings.TrimSpace(line.text)),
				Page:  page.number,
			})
			if err != nil {
				continue // a line the domain cannot represent is not a heading
			}
			headings = append(headings, heading)
		}
	}

	return headings, nil
}

// bodySize returns the document's body size: the font size carrying the most
// characters in the document.
//
// It is measured over the whole document and never per page. A page can be almost
// entirely display: a title page, a figure, a plate. A per-page answer would call
// that page's caption the body and promote the document's real body text to a
// heading on the pages around it.
func bodySize(pages []tocPage) int {
	weight := map[int]int{}

	for _, page := range pages {
		for _, run := range page.runs {
			// A run in the footer band is a running head, a page number or a rule:
			// printed on every page, and not the body of any of them.
			if run.size <= 0 || inFooter(run.top, page.height) {
				continue
			}
			weight[run.size] += utf8.RuneCountInString(strings.TrimSpace(run.text))
		}
	}

	body, carried := 0, 0
	for size, characters := range weight {
		// The larger size wins a tie. On a real book the character count decides;
		// the tie-break only has to be deterministic, and map order is not.
		if characters > carried || (characters == carried && size > body) {
			body, carried = size, characters
		}
	}
	return body
}

// isHeadingLine reports whether a clustered line reads as a heading.
//
// The four tests are all necessary. Size above the body is what makes a line a
// heading at all; the footer band is where a running footer lives; the rune bound
// keeps a lead paragraph set large from becoming a title nobody could cite; and
// the letter test keeps a display row of numbers — an axis, a numeric table — out
// of a list that would otherwise confirm an index entry by its digits alone.
func isHeadingLine(line tocLine, height, body int) bool {
	text := strings.TrimSpace(line.text)

	if text == "" || utf8.RuneCountInString(text) > maxHeadingRunes {
		return false
	}
	if line.size < body+headingDelta || inFooter(line.top, height) {
		return false
	}

	for _, letter := range text {
		if unicode.IsLetter(letter) {
			return true
		}
	}
	return false
}

// inFooter reports whether a y coordinate falls in the band a running footer
// lives in.
//
// The band is the index extractor's own, so both readings of one XML agree on
// where a page's text block ends.
func inFooter(top, height int) bool {
	return float64(top) > float64(height)*footerBand
}
