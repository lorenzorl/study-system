package pdf

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// TOCExtractor reads a document's printed table of contents from pdftohtml's
// XML layout output.
//
// It reports one source of structure and nothing else: the entries the index
// prints, in reading order. It knows nothing about page ranges, typographic
// confirmation or persistence, because those decide whether an entry is right,
// and a wrong section is worse than no section at all.
type TOCExtractor struct {
	pdfPath string
	runner  Runner
}

// NewTOCExtractor builds a table-of-contents extractor over pdfPath.
//
// A nil runner selects the real poppler binaries and verifies pdftohtml is
// installed, so a missing tool fails here rather than on the first index page.
func NewTOCExtractor(pdfPath string, runner Runner) (*TOCExtractor, error) {
	resolved, err := resolveRunner(runner, "pdftohtml")
	if err != nil {
		return nil, err
	}
	return &TOCExtractor{pdfPath: pdfPath, runner: resolved}, nil
}

// Headings returns every entry of the document's printed table of contents.
//
// A document with no index returns an empty slice and a nil error: an absent
// index is a normal document, not a failure, and inventing structure for it
// would be worse than reporting none.
func (t *TOCExtractor) Headings(ctx context.Context) ([]domain.Heading, error) {
	output, err := t.runner(ctx, "pdftohtml", "-xml", "-stdout", "-i", t.pdfPath)
	if err != nil {
		return nil, fmt.Errorf("read table of contents of %q: %w", t.pdfPath, err)
	}

	headings, err := parseTOC(output)
	if err != nil {
		return nil, fmt.Errorf("read table of contents of %q: %w", t.pdfPath, err)
	}
	return headings, nil
}

const (
	// baselineTolerance is how far apart two runs' baselines can sit and still
	// be the same line.
	baselineTolerance = 3

	// leaderLinesPerIndex is how many leader-dot rows make a page an index page.
	leaderLinesPerIndex = 3

	// labelColumnLeft is the left edge of the index's unindented label column.
	// A row whose leftmost run starts there is a part, whatever its text says.
	labelColumnLeft = 140

	// footerBand is the fraction of the page height below which a running footer
	// lives rather than index rows.
	footerBand = 0.9
)

var (
	xmlAttribute = regexp.MustCompile(`(\w+)="([^"]*)"`)
	xmlInnerTag  = regexp.MustCompile(`<[^>]+>`)
	leaderDots   = regexp.MustCompile(`\.{3,}`)
	numberedRow  = regexp.MustCompile(`^\d+\.\s*`)
	wordGap      = regexp.MustCompile(`\s{2,}`)
)

// tocRun is one <text> element: a styled fragment of text at one baseline.
//
// font is the id of the <fontspec> the run's size comes from, and size is that
// spec's point size. The index rules read neither — position and text decide an
// entry — while the typographic reading is nothing but size, so one scan of the
// XML serves both.
type tocRun struct {
	top  int
	left int
	text string
	font int
	size int
}

// tocPage is one <page> element's runs, in the order poppler wrote them.
type tocPage struct {
	number int
	height int
	runs   []tocRun
}

// tocLine is the runs that share a baseline, joined left to right.
type tocLine struct {
	top  int
	left int
	text string

	// size is the largest size among the line's runs: a small-caps title sets its
	// first letter one size and the rest another, and the line is as large as the
	// largest thing on it.
	size int
}

