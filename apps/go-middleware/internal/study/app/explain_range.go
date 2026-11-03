package app

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

const (
	// MaxRangeGaps is the most gaps one answer carries.
	//
	// The list is capped because an uncapped diff against a 28-page chapter lists
	// everything, which is the same as listing nothing. The producer ranks its
	// answer; the cap keeps the top of that ranking.
	MaxRangeGaps = 7

	// MaxExplainRangePages is the widest range one explanation may cover.
	//
	// The model answers a range in one synchronous request, and a measured run
	// spent 61 seconds on four pages: a chapter cannot be answered that way, and a
	// request past the edge proxy's ceiling fails with nothing to show for it.
	// Twelve pages keeps the request inside that ceiling and asks the reader for
	// one section at a time, which is also the unit they read in.
	MaxExplainRangePages = 12

	// MinExplanationCharactersPerPage is how much the reader must write per page of
	// the range before the explanation can be judged at all.
	//
	// An explanation too short to judge is refused, not graded: a model that
	// produces gaps out of two sentences is a slot machine, and its answer would
	// teach the reader to distrust the feature. Forty characters is roughly a
	// sentence; a page of this book carries about 2,400, so the threshold asks for
	// a section's worth of reading, not a précis.
	MinExplanationCharactersPerPage = 40

	// MinGapPointCharacters is how much a gap's point must say to be actionable.
	//
	// A gap becomes a card, so a point shorter than a sentence names nothing the
	// reader can study. Twenty characters is the length below which the point is a
	// label, not a claim.
	MinGapPointCharacters = 20

	// MinGapQuoteCharacters is how much of the page a gap's quote must carry.
	//
	// The quote is the evidence: it is what lets the reader check the gap against
	// the book without reading the page again. Fifteen characters is shorter than
	// the three dots a live run stored, and long enough that a kept quote is a
	// passage rather than a word that happens to appear everywhere.
	MinGapQuoteCharacters = 15
)

// RangePage is one page of the recorded text a gap producer reads.
type RangePage struct {
	// PageNumber is the 1-based page the text belongs to.
	PageNumber int

	// Text is the page's recorded text. A page with no recorded text is empty and
	// is not offered to the producer: a gap cannot cite a page nobody wrote.
	Text string
}

// RangeText is the recorded page text of one document range.
//
// It is a port, not an import. The page store belongs to the ingestion context,
// and the study context must never depend on it: the adapter lives in
// internal/composition and wraps the rag pages use case, which is also where the
// page-range bounds live. A refused range therefore reaches this use case as an
// error that unwraps to apperror.ErrInvalidInput, and an unknown document as
// apperror.ErrNotFound, so no bound is copied here and the transport still
// answers 400 and 404 from the sentinels alone.
type RangeText interface {
	// ReadRange returns the recorded pages of the range, ascending. An empty
	// result, or a result whose pages carry no text, means the range recorded
	// nothing.
	ReadRange(ctx context.Context, documentID string, from, to int) ([]RangePage, error)
}

// GapSource is what a gap producer reads: the range's text and the reader's
// explanation, both verbatim.
type GapSource struct {
	FromPage int
	ToPage   int

	// RangeText is the recorded text of the range, each page under a
	// "--- page N ---" marker, so a producer can name the page a gap comes from.
	RangeText string

	// Explanation is the reader's words, unchanged.
	Explanation string
}

// GapFinder turns a gap source into the gaps it found.
//
// It is one producer of a resolved gap list, not part of this use case. The model
// over OpenRouter is the producer built today; the agent plane, which reads the
// range itself and posts the gaps it found, is the next one. That is the reason
// the model call is behind a port and the invariants are applied to the resolved
// list afterwards: no producer may skip them, and swapping the producer must not
// touch the validation.
type GapFinder interface {
	// FindGaps returns the gaps its source left out, most important first. A
	// producer may return an unusable entry — the use case drops it — and a
	// failure here is the producer's failure, not the caller's.
	FindGaps(ctx context.Context, source GapSource) ([]domain.RangeGap, error)
}

// ExplainRangeInput is one request to explain a page range.
type ExplainRangeInput struct {
	DocumentID string
	FromPage   int
	ToPage     int

	// Text is the reader's explanation, verbatim: it is stored as written.
	Text string
}

// ExplanationRefusal is a refused explanation, carrying the sentence the reader
// reads.
//
// The sentence is data the caller's transport does not have — how wide the range
// is and how much was written — so the refusal is worded where those numbers are
// known. It unwraps to apperror.ErrInvalidInput, so a caller that only knows the
// sentinels still answers 400.
type ExplanationRefusal struct {
	message string
}

