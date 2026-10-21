package app

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

const (
	// offsetWindow is how far a printed page may sit from the PDF page it names.
	// Front matter in roman numerals and a scanned insert are the two things that
	// move it, and both move it by a handful of leaves.
	offsetWindow = 20

	// minimumConfirmed is the share of chapters that must land on their exact page
	// before any section is stored (S4). It is compared as a percentage in integer
	// arithmetic so the boundary cannot drift with a float.
	minimumConfirmed = 95

	// titleFloor is how similar a printed title and a line the page sets large must
	// be to count as the same heading. The floor was measured against the real
	// index: at 0.55 the parts and the chapters are found and the display text in
	// figures is not.
	titleFloor = 0.55

	// titleMinimumRunes is the shortest normalized title the similarity rule will
	// judge. Below it, "of", "1." and "The" appear everywhere and would confirm
	// anything.
	titleMinimumRunes = 4
)

// StructureInput is one document's two readings of its own structure: the printed
// index in printed page numbers, and the lines its pages set large in PDF pages.
//
// The two disagree by one unknown constant — front matter moves the printed
// numbering — which is exactly what this layer infers.
type StructureInput struct {
	// DocumentID is the document every section belongs to.
	DocumentID string

	// PageCount is how many leaves the PDF has, which bounds every range.
	PageCount int

	// TOC is the printed index, from the table-of-contents extractor.
	TOC []domain.Heading

	// Typographic is every line set above the body size, from the typographic
	// reader. It is a confirmer: on the real book only 55 of its 100 candidates
	// are entries the index also prints.
	Typographic []domain.Heading

	// Source names where the entries came from. The zero value means the printed
	// index. domain.SourcePattern means they were read from each page's own text,
	// which changes the gate: such an entry is already on its page, so there is no
	// offset to infer and no second reading that could confirm it. The papers in
	// this corpus are why this exists — their headings are set in bold at the body
	// size, so the typographic reader returns nothing and an index-based gate could
	// never pass.
	Source domain.SectionSource
}

// StructureReport says what BuildSections decided, and why when it decided
// nothing.
//
// It exists because the gate's answer is a count and not a sentence. A document
// with no sections has to be distinguishable from a document nobody looked at,
// both in the acceptance measurement (S4) and in whatever the caller records.
type StructureReport struct {
	// Offset is the inferred printed-page to PDF-page difference.
	Offset int

	// ChaptersNamed is how many entries the index prints at chapter level.
	ChaptersNamed int

	// ChaptersConfirmed is how many of them a page sets large on the exact page
	// the offset predicts.
	ChaptersConfirmed int

	// PartsNamed and PartsConfirmed are the same counts for parts. They are
	// reported and never gated: a part is confirmed by the offset the chapters
	// established.
	PartsNamed     int
	PartsConfirmed int

	// EntriesDropped counts entries that named no page this document has, or that
	// the domain refused. They are not sections and they never count as confirmed.
	EntriesDropped int

	// GatePassed reports whether enough chapters confirmed for any section to be
	// stored.
	GatePassed bool

	// Reason explains an empty result in one sentence, and is empty when sections
	// were produced.
	Reason string
}

// plannedSection is one entry on its way to becoming a Section: the index's
// heading, the PDF page it starts on, and the provenance the confirmation
// settled.
type plannedSection struct {
	heading domain.Heading
	start   int
	source  domain.SectionSource
}

