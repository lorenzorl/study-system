package domain

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// numberedHeading matches a line that opens with the document's own section
// numbering and then a title: "1. INTRODUCTION", "2.1. TRES Spectra",
// "3.1.1. Anything".
//
// A number is one or two digits because that is what papers print, and the line
// has to be a whole line — the pattern is anchored at both ends — because a
// numbering that shows up mid-sentence is a citation ("as shown in 2.1"), not a
// heading.
var numberedHeading = regexp.MustCompile(`^\s*(\d{1,2}(?:\.\d{1,2})*)\.?\s+(\S.*)$`)

const (
	// maxHeadingRunes and maxHeadingWords are the point past which a line is
	// prose. A heading is a label; anything of body length that carries a number
	// is a numbered list item, a table row or a wrapped sentence.
	maxHeadingRunes = 80
	maxHeadingWords = 12
)

// HeadingsFromPageText returns the numbered headings the page's own text prints,
// in the order they appear. page is the 1-based page the text came from.
//
// This is a candidate generator and not a verdict, and the limit is worth stating
// plainly: text alone cannot tell a heading from a body line, because both are
// just lines, and the page's type is what separates them. What this function can
// do is refuse the lines that are certainly prose — a sentence, a line long
// enough to be body, a number with no title — and report the rest for the
// typographic confirmation to accept or drop. A blank line after a heading is
// deliberately not a signal: pdftotext preserves the gap a typesetter left around
// a heading, and the real corpus sets every heading that way, so treating it as a
// rejection drops all of them. A document
// that has a table of contents never reaches here: its structure comes from the
// index, so the false positives this cannot see never matter.
//
// A page number below 1 is refused rather than trusted, because the only thing
// this function adds to a title is a page, and a page it was not given is a
// citation nothing printed. A page with no numbered heading returns an empty
// slice: no structure is a normal answer, and this feature never invents any.
func HeadingsFromPageText(text string, page int) []Heading {
	headings := []Heading{}
	if page < 1 {
		return headings
	}

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		level, title, ok := numberedHeadingLine(line)
		if !ok {
			continue
		}
		if !readsAsHeading(line, title) {
			continue
		}

		headings = append(headings, Heading{Level: level, Title: title, Page: page})
	}
	return headings
}

// numberedHeadingLine reads one line's nesting level and title, if the line
// carries the document's own numbering.
//
// The level is the number of dot-separated components the document printed, so
// "2.1." is the second level because the paper says so and not because of how it
// is indented.
func numberedHeadingLine(line string) (int, string, bool) {
	match := numberedHeading.FindStringSubmatch(line)
	if match == nil {
		return 0, "", false
	}

	title := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(match[2]), "."))
	if title == "" {
		return 0, "", false
	}

	return strings.Count(match[1], ".") + 1, title, true
}

// readsAsHeading rejects the lines that match the numbering but cannot be a
// heading.
//
// The title rules are what the real corpus taught: a paper's page text prints
// axis ticks ("0.0 0.1 0.2 0.3"), footnote URLs ("https://vizier…") and body
// lines that begin with a reference number ("dex, log g = 4.15 …"), and all of
// them match the numbering. A heading names something, so its title starts with a
// capital letter and holds a letter at all, and it is never a link.
func readsAsHeading(line, title string) bool {
	if utf8.RuneCountInString(strings.TrimSpace(line)) > maxHeadingRunes {
		return false
	}
	if len(strings.Fields(title)) > maxHeadingWords {
		return false
	}
	if !startsAsAName(title) || strings.Contains(title, "://") {
		return false
	}
	return !endsMidSentence(title)
}

// startsAsAName reports whether a title opens with a capital letter and carries
// at least one letter.
//
// The capital is the narrower half of the rule and the one that earns its keep:
// an axis tick and a reference number have no letters, and a wrapped body line
// that begins with a number reads on in lower case. A sentence-case heading
// ("Data reduction") still passes, and a document that sets headings in lower
// case is out of scope — its typography, not its text, would have to say so.
func startsAsAName(title string) bool {
	first := []rune(title)
	if len(first) == 0 {
		return false
	}

	hasLetter := false
	for _, letter := range first {
		if unicode.IsLetter(letter) {
			hasLetter = true
			break
		}
	}

	return hasLetter && unicode.IsUpper(first[0])
}

// sentenceContinuations are the words a title does not end on.
//
// A heading names something, so it closes on the name; a body line wrapped by the
// typesetter closes on whatever word the line happened to reach, which is almost
// always a preposition, an article, a pronoun or an auxiliary. The list is these
// function words, and not "any lowercase word", on purpose: a sentence-case
// heading ("Data reduction") is real structure and dropping it would lose
// sections the document printed.
var sentenceContinuations = func() map[string]bool {
	words := strings.Fields(`
		a an the of to and or but nor in on at by for from with without into onto
		over under than then that this these those which where when while whose
		whom what how why as if so is are was were be been being has have had do
		does did not no its their our we you they it he she us them there here
		about across after before between during through within upon per via each
		every some any all both more most less least other another such only also
		just very can could may might must shall should will would`)

	continuations := make(map[string]bool, len(words))
	for _, word := range words {
		continuations[word] = true
	}
	return continuations
}()

// endsMidSentence reports whether a title ends on the word a sentence was
// interrupted at, which is what a wrapped body line looks like.
func endsMidSentence(title string) bool {
	words := strings.Fields(title)
	if len(words) == 0 {
		return false
	}

	last := words[len(words)-1]
	if strings.ToLower(last) != last {
		return false // an uppercase letter means a name, not a continuation
	}
	return sentenceContinuations[last]
}