// Error is the caller-facing refusal.
func (e *ExplanationRefusal) Error() string { return e.message }

// Unwrap makes every refused explanation an invalid input to any caller that
// classifies by sentinel.
func (e *ExplanationRefusal) Unwrap() error { return apperror.ErrInvalidInput }

// NewExplanationRefusal builds a refusal carrying message.
//
// It exists so the one refusal type can be worded where its numbers are known —
// the range's width, the characters written — and still be classified by any
// caller that only knows apperror.ErrInvalidInput.
func NewExplanationRefusal(message string) *ExplanationRefusal {
	return &ExplanationRefusal{message: message}
}

// ExplainRangeUseCase explains a page range in the reader's own words, resolves
// the gaps that explanation left out, and stores both.
type ExplainRangeUseCase struct {
	rangeText    RangeText
	gaps         GapFinder
	explanations domain.RangeExplanationRepository
}

// NewExplainRangeUseCase wires the range-explanation use case.
func NewExplainRangeUseCase(
	rangeText RangeText,
	gaps GapFinder,
	explanations domain.RangeExplanationRepository,
) *ExplainRangeUseCase {
	return &ExplainRangeUseCase{rangeText: rangeText, gaps: gaps, explanations: explanations}
}

// Execute reads the range, asks the gap producer what the explanation left out,
// applies the invariants to that list, and stores the explanation with it.
//
// The write is the last thing that happens, on purpose: a refused range, a range
// with no recorded text, an explanation too short to judge, and a producer that
// failed all leave both tables exactly as they were.
func (uc *ExplainRangeUseCase) Execute(
	ctx context.Context,
	input ExplainRangeInput,
) (domain.RangeExplanation, error) {
	// The width is refused before a single page is read: it is a property of the
	// request, so reading the range first would spend the time the cap exists to
	// save.
	if err := refuseWideRange(input); err != nil {
		return domain.RangeExplanation{}, err
	}

	pages, err := uc.rangeText.ReadRange(ctx, input.DocumentID, input.FromPage, input.ToPage)
	if err != nil {
		// The page store's refusal and its not-found pass through unchanged: it
		// owns the range bounds, and the sentinels it wraps are what the transport
		// maps to 400 and 404.
		return domain.RangeExplanation{}, err
	}

	source, err := gapSource(input, pages)
	if err != nil {
		return domain.RangeExplanation{}, err
	}

	// The list is resolved before any invariant is applied, and the resolved list
	// is what the storage step receives. The model is one producer of it; the
	// agent plane will be the other, which is why nothing below this line knows
	// who computed the gaps.
	resolved, err := uc.gaps.FindGaps(ctx, source)
	if err != nil {
		// A model failure is this server's failure. It is deliberately not wrapped
		// in an apperror sentinel: that would turn it into a 4xx and blame the
		// reader for an answer the model never produced.
		return domain.RangeExplanation{}, fmt.Errorf("find the gaps the explanation left out: %w", err)
	}

	return uc.store(ctx, input, pages, resolved)
}

// refuseWideRange refuses a span above the range cap.
//
// A span of zero or negative pages is not refused here: the page store owns the
// bounds of a range and its apperror.ErrInvalidInput must reach the caller
// unchanged, so this refusal only ever fires above the cap and never preempts
// that bound.
func refuseWideRange(input ExplainRangeInput) error {
	pagesInRange := input.ToPage - input.FromPage + 1
	if pagesInRange <= MaxExplainRangePages {
		return nil
	}

	return NewExplanationRefusal(fmt.Sprintf(
		"this range is %d pages and explaining a range is limited to %d: explain one section at a time",
		pagesInRange, MaxExplainRangePages))
}

// store applies the invariants to a resolved gap list and writes the explanation
// with the accepted gaps.
//
// It takes the resolved list, not a producer, so the next slice — the agent plane
// posting its own gaps — adds a branch at the call site and changes nothing here.
func (uc *ExplainRangeUseCase) store(
	ctx context.Context,
	input ExplainRangeInput,
	pages []RangePage,
	resolved []domain.RangeGap,
) (domain.RangeExplanation, error) {
	accepted := AcceptGaps(input.FromPage, input.ToPage, pages, resolved)

	explanation := domain.RangeExplanation{
		ID:         uuid.New().String(),
		DocumentID: input.DocumentID,
		FromPage:   input.FromPage,
		ToPage:     input.ToPage,
		Text:       input.Text,
		CreatedAt:  domain.Now(),
		Gaps:       accepted,
	}

	for index, gap := range explanation.Gaps {
		gap.ID = uuid.New().String()
		gap.ExplanationID = explanation.ID
		gap.State = domain.RangeGapStateOpen
		explanation.Gaps[index] = gap
	}

	if err := uc.explanations.Create(ctx, &explanation); err != nil {
		return domain.RangeExplanation{}, fmt.Errorf("store the explanation: %w", err)
	}

	return explanation, nil
}