// parseTOC reads pdftohtml's XML into the index entries it prints.
//
// The XML is scanned line by line because poppler writes one element per line:
// a tree parser would buy ordering this does not need. Font specs are skipped on
// purpose — the index rules read position and text only, so the sizes they
// declare belong to the typography confirmation, not here.
func parseTOC(output []byte) ([]domain.Heading, error) {
	pages, err := parseTOCPages(output)
	if err != nil {
		return nil, err
	}

	headings := []domain.Heading{}
	printed := map[string]bool{}
	for _, page := range pages {
		for _, heading := range indexHeadings(page) {
			// A title the index prints twice ON THE SAME PAGE is one section, not
			// two: that is a row repeated across a page break. The same title on
			// another page is a different section — "Summary" closes most
			// chapters — and dropping it would lose real structure.
			key := strconv.Itoa(heading.Level) + "|" + strconv.Itoa(heading.Page) +
				"|" + strings.ToLower(heading.Title)
			if printed[key] {
				continue
			}
			printed[key] = true
			headings = append(headings, heading)
		}
	}
	return headings, nil
}

// parseTOCPages reads the <page> and <text> elements in the order poppler wrote
// them, which is the document's own reading order.
func parseTOCPages(output []byte) ([]tocPage, error) {
	pages := []tocPage{}
	sizes := map[int]int{}
	open := -1

	for _, raw := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(raw)

		switch {
		case strings.HasPrefix(line, "<fontspec "):
			// A font spec the file cannot describe is skipped rather than fatal: the
			// index rules do not need sizes, and a run whose size is unknown is a run
			// the typographic reading ignores.
			attributes := parseAttributes(line)
			id, idErr := strconv.Atoi(attributes["id"])
			size, sizeErr := strconv.Atoi(attributes["size"])
			if idErr == nil && sizeErr == nil {
				sizes[id] = size
			}

		case strings.HasPrefix(line, "<page "):
			attributes := parseAttributes(line)
			height, err := numericAttribute(attributes, "height", line)
			if err != nil {
				return nil, err
			}
			// The page number is read leniently and never validated: the index reads
			// pages in the order poppler wrote them, and only the typographic
			// reading, which reports PDF pages, needs the number.
			number, _ := strconv.Atoi(attributes["number"])
			pages = append(pages, tocPage{number: number, height: height})
			open = len(pages) - 1

		case strings.HasPrefix(line, "<text ") && open >= 0:
			attributes := parseAttributes(line)
			top, err := numericAttribute(attributes, "top", line)
			if err != nil {
				return nil, err
			}
			left, err := numericAttribute(attributes, "left", line)
			if err != nil {
				return nil, err
			}
			font := -1
			if id, err := strconv.Atoi(attributes["font"]); err == nil {
				font = id
			}
			pages[open].runs = append(pages[open].runs,
				tocRun{top: top, left: left, text: innerText(line), font: font})
		}
	}

	// Sizes are resolved once the whole file has been read: font specs describe the
	// document and poppler declares them first, but nothing in the format promises
	// that, and a second pass costs one loop.
	for index := range pages {
		for at := range pages[index].runs {
			pages[index].runs[at].size = sizes[pages[index].runs[at].font]
		}
	}

	return pages, nil
}

// indexHeadings returns the entries of one page, or nothing when the page is not
// part of the index.
//
// The page's title is deliberately not the test. An index that spans pages
// prints "Table of Contents" on its first page only, so keying on the title
// keeps that page and silently drops the rest of the entries.
func indexHeadings(page tocPage) []domain.Heading {
	lines := clusterRuns(page.runs)
	if countLeaderLines(lines) < leaderLinesPerIndex {
		return nil
	}

	headings := []domain.Heading{}
	for _, line := range lines {
		// A running footer sits below the text block; leader dots there are
		// decoration, not an entry.
		if float64(line.top) > float64(page.height)*footerBand {
			continue
		}

		draft, ok := parseIndexRow(line)
		if !ok {
			continue
		}
		heading, err := domain.NewHeading(draft)
		if err != nil {
			continue // a row the domain cannot represent is not a heading
		}
		headings = append(headings, heading)
	}
	return headings
}