// BuildSections turns a document's printed index and its typography into its
// sections, or into an empty slice when the document cannot be confirmed (S4,
// S10).
//
// It is a pure function of its input and the instant it is handed: no database,
// no PDF, no clock. That is deliberate — the whole decision of whether a document
// has a structure is testable without any of them, and the decision is where this
// feature's safety lives.
func BuildSections(input StructureInput, now time.Time) ([]*domain.Section, StructureReport) {
	report := StructureReport{}
	sections := []*domain.Section{}

	if strings.TrimSpace(input.DocumentID) == "" {
		// Without a document there is nothing a section could belong to, and a
		// report claiming a passing gate over zero sections would be a lie.
		report.Reason = "no document to attach sections to"
		return sections, report
	}

	// A document read from its own page text prints its structure in two or three
	// levels; the deeper ones are the paper's subsections and, in this corpus, the
	// noise the name rules cannot tell from a heading. The index's levels are kept
	// whole, because the index is the document's own claim.
	headings := input.TOC
	pattern := input.Source == domain.SourcePattern
	if pattern {
		headings = topLevels(headings)
	}

	offset := inferOffset(headings, input.Typographic)
	report.Offset = offset

	byPage := indexByPage(input.Typographic)
	confirmed := make([]bool, len(headings))
	matches := make([]*domain.Heading, len(headings))
	for index, entry := range headings {
		matches[index] = confirmingHeading(byPage, entry.Title, entry.Page+offset)
		confirmed[index] = matches[index] != nil

		switch entry.Level {
		case 1:
			report.PartsNamed++
			if confirmed[index] {
				report.PartsConfirmed++
			}
		case 2:
			report.ChaptersNamed++
			if confirmed[index] {
				report.ChaptersConfirmed++
			}
		}
	}

	if pattern {
		// The confirmation above still runs, because a page's typography is what
		// gives an entry the stronger provenance. It cannot be the gate here: these
		// documents set their headings at the body size, so a gate on typography
		// would reject every one of them and leave the fallback dead.
		if !namesStructure(headings) {
			report.Reason = fmt.Sprintf(
				"the page text names %d heading(s), too thin to call a structure", len(headings))
			return sections, report
		}
	} else {
		switch {
		case report.ChaptersNamed == 0:
			// A part or a section alone cannot establish the offset: confirming one
			// would be confirming a guess with its own echo.
			report.Reason = "the index names no chapter, so there is no page to confirm the offset against"
			return sections, report
		case report.ChaptersConfirmed*100 < minimumConfirmed*report.ChaptersNamed:
			report.Reason = fmt.Sprintf(
				"the index names %d chapters and %d of them are set large on the exact page, below the %d%% gate",
				report.ChaptersNamed, report.ChaptersConfirmed, minimumConfirmed)
			return sections, report
		}
	}

	report.GatePassed = true

	entries := make([]plannedSection, 0, len(headings))
	for index, entry := range headings {
		start := entry.Page + offset
		if start < 1 || start > input.PageCount {
			// A page the document does not have is a misread row, not a section:
			// it can own no chunk, and clamping it would attribute one page twice.
			report.EntriesDropped++
			continue
		}
		entries = append(entries, plannedSection{
			heading: titled(entry, byPage[entry.Page+offset]),
			start:   start,
			source:  sourceFor(confirmed[index], pattern),
		})
	}

	// Reading order is the order a chunk's page meets the sections. A stable sort
	// keeps the index's own order where two entries start on the same page at the
	// same level.
	sort.SliceStable(entries, func(earlier, later int) bool {
		if entries[earlier].start != entries[later].start {
			return entries[earlier].start < entries[later].start
		}
		return entries[earlier].heading.Level < entries[later].heading.Level
	})

	ends, parents := endPages(entries, input.PageCount)

	identities := make([]string, len(entries))
	for index := range identities {
		// The identity is minted here and not by the repository because ParentID
		// has to name it before anything is written.
		identities[index] = uuid.NewString()
	}

	for index, entry := range entries {
		parent := ""
		if at := parents[index]; at >= 0 {
			parent = identities[at]
		}

		section, err := domain.NewSection(domain.Section{
			ID:         identities[index],
			DocumentID: input.DocumentID,
			ParentID:   parent,
			Level:      entry.heading.Level,
			Title:      entry.heading.Title,
			StartPage:  entry.start,
			EndPage:    ends[index],
			Source:     entry.source,
		}, now)
		if err != nil {
			// The range is already clamped, so the domain is refusing the row for
			// another reason: an empty title or a level the index never declared.
			// Dropping it and counting it is the only honest answer, and the
			// children are re-parented below rather than left dangling.
			report.EntriesDropped++
			continue
		}
		sections = append(sections, section)
	}

	return reparented(sections), report
}