// gapSource renders the pages a producer reads and refuses the two requests that
// cannot be judged.
//
// The page markers are what make a gap actionable: the producer is told to name
// the page a point comes from, and a text with no page boundaries gives it no way
// to do that honestly.
func gapSource(input ExplainRangeInput, pages []RangePage) (GapSource, error) {
	text := rangeSourceText(pages)
	if text == "" {
		return GapSource{}, NewExplanationRefusal("the range has no recorded text")
	}

	pagesInRange := input.ToPage - input.FromPage + 1
	characters := utf8.RuneCountInString(input.Text)

	if characters < pagesInRange*MinExplanationCharactersPerPage {
		return GapSource{}, NewExplanationRefusal(fmt.Sprintf(
			"this range is %d pages and your explanation covers %d characters: explain one section, or say more",
			pagesInRange, characters))
	}

	return GapSource{
		FromPage:    input.FromPage,
		ToPage:      input.ToPage,
		RangeText:   text,
		Explanation: input.Text,
	}, nil
}

// rangeSourceText renders the pages under their page numbers, skipping the pages
// that recorded nothing: a page the run never recorded is absent, not blank.
func rangeSourceText(pages []RangePage) string {
	var source strings.Builder

	for _, page := range pages {
		text := strings.TrimSpace(page.Text)
		if text == "" {
			continue
		}
		fmt.Fprintf(&source, "--- page %d ---\n%s\n\n", page.PageNumber, text)
	}

	return strings.TrimSpace(source.String())
}

// AcceptGaps applies the invariants a gap producer cannot be trusted with: the
// page that states the gap lies inside the explained range, the point and the
// quote carry enough text to be worth a card, the quote is genuinely on the page
// it cites, and the list is capped while keeping its ranking.
//
// The quote check is what makes a citation trustworthy. A live run stored
// {"point":"...","quote":"..."} because the old rule only looked for blank text,
// and a gap with a fake citation becomes a card with a fake source, which is worse
// than no gap at all. A cited page with no recorded text verifies nothing, so its
// gap is dropped too.
//
// A gap that fails an invariant is dropped rather than failing the request: S3
// rejects a gap without a page, and the rejection is that it never reaches the
// table. One unusable entry from a model is not a reason to throw away the six it
// got right, and the reader sees only what can be turned into a card.
//
// Both producers of a resolved list go through this one function, which is what
// keeps the model path and the agent path on one validation.
func AcceptGaps(fromPage, toPage int, pages []RangePage, gaps []domain.RangeGap) []domain.RangeGap {
	pageTexts := make(map[int]string, len(pages))
	for _, page := range pages {
		pageTexts[page.PageNumber] = page.Text
	}

	accepted := make([]domain.RangeGap, 0, min(len(gaps), MaxRangeGaps))

	for _, gap := range gaps {
		if gap.Page < fromPage || gap.Page > toPage {
			continue
		}
		if utf8.RuneCountInString(strings.TrimSpace(gap.Point)) < MinGapPointCharacters {
			continue
		}
		if utf8.RuneCountInString(strings.TrimSpace(gap.Quote)) < MinGapQuoteCharacters {
			continue
		}
		if !quoteAppearsOnPage(gap.Quote, pageTexts[gap.Page]) {
			continue
		}

		accepted = append(accepted, gap)
		if len(accepted) == MaxRangeGaps {
			break
		}
	}

	return accepted
}

// quoteAppearsOnPage reports whether the quote is genuinely on the page it cites,
// compared after both sides are normalised the same way.
//
// A page with no recorded text carries an empty text, which no quote is contained
// in: it verifies nothing, so the gap is dropped instead of passing on its page
// number alone.
func quoteAppearsOnPage(quote, pageText string) bool {
	normalisedQuote := normaliseGapText(quote)
	if normalisedQuote == "" {
		return false
	}

	return strings.Contains(normaliseGapText(pageText), normalisedQuote)
}

// normaliseGapText lowercases text and collapses every run of whitespace,
// including the leading and trailing ones, to a single space.
//
// The comparison is normalised because the quote is retyped, not copied: a
// producer that lowercases a passage or breaks it across lines is still quoting
// the book, while one that invents a passage is not quoting it in any shape.
func normaliseGapText(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(text)), " ")
}