// parseIndexRow reads one leader-dot row: the title before the first run of
// dots, and the printed page after it.
func parseIndexRow(line tocLine) (domain.Heading, bool) {
	parts := leaderDots.Split(line.text, 2)
	if len(parts) != 2 {
		return domain.Heading{}, false
	}

	page := printedPage(parts[1])
	title := despaced(strings.Trim(parts[0], " .:"))
	if page < 1 || title == "" {
		return domain.Heading{}, false
	}

	level, title := rowLevel(line.left, title)
	return domain.Heading{Level: level, Title: title, Page: page}, true
}

// printedPage reads the printed page number from everything after the leader
// dots.
//
// The print letter-spaces a multi-digit number ("1 2" for 12), so the page is the
// digits joined rather than the first number the text yields.
func printedPage(afterDots string) int {
	digits := strings.Map(func(letter rune) rune {
		if letter >= '0' && letter <= '9' {
			return letter
		}
		return -1
	}, afterDots)
	if digits == "" {
		return 0
	}

	page, err := strconv.Atoi(digits)
	if err != nil {
		return 0 // more digits than a page number can hold
	}
	return page
}

// rowLevel reads the nesting the index's own layout declares: a part sits in the
// far-left label column or says "Part", a numbered row is a chapter and loses its
// number, and everything else is a section.
func rowLevel(left int, title string) (int, string) {
	switch {
	case isPartRow(title), left <= labelColumnLeft:
		return 1, title
	case numberedRow.MatchString(title):
		return 2, numberedRow.ReplaceAllString(title, "")
	default:
		return 3, title
	}
}

// isPartRow reports whether a title is the "Part" label. The boundary is explicit
// so a title such as "Partnership" is not read as a part.
func isPartRow(title string) bool {
	const prefix = "Part"

	if !strings.HasPrefix(title, prefix) {
		return false
	}
	rest := title[len(prefix):]
	if rest == "" {
		return true
	}

	head, _ := utf8.DecodeRuneInString(rest)
	return !isWordRune(head)
}

// isWordRune matches the characters a regular expression's \b treats as word
// characters.
func isWordRune(letter rune) bool {
	return letter == '_' || unicode.IsLetter(letter) || unicode.IsDigit(letter)
}

// despaced joins the single spaces a PDF inserts between the glyphs of a
// letter-spaced title.
//
// A letter-spaced title is set one glyph per run — "P u t t in g  t h e  Do m a
// in" — with a single space inside a word and a double space between words, so
// the double space is the word boundary and a word made only of very short
// glyphs is a word that was spaced out, not several words.
//
// One limit is worth naming: when the typesetter also collapsed a word boundary
// to a single space ("o f a" for "of a"), no run-level geometry can tell it apart
// from glyph spacing, so the words come out welded ("ofa"). The title is then a
// matching key and not a display string; the page's own heading, which is not
// letter-spaced, is what supplies the title once it confirms the entry.
func despaced(title string) string {
	words := wordGap.Split(strings.TrimSpace(title), -1)

	for index, word := range words {
		words[index] = despacedGroup(word)
	}

	return strings.Join(words, " ")
}

// despacedGroup joins the glyphs of one word group.
//
// A token that ends a label ("I.", "II.") stays its own word: the space after it
// separates the label from the title that follows, and joining across it produces
// "I.Putting" — the way a real index prints "Part I. Putting the Domain Model to
// Work".
func despacedGroup(word string) string {
	glyphs := strings.Fields(word)
	if len(glyphs) < 2 || !mostlyShort(glyphs) {
		return strings.Join(glyphs, " ")
	}

	words := []string{}
	current := []string{}
	for _, glyph := range glyphs {
		current = append(current, glyph)
		if strings.HasSuffix(glyph, ".") {
			words = append(words, strings.Join(current, ""))
			current = nil
		}
	}
	if len(current) > 0 {
		words = append(words, strings.Join(current, ""))
	}

	return strings.Join(words, " ")
}

