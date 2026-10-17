package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Chunking turns one page's stored text into the units a lexical index searches.
//
// It is a pure function of the text, the page number, and the options, for the
// same reason RenderMarkdown is: the boundary rules are the part that is easy to
// get wrong, and a rule that lives in a pure function can be tested without a
// database, a file, or a model call.

// Chunking sizes.
const (
	// DefaultChunkTargetChars is the size one chunk's text aims for, ≈300 tokens
	// (S4). Characters are counted, not bytes, so a page of Spanish costs the
	// same number of "tokens" as the same page of English.
	DefaultChunkTargetChars = 1200

	// DefaultChunkOverlapChars is how much of the previous chunk is repeated at
	// the start of the next one: 15% of the target (S4). The overlap is what
	// keeps a sentence that straddles a boundary findable from either side.
	DefaultChunkOverlapChars = DefaultChunkTargetChars * 15 / 100
)

// ChunkOptions controls ChunkPage.
//
// A zero or negative field means the default, so ChunkOptions{} is the tuned
// configuration and a caller or a test names only the field it varies.
type ChunkOptions struct {
	// TargetChars is the size a chunk's accumulated paragraphs aim for.
	TargetChars int

	// OverlapChars is roughly how many trailing characters of the previous chunk
	// are repeated at the start of the next one, snapped to a word boundary.
	OverlapChars int
}

// Chunk is one indexable unit of one page's text.
type Chunk struct {
	// PageNumber is the 1-based page the text came from. A chunk belongs to
	// exactly one page, which is what makes a search result citable.
	PageNumber int

	// Ordinal is the chunk's 0-based position within its page, ascending.
	Ordinal int

	// Text is the chunk's text: the accumulated paragraphs of this chunk plus
	// the overlap carried from the previous chunk of the same page.
	Text string
}

// paragraphSeparator joins the paragraphs packed into one chunk. It is the same
// blank line the text was split on, so a chunk reads like the page it came from.
const paragraphSeparator = "\n\n"

// ChunkPage splits one page's text into chunks.
//
// A page shorter than the target produces one chunk rather than nothing, so a
// short page is searchable at all. A whitespace-only page produces none, because
// indexing a chunk of spaces would only add a row that can never match. A page
// number below 1 produces none too: the schema's CHECK refuses it, and inventing
// chunks that cannot cite a page would push an impossible value to the caller.
//
// The page's own text is never merged with another page's: a chunk that spanned
// two pages could not name the page a reader should open.
//
// A chunk is at most TargetChars of the page's own text; the carry from the
// previous chunk is prepended on top of that and is bounded by OverlapChars. The
// alternative reading — counting the carry inside the target — would either have
// to split a paragraph that fits the target on its own or emit a chunk well below
// the size the target asks for.
func ChunkPage(text string, pageNumber int, options ChunkOptions) []Chunk {
	options = options.withDefaults()
	if pageNumber < 1 {
		return nil
	}

	paragraphs := splitIntoParagraphs(text)
	if len(paragraphs) == 0 {
		return nil
	}

	// A paragraph that is longer than the target cannot be accumulated with
	// anything; it is cut at word boundaries first, so the merge below only ever
	// sees units that fit.
	units := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if runeLength(paragraph) > options.TargetChars {
			units = append(units, splitAtWordBoundaries(paragraph, options.TargetChars)...)
			continue
		}
		units = append(units, paragraph)
	}

	texts := mergeIntoChunks(units, options.TargetChars)
	texts = carryOverlaps(texts, options.OverlapChars)

	chunks := make([]Chunk, 0, len(texts))
	for ordinal, chunkText := range texts {
		chunks = append(chunks, Chunk{
			PageNumber: pageNumber,
			Ordinal:    ordinal,
			Text:       chunkText,
		})
	}
	return chunks
}

// withDefaults fills in the specified sizes for options a caller left unset.
func (o ChunkOptions) withDefaults() ChunkOptions {
	if o.TargetChars <= 0 {
		o.TargetChars = DefaultChunkTargetChars
	}
	if o.OverlapChars <= 0 {
		o.OverlapChars = DefaultChunkOverlapChars
	}
	return o
}