// endPages returns each entry's last page and the index of its parent, or -1.
//
// The end is the page before the next entry whose level is at most its own, so a
// chapter runs past the sections inside it and a part runs past its chapters. The
// last entry of a level owns the rest of the document. A child is then cut at its
// parent's end: a range outside the parent would attribute pages the parent never
// owned, and the deepest section containing a page is what a citation names.
func endPages(entries []plannedSection, pageCount int) ([]int, []int) {
	ends := make([]int, len(entries))
	parents := make([]int, len(entries))

	for index := range entries {
		parents[index] = -1
		for earlier := index - 1; earlier >= 0; earlier-- {
			if entries[earlier].heading.Level < entries[index].heading.Level {
				parents[index] = earlier
				break
			}
		}

		end := pageCount
		for later := index + 1; later < len(entries); later++ {
			if entries[later].heading.Level <= entries[index].heading.Level {
				end = entries[later].start - 1
				break
			}
		}
		ends[index] = end
	}

	for index := range entries {
		// Parents precede their children in reading order, so the parent's end is
		// already cut by its own parent when the child is reached.
		if parent := parents[index]; parent >= 0 && ends[parent] < ends[index] {
			ends[index] = ends[parent]
		}

		// Two entries can start on one page — a closing section printed where the
		// next chapter opens — and a range can never be empty or inverted.
		ends[index] = max(min(ends[index], pageCount), entries[index].start)
	}

	return ends, parents
}

// reparented re-points a child whose parent the domain refused at the nearest
// ancestor that survived, so no section ever names a row that was not written.
func reparented(sections []*domain.Section) []*domain.Section {
	kept := make(map[string]bool, len(sections))
	for _, section := range sections {
		kept[section.ID] = true
	}

	for _, section := range sections {
		if section.ParentID == "" || kept[section.ParentID] {
			continue
		}
		// The refused row is gone. The section becomes top-level rather than being
		// re-pointed at a guessed ancestor: a wrong parent is worse than none, and
		// SectionAt answers by page, never through the tree.
		section.ParentID = ""
	}
	return sections
}

// inferOffset finds the constant that turns the index's printed pages into PDF
// pages: the one where the most entries land on a page that sets their title
// large.
func inferOffset(entries []domain.Heading, typographic []domain.Heading) int {
	byPage := indexByPage(typographic)

	bestOffset, bestMatches := 0, -1
	for _, offset := range offsetCandidates() {
		matches := 0
		for _, entry := range entries {
			if confirmingHeading(byPage, entry.Title, entry.Page+offset) != nil {
				matches++
			}
		}

		// The candidate order is the tie-break, so a strict comparison keeps the
		// first of equals and the answer is deterministic.
		if matches > bestMatches {
			bestOffset, bestMatches = offset, matches
		}
	}
	return bestOffset
}

// offsetCandidates lists the offsets to try in the order that breaks a tie: zero
// first, then one page out in either direction, and so on.
//
// A smaller correction is the likelier one, and within a distance the negative
// comes first because that is what a document with front matter in roman numerals
// needs: the printed numbering starts after the leaves the front matter used.
func offsetCandidates() []int {
	offsets := []int{0}
	for distance := 1; distance <= offsetWindow; distance++ {
		offsets = append(offsets, -distance, distance)
	}
	return offsets
}

// indexByPage groups the headings by the PDF page they sit on, so one
// confirmation is a lookup and one offset is a walk of the index.
func indexByPage(headings []domain.Heading) map[int][]domain.Heading {
	byPage := map[int][]domain.Heading{}
	for _, heading := range headings {
		byPage[heading.Page] = append(byPage[heading.Page], heading)
	}
	return byPage
}

// confirmingHeading returns the line that confirms the printed entry, or nil.
func confirmingHeading(byPage map[int][]domain.Heading, title string, page int) *domain.Heading {
	for index, heading := range byPage[page] {
		if titleSimilarity(title, heading.Title) >= titleFloor {
			return &byPage[page][index]
		}
	}
	return nil
}

