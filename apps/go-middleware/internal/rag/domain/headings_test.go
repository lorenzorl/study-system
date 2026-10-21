package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// The strings below are the ones two research papers actually print in their
// text layer, so the tests fail on the paper's real lines and not on a shape
// invented for the test.
func TestHeadingsFromPageText_ReadsTheNumberingThePagePrints(t *testing.T) {
	text := strings.Join([]string{
		"1. INTRODUCTION",
		"The study of brown dwarfs has a long history in the literature.",
		"",
		"2. DATA",
		"The survey sample is described in the following subsections.",
		"  2.1. TRES Spectra",
		"Spectra were reduced with the standard pipeline.",
		"2.1.1. Wavelength coverage",
		"Coverage runs from 3500 to 10000 angstroms.",
	}, "\n")

	headings := domain.HeadingsFromPageText(text, 4)

	assert.Equal(t, []domain.Heading{
		{Level: 1, Title: "INTRODUCTION", Page: 4},
		{Level: 1, Title: "DATA", Page: 4},
		{Level: 2, Title: "TRES Spectra", Page: 4},
		{Level: 3, Title: "Wavelength coverage", Page: 4},
	}, headings)
}

func TestHeadingsFromPageText_KeepsThePrintedPunctuationButNotATrailingPeriod(t *testing.T) {
	text := strings.Join([]string{
		"3.1. Sample Selection.",
		"Objects were kept only when their spectrum was clean.",
		"3.2. Sample Selection: revisiting",
		"The revision follows the first selection.",
	}, "\n")

	headings := domain.HeadingsFromPageText(text, 7)

	assert.Equal(t, []domain.Heading{
		{Level: 2, Title: "Sample Selection", Page: 7},
		{Level: 2, Title: "Sample Selection: revisiting", Page: 7},
	}, headings)
}

func TestHeadingsFromPageText_RejectsALineThatIsBodyText(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			// The real paper's worst case: it matches the numbering rule and is
			// a sentence the typesetter wrapped.
			name: "wrapped sentence ending in a function word",
			line: "13 MJup . We restricted our sample to those where the",
		},
		{
			name: "more runes than a heading fits in",
			line: "1. " + strings.Repeat("Extraordinarily ", 6),
		},
		{
			name: "more words than a heading fits in",
			line: "2.1. a b c d e f g h i j k l m",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			text := test.line + "\n" + "The survey sample is described in the following subsections."

			headings := domain.HeadingsFromPageText(text, 12)

			assert.Empty(t, headings)
			assert.NotNil(t, headings)
		})
	}
}

// The real corpus sets a heading on its own line with a blank line before the
// body, because pdftotext preserves the gap the typesetter left around it. A
// blank line is therefore not a rejection: treating it as one drops every heading
// the document prints.
func TestHeadingsFromPageText_FindsAHeadingFollowedByABlankLine(t *testing.T) {
	text := "1. INTRODUCTION\n\nThe Milky Way comprises three broad stellar populations: the thin disk, the thick disk, and the halo."

	assert.Equal(t, []domain.Heading{
		{Level: 1, Title: "INTRODUCTION", Page: 3},
	}, domain.HeadingsFromPageText(text, 3))
}

func TestHeadingsFromPageText_FindsAHeadingAtTheFootOfThePage(t *testing.T) {
	text := "The sample is described above.\n\n2. DATA\n"

	assert.Equal(t, []domain.Heading{
		{Level: 1, Title: "DATA", Page: 3},
	}, domain.HeadingsFromPageText(text, 3))
}

// The real corpus prints axis ticks, footnote links and body lines that open with
// a reference number, and every one of them matches the numbering rule.
func TestHeadingsFromPageText_RejectsARowThatIsNotAName(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"axis ticks", "1 0.0 0.1 0.2 0.3"},
		{"a footnote link", "1 https://vizier.cds.unistra.fr/viz-bin/VizieR-3"},
		{"a body line beginning with a number", "1. dex, log g = 4.15 ± 0.09. Yee et al. 2025 found"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			text := test.line + "\n\nThe body that follows."

			assert.Empty(t, domain.HeadingsFromPageText(text, 5))
		})
	}
}

func TestHeadingsFromPageText_RejectsAHeadingWithNoTitle(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"number and dot only", "1."},
		{"number and trailing spaces", "2.1.    "},
		{"number and a bare period", "2.1. ."},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			text := test.line + "\n" + "The text that follows the number."

			assert.Empty(t, domain.HeadingsFromPageText(text, 5))
		})
	}
}

func TestHeadingsFromPageText_RefusesToAttributeAPageBelowOne(t *testing.T) {
	text := "1. INTRODUCTION\nThe text that follows the heading."

	for _, page := range []int{0, -7} {
		headings := domain.HeadingsFromPageText(text, page)

		assert.Empty(t, headings, "page %d", page)
		assert.NotNil(t, headings, "page %d", page)
	}
}

func TestHeadingsFromPageText_ReturnsAnEmptySliceForAPageWithoutNumberedHeadings(t *testing.T) {
	text := strings.Join([]string{
		"The survey sample is described in the following subsections.",
		"Objects were kept only when their spectrum was clean.",
		"",
		"We restricted our sample to those where the survey had spectra.",
	}, "\n")

	headings := domain.HeadingsFromPageText(text, 9)

	assert.Empty(t, headings)
	assert.NotNil(t, headings)
}

func TestHeadingsFromPageText_KeepsSeveralHeadingsInTheOrderTheyAppear(t *testing.T) {
	text := strings.Join([]string{
		"4. RESULTS",
		"The sample sizes are listed per subsample.",
		"4.1. Spectra",
		"Each object contributes one spectrum.",
		"4.2. Photometry",
		"Each object contributes one light curve.",
		"4.2.1. Calibration",
		"Calibration follows the standard recipe.",
	}, "\n")

	headings := domain.HeadingsFromPageText(text, 11)

	assert.Equal(t, []string{
		"RESULTS",
		"Spectra",
		"Photometry",
		"Calibration",
	}, titles(headings))
	assert.Equal(t, []int{1, 2, 2, 3}, levels(headings))
}

func titles(headings []domain.Heading) []string {
	titles := make([]string, 0, len(headings))
	for _, heading := range headings {
		titles = append(titles, heading.Title)
	}
	return titles
}

func levels(headings []domain.Heading) []int {
	levels := make([]int, 0, len(headings))
	for _, heading := range headings {
		levels = append(levels, heading.Level)
	}
	return levels
}