// splitIntoParagraphs splits a page's text on blank lines.
//
// A blank line is the only paragraph boundary a converted page reliably carries:
// the stored text is verbatim, and this step is forbidden to infer structure
// (no headings, no columns). Line endings are normalised because the extractor's
// output is not guaranteed to be LF-only, and a stray carriage return would hide
// a paragraph boundary.
//
// Each paragraph is trimmed. Leading indentation and trailing spaces are layout,
// not text, and carrying them into a chunk would only pad the index entry that
// gets emitted.
func splitIntoParagraphs(text string) []string {
	paragraphs := make([]string, 0)
	lines := make([]string, 0)

	flush := func() {
		if len(lines) == 0 {
			return
		}
		if paragraph := strings.TrimSpace(strings.Join(lines, "\n")); paragraph != "" {
			paragraphs = append(paragraphs, paragraph)
		}
		lines = lines[:0]
	}

	for _, line := range strings.Split(normalizeLineEndings(text), "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		lines = append(lines, line)
	}
	flush()

	return paragraphs
}

// normalizeLineEndings turns CRLF and CR into LF, so one blank line is one
// paragraph boundary whatever the extractor emitted.
func normalizeLineEndings(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

// splitAtWordBoundaries cuts an over-long paragraph into pieces of at most
// target characters, at word boundaries.
//
// A word longer than the target is emitted as its own piece rather than cut: a
// cut identifier is a term nobody can search for, and the damage is bounded to
// one over-sized chunk instead of every chunk after it.
func splitAtWordBoundaries(paragraph string, target int) []string {
	pieces := make([]string, 0)
	current := ""

	for _, word := range strings.Fields(paragraph) {
		switch {
		case current == "":
			current = word
		case runeLength(current)+1+runeLength(word) <= target:
			current += " " + word
		default:
			pieces = append(pieces, current)
			current = word
		}
	}
	if current != "" {
		pieces = append(pieces, current)
	}

	return pieces
}

// mergeIntoChunks packs consecutive units into one chunk while the result stays
// within the target, and starts a new chunk when it would not.
//
// No unit is ever dropped or split here: the caller has already made every unit
// fit, so a chunk that skipped one would silently lose text from the index.
func mergeIntoChunks(units []string, target int) []string {
	chunks := make([]string, 0, len(units))
	current := ""

	for _, unit := range units {
		switch {
		case current == "":
			current = unit
		case runeLength(current)+runeLength(paragraphSeparator)+runeLength(unit) <= target:
			current += paragraphSeparator + unit
		default:
			chunks = append(chunks, current)
			current = unit
		}
	}
	if current != "" {
		chunks = append(chunks, current)
	}

	return chunks
}

// carryOverlaps repeats the tail of each chunk at the start of the next one.
//
// The tail is taken from the chunk's own text, before a carry was added to it,
// so a carry can never chain text from two chunks back.
func carryOverlaps(chunks []string, overlap int) []string {
	carried := make([]string, len(chunks))

	for i, chunk := range chunks {
		if i == 0 {
			carried[i] = chunk
			continue
		}
		if tail := overlapTail(chunks[i-1], overlap); tail != "" {
			chunk = tail + paragraphSeparator + chunk
		}
		carried[i] = chunk
	}

	return carried
}

// overlapTail returns about overlap trailing characters of text, starting at a
// word boundary, or nothing when there is no boundary to start at.
//
// The forward snap is what keeps a term whole: cutting a fixed number of
// characters lands inside a word as often as not, and a chunk that opens with
// half a word indexes a term that exists nowhere else. When the requested tail
// holds no whitespace at all, no carry is taken — repeating a single huge word,
// or a whole short chunk, would index the same text twice.
func overlapTail(text string, overlap int) string {
	if overlap <= 0 {
		return ""
	}

	runes := []rune(text)
	if len(runes) <= overlap {
		return ""
	}

	cut := len(runes) - overlap
	for cut < len(runes) && !unicode.IsSpace(runes[cut]) {
		cut++
	}

	return strings.TrimRightFunc(strings.TrimLeftFunc(string(runes[cut:]), unicode.IsSpace), unicode.IsSpace)
}

// runeLength counts characters, which is the unit the sizes are specified in: a
// byte count would make a Spanish page hold fewer chunks than the same English
// page for no reason a reader would recognise.
func runeLength(text string) int { return utf8.RuneCountInString(text) }
