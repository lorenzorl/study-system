package domain

import (
	"context"
	"strings"
	"unicode/utf8"
)

// TextExtractor reads the text layer of a PDF page by page.
//
// Pages are 1-based everywhere, matching PageOutcome.PageNumber: page 1 is the
// first page. A page number below 1 is apperror.ErrInvalidInput and must be
// rejected before any external process is spawned, so a bad number aborts the
// run at the call site rather than in the middle of a batch of renders.
type TextExtractor interface {
	// PageCount returns the number of pages in the document.
	PageCount(ctx context.Context) (int, error)

	// PageText returns one page's text with trailing whitespace and form feeds
	// stripped. A page that carries no text is not an error: it returns "" with a
	// nil error, and the heuristic is what sends such a page to the model.
	PageText(ctx context.Context, page int) (string, error)
}

// FigureIndex reports which pages carry at least one raster image.
//
// Masks and other non-image objects do not count: only a real picture is a
// reason to ask a vision model to look at the page.
type FigureIndex interface {
	// ImageCounts returns the number of images on each page that has any. Pages
	// with no image are absent from the map, never mapped to zero, so a page that
	// is simply not a diagram cannot be confused with one that failed to parse.
	ImageCounts(ctx context.Context) (map[int]int, error)
}

// PageRenderer turns one page into PNG bytes for a vision call.
//
// Nothing is persisted: the caller holds the bytes and decides their lifetime.
// Implementations render into scratch space and clean it up on every path, so a
// long book does not accumulate one orphan image per page.
type PageRenderer interface {
	// RenderPage returns PNG bytes of one page. Page numbers are 1-based like the
	// other ports, and a page below 1 is apperror.ErrInvalidInput.
	RenderPage(ctx context.Context, page int) ([]byte, error)
}

// MinExtractableText is the rune count below which extracted text is not trusted
// to be a page's real content.
//
// The threshold has an empirical basis: the real test book yields roughly 2,400
// characters per page, so 200 is an order of magnitude below a normal page. It
// catches pages that produced only a heading or a caption while leaving a
// full-but-sparse page to the extractor.
const MinExtractableText = 200

// NeedsModel reports whether a page should be sent to the vision model rather
// than trusted as extracted text.
//
// It is true when the trimmed text is shorter than MinExtractableText (counted
// in runes, not bytes, so a page of CJK text is measured by its characters) or
// when the page carries at least one image.
//
// The model is rationed deliberately. An extractor that fails produces visibly
// garbled text that matches no query; a model that is unsure invents
// confidently, and an invention matches queries and answers them wrongly. Since
// the second failure is silent, limiting how much of the corpus can be
// confidently wrong is the point of sending only weak or illustrated pages to
// the model.
//
// Detecting *garbled* text, as opposed to short text, is deliberately out of
// scope: a garbled page is typically long, and telling it apart from a legitimate
// page needs a model or a heuristic that would itself be guesswork. Short text
// and images are the two signals that can be measured without guessing.
//
// A blank page (no text, no image) currently reaches the model, because its
// trimmed text is well under the threshold. That is accepted waste, not an
// oversight: it costs a handful of extra calls on a 359-page book and is the
// price of not having to special-case a page that carries no signal at all.
func NeedsModel(extracted string, imagesOnPage int) bool {
	return utf8.RuneCountInString(strings.TrimSpace(extracted)) < MinExtractableText || imagesOnPage > 0
}