// mostlyShort reports whether most of a group's tokens hold one or two
// characters, which is what a letter-spaced word looks like once its glyphs are
// separated by spaces.
//
// The test is a majority and not "any short token": a title such as "Read the
// Book" has a two-character word and is not letter-spaced, so joining it would
// weld three words into one. A group that is mostly short glyphs is a word the
// typesetter spaced out, and its glyphs belong together whatever their lengths.
func mostlyShort(glyphs []string) bool {
	short := 0
	for _, glyph := range glyphs {
		if utf8.RuneCountInString(glyph) <= 2 {
			short++
		}
	}
	return short*2 > len(glyphs)
}

// clusterRuns groups the runs that share a baseline, in reading order.
//
// Each run is compared against the first run of its line, never against the
// line's minimum: a drop cap sits a couple of points above the word it belongs
// to, and re-anchoring the line on the minimum would drift the anchor and push
// the rest of that word onto a line of its own.
func clusterRuns(runs []tocRun) []tocLine {
	content := make([]tocRun, 0, len(runs))
	for _, run := range runs {
		// A whitespace-only run is the PDF's invisible margin spacer: it carries
		// no text, and its position would otherwise become the line's anchor.
		if strings.TrimSpace(run.text) == "" {
			continue
		}
		content = append(content, run)
	}

	sort.SliceStable(content, func(earlier, later int) bool {
		if content[earlier].top != content[later].top {
			return content[earlier].top < content[later].top
		}
		return content[earlier].left < content[later].left
	})

	var anchors []int
	var grouped [][]tocRun

	for _, run := range content {
		at := -1
		for index, anchor := range anchors {
			if within(run.top, anchor) {
				at = index
				break
			}
		}
		if at < 0 {
			anchors = append(anchors, run.top)
			grouped = append(grouped, nil)
			at = len(anchors) - 1
		}
		grouped[at] = append(grouped[at], run)
	}

	lines := make([]tocLine, 0, len(grouped))
	for _, group := range grouped {
		sort.SliceStable(group, func(earlier, later int) bool {
			return group[earlier].left < group[later].left
		})

		var text strings.Builder
		line := tocLine{top: group[0].top, left: group[0].left}
		for _, run := range group {
			text.WriteString(run.text)
			if run.size > line.size {
				line.size = run.size
			}
		}
		line.text = text.String()
		lines = append(lines, line)
	}
	return lines
}

// within reports whether two baselines are close enough to be one line.
func within(top, anchor int) bool {
	distance := top - anchor
	if distance < 0 {
		distance = -distance
	}
	return distance <= baselineTolerance
}

// countLeaderLines counts the rows carrying a leader-dot run, which is what makes
// a page an index page.
func countLeaderLines(lines []tocLine) int {
	count := 0
	for _, line := range lines {
		if leaderDots.MatchString(line.text) {
			count++
		}
	}
	return count
}

// innerText returns a <text> element's content with its nested tags removed and
// its entities decoded, because poppler wraps emphasised words in <b> and writes
// "&amp;" for an ampersand.
func innerText(line string) string {
	start := strings.Index(line, ">")
	end := strings.LastIndex(line, "</text>")
	if start < 0 || end <= start {
		return ""
	}

	return html.UnescapeString(xmlInnerTag.ReplaceAllString(line[start+1:end], ""))
}

// parseAttributes reads an element's name="value" pairs.
func parseAttributes(line string) map[string]string {
	attributes := map[string]string{}
	for _, pair := range xmlAttribute.FindAllStringSubmatch(line, -1) {
		attributes[pair[1]] = pair[2]
	}
	return attributes
}

// numericAttribute reads a whole-point coordinate.
//
// A missing or unparseable value is an error naming the element, never a silent
// skip: a dropped run shortens a title, and a shorter title looks exactly like a
// title the index printed that way.
func numericAttribute(attributes map[string]string, name, line string) (int, error) {
	value, ok := attributes[name]
	if !ok {
		return 0, fmt.Errorf("pdftohtml element %q has no %s attribute", line, name)
	}

	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("unparseable pdftohtml %s in element %q: %w", name, line, err)
	}
	return number, nil
}