// titled returns the entry with the print's own spacing when a line the page sets
// large carries the same characters.
//
// It is not a license for the typography to rename a section: the index names
// every section (S4), and the letters are compared so only spacing and
// punctuation can change. What it fixes is the one thing the index cannot report
// — a letter-spaced row ("P u t t in g") whose word boundaries are unrecoverable
// — and it is what keeps a citation reading "Part II. Building Blocks of a
// Model-Driven Design" instead of "… Blocks ofa …".
func titled(entry domain.Heading, page []domain.Heading) domain.Heading {
	want := normalizedTitle(entry.Title)
	for _, candidate := range page {
		// Any line on the confirmed page, not only the one that confirmed the
		// entry: a chapter page sets its number and its title as separate lines, and
		// the number alone is a full similarity match by containment.
		if normalizedTitle(candidate.Title) == want {
			entry.Title = candidate.Title
			return entry
		}
	}
	return entry
}

// sourceFor records provenance: an entry only the index names is the index's
// claim, and one a page's typography confirms is mixed (S4).
func sourceFor(confirmed, pattern bool) domain.SectionSource {
	if confirmed {
		return domain.SourceMixed
	}
	if pattern {
		return domain.SourcePattern
	}
	return domain.SourceTOC
}

// topLevels keeps the entries a page-text reading can be trusted with: the
// document's numbered sections and their first level of children.
func topLevels(entries []domain.Heading) []domain.Heading {
	kept := make([]domain.Heading, 0, len(entries))
	for _, entry := range entries {
		if entry.Level <= 2 {
			kept = append(kept, entry)
		}
	}
	return kept
}

// namesStructure reports whether a page-text reading looks like a structure at
// all: at least two entries, one of them the document's own first level.
//
// One entry is a numbered list item, and two entries with no first level are a
// table's rows; neither is worth storing as a citation.
func namesStructure(entries []domain.Heading) bool {
	if len(entries) < 2 {
		return false
	}
	for _, entry := range entries {
		if entry.Level == 1 {
			return true
		}
	}
	return false
}

// titleSimilarity scores how well a line the page sets large confirms a printed
// entry, on the index's own terms.
//
// Spacing and punctuation are layout, not content, so only letters and digits
// count — which is also what makes the printed title usable at all: the index
// letter-spaces some of its rows into "P u t t in g". A title the page adds to or
// wraps around the index's words ("Part I. Putting…" for "Putting…") is a full
// match rather than a partial one.
func titleSimilarity(printed, set string) float64 {
	left, right := normalizedTitle(printed), normalizedTitle(set)

	if utf8.RuneCountInString(left) < titleMinimumRunes || utf8.RuneCountInString(right) < titleMinimumRunes {
		return 0
	}
	if strings.Contains(left, right) || strings.Contains(right, left) {
		return 1
	}
	return sharedRuneRatio(left, right)
}

// normalizedTitle reduces a title to the characters that carry meaning:
// lower-case letters and digits, with combining marks dropped.
//
// NFKD folding would also turn a precomposed "á" into "a", and it is not
// available without a new dependency. Keeping it is harmless here: both titles
// come from the same print, so the same glyph arrives on both sides and compares
// equal to itself.
func normalizedTitle(title string) string {
	var normalized strings.Builder

	for _, letter := range strings.ToLower(title) {
		switch {
		case unicode.Is(unicode.Mn, letter):
			continue
		case unicode.IsLetter(letter), unicode.IsDigit(letter):
			normalized.WriteRune(letter)
		}
	}
	return normalized.String()
}

// sharedRuneRatio is the share of characters two titles hold in common in order:
// 2M/T, where M is the longest common subsequence and T the total length.
//
// It is the ratio the feasibility probe's 0.55 floor was measured against, so the
// threshold means here what it meant there.
func sharedRuneRatio(left, right string) float64 {
	outer, inner := []rune(left), []rune(right)

	previous := make([]int, len(inner)+1)
	current := make([]int, len(inner)+1)

	for _, leftRune := range outer {
		for index, rightRune := range inner {
			switch {
			case leftRune == rightRune:
				current[index+1] = previous[index] + 1
			case previous[index+1] >= current[index]:
				current[index+1] = previous[index+1]
			default:
				current[index+1] = current[index]
			}
		}
		previous, current = current, previous
	}

	return 2 * float64(previous[len(inner)]) / float64(len(outer)+len(inner))
}
