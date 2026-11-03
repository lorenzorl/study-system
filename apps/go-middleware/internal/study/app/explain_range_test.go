package app_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// fakeRangeText stands in for the ingestion context's page read. It records that
// it was called, so a refusal can prove it never read a page.
type fakeRangeText struct {
	pages []app.RangePage
	err   error
	calls int
}

func (f *fakeRangeText) ReadRange(ctx context.Context, documentID string, from, to int) ([]app.RangePage, error) {
	f.calls++
	return f.pages, f.err
}

// fakeGapFinder is one producer of a resolved gap list, standing in for the model.
type fakeGapFinder struct {
	gaps    []domain.RangeGap
	err     error
	sources []app.GapSource
}

func (f *fakeGapFinder) FindGaps(ctx context.Context, source app.GapSource) ([]domain.RangeGap, error) {
	f.sources = append(f.sources, source)
	return f.gaps, f.err
}

// newExplainRange wires the use case over a migrated in-memory database.
//
// The repository is the real one on purpose: "nothing is stored on a refusal" is
// a claim about the tables, and a mock's call count would prove only that the use
// case did not call it.
func newExplainRange(
	t *testing.T,
	pages *fakeRangeText,
	finder *fakeGapFinder,
) (*app.ExplainRangeUseCase, *sql.DB) {
	t.Helper()

	ctx := t.Context()
	sqlDB, err := db.Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.Migrate(ctx, sqlDB, sqlite.Migrations()))

	return app.NewExplainRangeUseCase(
		pages,
		finder,
		sqlite.NewRangeExplanationRepository(sqlDB),
	), sqlDB
}

// storedRows reports how many explanations and gaps the tables hold.
func storedRows(t *testing.T, sqlDB *sql.DB) (explanations, gaps int) {
	t.Helper()

	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM range_explanations`).Scan(&explanations))
	require.NoError(t, sqlDB.QueryRow(`SELECT count(*) FROM range_gaps`).Scan(&gaps))
	return explanations, gaps
}

const (
	// firstPage is the fixture's first page, and explainedPages is how many pages
	// the fixture's range covers: twelve, which is the widest span one explanation
	// may cover.
	firstPage      = 88
	explainedPages = 12
	lastPage       = firstPage + explainedPages - 1

	// pagePassage is the passage every recorded page of the fixture carries, and it
	// is what the fixture's gaps quote: a gap is kept only when its quote is on the
	// page it cites.
	pagePassage = "cluster the entities and value objects into aggregates and bound them with an aggregate root"
)

// pageText is one recorded page: it names itself, so a gap can be checked against
// the page it cites, and it carries the passage the fixture's gaps quote.
func pageText(number int) string {
	return fmt.Sprintf("Page %d states an invariant of the aggregate: %s.", number, pagePassage)
}

// rangePages is the recorded text of pages 88 to 99, one line per page, which is
// what the model is asked to read.
func rangePages() []app.RangePage {
	pages := make([]app.RangePage, 0, explainedPages)
	for number := firstPage; number <= lastPage; number++ {
		pages = append(pages, app.RangePage{
			PageNumber: number,
			Text:       pageText(number),
		})
	}
	return pages
}

// anExplanation is a text long enough for a range of pages to be judged.
func anExplanation(pages int) string {
	return strings.Repeat("the aggregate is a consistency boundary. ", pages)
}

func validInput() app.ExplainRangeInput {
	return app.ExplainRangeInput{
		DocumentID: "c6d1e9a3-0000-4000-8000-000000000001",
		FromPage:   firstPage,
		ToPage:     lastPage,
		Text:       anExplanation(explainedPages),
	}
}

func validGap() domain.RangeGap {
	return domain.RangeGap{
		Point: "the boundary is defined by the invariants that must hold in one transaction",
		Page:  89,
		Quote: "cluster the entities and value objects into aggregates",
	}
}

// S2 and S3: the reader's words are stored verbatim beside the gaps, and every
// gap carries its point, its page and its quote.
func TestExplainRange_StoresTheExplanationAndItsGaps(t *testing.T) {
	ctx := t.Context()

	input := validInput()
	input.Text = strings.Repeat("An aggregate is a consistency boundary.\nIt is not a bag of objects.\n", 20)

	pages := &fakeRangeText{pages: rangePages()}
	finder := &fakeGapFinder{gaps: []domain.RangeGap{
		validGap(),
		{Point: "a repository is per aggregate root", Page: 95, Quote: "bound them with an aggregate root"},
	}}

	useCase, sqlDB := newExplainRange(t, pages, finder)

	explanation, err := useCase.Execute(ctx, input)
	require.NoError(t, err)

	assert.NotEmpty(t, explanation.ID, "a stored explanation must be identifiable")
	assert.Equal(t, input.DocumentID, explanation.DocumentID)
	assert.Equal(t, firstPage, explanation.FromPage)
	assert.Equal(t, lastPage, explanation.ToPage)

	stored, err := sqlite.NewRangeExplanationRepository(sqlDB).FindByID(ctx, explanation.ID)
	require.NoError(t, err)
	assert.Equal(t, input.Text, stored.Text, "the explanation must be stored verbatim")
	require.Len(t, stored.Gaps, 2)
	assert.Equal(t, validGap().Point, stored.Gaps[0].Point)
	assert.Equal(t, 89, stored.Gaps[0].Page)
	assert.Equal(t, validGap().Quote, stored.Gaps[0].Quote)
	assert.Equal(t, domain.RangeGapStateOpen, stored.Gaps[0].State)

	explanations, gaps := storedRows(t, sqlDB)
	assert.Equal(t, 1, explanations)
	assert.Equal(t, 2, gaps)

	// The producer is handed the range text under its page numbers — a gap that
	// cannot name its page is not actionable — and the explanation verbatim.
	require.Len(t, finder.sources, 1)
	assert.Contains(t, finder.sources[0].RangeText, "--- page 89 ---")
	assert.Contains(t, finder.sources[0].RangeText, "Page 89 states an invariant")
	assert.Equal(t, input.Text, finder.sources[0].Explanation)
	assert.Equal(t, firstPage, finder.sources[0].FromPage)
	assert.Equal(t, lastPage, finder.sources[0].ToPage)
}

// S3: "Cada hueco lleva el punto, la página y una cita corta. Un hueco sin página
// se rechaza." The gap is rejected in the only place that matters — it never
// reaches the table — while the gaps that do carry their evidence are kept.
func TestExplainRange_DropsGapsWithoutEvidence(t *testing.T) {
	ctx := t.Context()

	kept := validGap()
	usablePoint := "the boundary is defined by the invariants that must hold in one transaction"
	usableQuote := "cluster the entities and value objects into aggregates"

	finder := &fakeGapFinder{gaps: []domain.RangeGap{
		{Point: usablePoint, Page: 0, Quote: usableQuote},
		{Point: usablePoint, Page: firstPage - 1, Quote: usableQuote},
		{Point: usablePoint, Page: lastPage + 1, Quote: usableQuote},
		{Point: usablePoint, Page: 90, Quote: "   "},
		{Point: "  ", Page: 91, Quote: usableQuote},
		kept,
	}}

	useCase, sqlDB := newExplainRange(t, &fakeRangeText{pages: rangePages()}, finder)

	explanation, err := useCase.Execute(ctx, validInput())
	require.NoError(t, err)
	require.Len(t, explanation.Gaps, 1, "only the gap with its point, its page and its quote is stored")
	assert.Equal(t, kept.Point, explanation.Gaps[0].Point)
	assert.Equal(t, 89, explanation.Gaps[0].Page)

	_, gaps := storedRows(t, sqlDB)
	assert.Equal(t, 1, gaps)
}

// S5: the list is capped so a diff does not list everything, and it keeps the
// producer's ranking instead of an arbitrary seven.
func TestExplainRange_CapsTheGapListAtSeven(t *testing.T) {
	ctx := t.Context()

	gaps := make([]domain.RangeGap, 0, explainedPages)
	for index := 0; index < explainedPages; index++ {
		gaps = append(gaps, domain.RangeGap{
			Point: fmt.Sprintf("gap %d names a boundary the explanation never states", index+1),
			Page:  firstPage + index,
			Quote: pagePassage,
		})
	}

	useCase, sqlDB := newExplainRange(t, &fakeRangeText{pages: rangePages()}, &fakeGapFinder{gaps: gaps})

	explanation, err := useCase.Execute(ctx, validInput())
	require.NoError(t, err)
	require.Len(t, explanation.Gaps, 7, "twelve gaps must be capped to seven")

	for index, gap := range explanation.Gaps {
		assert.Equal(t, index+1, gap.Page-firstPage+1, "the cap must keep the ranking")
	}

	_, stored := storedRows(t, sqlDB)
	assert.Equal(t, 7, stored)
}

// S6: an explanation too short to judge is refused, with a sentence that says
// what to do instead, and the refusal stores nothing and calls nobody.
func TestExplainRange_RefusesAnExplanationTooShortForTheRange(t *testing.T) {
	threshold := explainedPages * app.MinExplanationCharactersPerPage

	tests := []struct {
		name       string
		text       string
		wantRefuse bool
		wantMsg    string
	}{
		{
			name:       "three characters",
			text:       "ok.",
			wantRefuse: true,
			wantMsg:    "this range is 12 pages and your explanation covers 3 characters: explain one section, or say more",
		},
		{
			name:       "one character below the threshold",
			text:       strings.Repeat("x", threshold-1),
			wantRefuse: true,
			wantMsg: fmt.Sprintf(
				"this range is 12 pages and your explanation covers %d characters: explain one section, or say more",
				threshold-1),
		},
		{
			name: "exactly the threshold is enough",
			text: strings.Repeat("x", threshold),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()

			pages := &fakeRangeText{pages: rangePages()}
			finder := &fakeGapFinder{}
			useCase, sqlDB := newExplainRange(t, pages, finder)

			input := validInput()
			input.Text = test.text

			_, err := useCase.Execute(ctx, input)

			if !test.wantRefuse {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.Equal(t, test.wantMsg, err.Error(), "the refusal must say what to do instead")
			assert.ErrorIs(t, err, apperror.ErrInvalidInput, "a refusal must map to 400")

			explanations, gaps := storedRows(t, sqlDB)
			assert.Zero(t, explanations, "a refused explanation stores nothing")
			assert.Zero(t, gaps, "a refused explanation stores no gap")
			assert.Zero(t, finder.sources, "a refused explanation never calls the model")
		})
	}
}

// The refusal table: a range with no recorded text is a 400 with that sentence,
// not an empty list of gaps that reads as a perfect explanation.
func TestExplainRange_RefusesARangeWithNoRecordedText(t *testing.T) {
	ctx := t.Context()

	pages := &fakeRangeText{pages: []app.RangePage{{PageNumber: 88, Text: "  "}}}
	finder := &fakeGapFinder{}
	useCase, sqlDB := newExplainRange(t, pages, finder)

	_, err := useCase.Execute(ctx, validInput())

	require.Error(t, err)
	assert.Equal(t, "the range has no recorded text", err.Error())
	assert.ErrorIs(t, err, apperror.ErrInvalidInput)

	explanations, gaps := storedRows(t, sqlDB)
	assert.Zero(t, explanations)
	assert.Zero(t, gaps)
	assert.Zero(t, finder.sources)
}

// The range bounds belong to the pages use case, and this is the propagation: its
// refusal arrives as invalid input, so the endpoint answers 400 without owning a
// copy of the bound.
func TestExplainRange_PropagatesARefusedRangeAsInvalidInput(t *testing.T) {
	ctx := t.Context()

	t.Run("a range the page store refuses", func(t *testing.T) {
		pages := &fakeRangeText{
			err: fmt.Errorf("read pages 88 to 99: %w", apperror.ErrInvalidInput),
		}
		finder := &fakeGapFinder{}
		useCase, sqlDB := newExplainRange(t, pages, finder)

		_, err := useCase.Execute(ctx, validInput())

		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrInvalidInput, "the pages use case owns the bound, and its 400 must survive")

		explanations, gaps := storedRows(t, sqlDB)
		assert.Zero(t, explanations)
		assert.Zero(t, gaps)
		assert.Zero(t, finder.sources, "a refused range must not call the model")
	})

	// A span of zero or negative pages is the page store's to refuse, not the range
	// cap's, so its invalid input must surface from the read unchanged.
	t.Run("a degenerate span reaches the page store", func(t *testing.T) {
		for _, span := range []struct{ from, to int }{
			{from: 95, to: 94},
			{from: 99, to: 88},
		} {
			pages := &fakeRangeText{
				err: fmt.Errorf("read pages %d to %d: %w", span.from, span.to, apperror.ErrInvalidInput),
			}
			finder := &fakeGapFinder{}
			useCase, _ := newExplainRange(t, pages, finder)

			input := validInput()
			input.FromPage = span.from
			input.ToPage = span.to

			_, err := useCase.Execute(ctx, input)

			assert.ErrorIs(t, err, apperror.ErrInvalidInput, "the range cap must not preempt the page store's bound")
			assert.Equal(t, 1, pages.calls, "a degenerate span must reach the page store")
			assert.Zero(t, finder.sources, "a refused range must not call the model")
		}
	})
}

func TestExplainRange_ReportsAnUnknownDocument(t *testing.T) {
	ctx := t.Context()

	pages := &fakeRangeText{err: fmt.Errorf("find document: %w", apperror.ErrNotFound)}
	finder := &fakeGapFinder{}
	useCase, sqlDB := newExplainRange(t, pages, finder)

	_, err := useCase.Execute(ctx, validInput())

	require.Error(t, err)
	assert.ErrorIs(t, err, apperror.ErrNotFound, "an unknown document must map to 404")

	explanations, gaps := storedRows(t, sqlDB)
	assert.Zero(t, explanations)
	assert.Zero(t, gaps)
}

// The model failure path: a 500 that carries the model's own message in the log
// and never leaks it to the reader as a 4xx, with both tables untouched.
func TestExplainRange_StoresNothingWhenTheModelFails(t *testing.T) {
	ctx := t.Context()

	finder := &fakeGapFinder{err: errors.New("openrouter returned 502: upstream timeout")}
	useCase, sqlDB := newExplainRange(t, &fakeRangeText{pages: rangePages()}, finder)

	_, err := useCase.Execute(ctx, validInput())

	require.Error(t, err)
	assert.NotErrorIs(t, err, apperror.ErrInvalidInput, "a model failure is not the caller's mistake")
	assert.NotErrorIs(t, err, apperror.ErrNotFound)
	assert.Contains(t, err.Error(), "502", "the cause must reach the log")

	explanations, gaps := storedRows(t, sqlDB)
	assert.Zero(t, explanations, "a model failure stores nothing")
	assert.Zero(t, gaps, "a model failure stores no gap")
}

// A quote that is not on the page it cites is a manufactured citation: the gap is
// dropped, and the gap beside it whose quote is genuinely on its page is kept.
func TestExplainRange_DropsAGapWhoseQuoteIsNotOnItsPage(t *testing.T) {
	ctx := t.Context()

	verifiable := validGap()
	finder := &fakeGapFinder{gaps: []domain.RangeGap{
		{
			Point: "an invented point that carries enough characters to pass the length rule",
			Page:  90,
			Quote: "only AGGREGATE roots may be obtained from the database directly",
		},
		verifiable,
	}}

	useCase, sqlDB := newExplainRange(t, &fakeRangeText{pages: rangePages()}, finder)

	explanation, err := useCase.Execute(ctx, validInput())
	require.NoError(t, err)
	require.Len(t, explanation.Gaps, 1, "a quote that is not on the cited page verifies nothing")
	assert.Equal(t, verifiable.Quote, explanation.Gaps[0].Quote)
	assert.Equal(t, 89, explanation.Gaps[0].Page)

	_, gaps := storedRows(t, sqlDB)
	assert.Equal(t, 1, gaps)
}

// The placeholder a live run stored: "..." passes a blank check and carries no
// evidence at all, so the minimum lengths drop it while the real gap survives.
func TestExplainRange_DropsAPlaceholderGap(t *testing.T) {
	ctx := t.Context()

	kept := validGap()
	finder := &fakeGapFinder{gaps: []domain.RangeGap{
		{Point: "...", Page: 89, Quote: "..."},
		kept,
	}}

	useCase, sqlDB := newExplainRange(t, &fakeRangeText{pages: rangePages()}, finder)

	explanation, err := useCase.Execute(ctx, validInput())
	require.NoError(t, err)
	require.Len(t, explanation.Gaps, 1, "a placeholder is not evidence")
	assert.Equal(t, kept.Point, explanation.Gaps[0].Point)

	_, gaps := storedRows(t, sqlDB)
	assert.Equal(t, 1, gaps)
}

// A producer retypes the passage it quotes, so its capitals and line breaks are
// not evidence either way: the same words in another shape are still on the page.
func TestExplainRange_AcceptsAQuoteThatDiffersOnlyInCaseAndWhitespace(t *testing.T) {
	ctx := t.Context()

	retyped := domain.RangeGap{
		Point: "the boundary is defined by the invariants that must hold in one transaction",
		Page:  89,
		Quote: "  CLUSTER the Entities\n\tand VALUE objects into   aggregates ",
	}
	finder := &fakeGapFinder{gaps: []domain.RangeGap{retyped}}

	useCase, sqlDB := newExplainRange(t, &fakeRangeText{pages: rangePages()}, finder)

	explanation, err := useCase.Execute(ctx, validInput())
	require.NoError(t, err)
	require.Len(t, explanation.Gaps, 1, "case and whitespace must not hide a quote that is on the page")
	assert.Equal(t, retyped.Quote, explanation.Gaps[0].Quote, "the quote is stored as the producer wrote it")

	_, gaps := storedRows(t, sqlDB)
	assert.Equal(t, 1, gaps)
}

// A page the run recorded nothing for cannot verify a citation, so a gap citing it
// is dropped even though the page number lies inside the range.
func TestExplainRange_DropsAGapCitingAPageWithNoRecordedText(t *testing.T) {
	ctx := t.Context()

	pages := rangePages()
	pages[95-firstPage].Text = "   "

	kept := validGap()
	finder := &fakeGapFinder{gaps: []domain.RangeGap{
		{
			Point: "a point whose page recorded nothing at all, and so verifies nothing",
			Page:  95,
			Quote: pagePassage,
		},
		kept,
	}}

	useCase, sqlDB := newExplainRange(t, &fakeRangeText{pages: pages}, finder)

	explanation, err := useCase.Execute(ctx, validInput())
	require.NoError(t, err)
	require.Len(t, explanation.Gaps, 1, "a page with no recorded text verifies nothing")
	assert.Equal(t, 89, explanation.Gaps[0].Page)

	_, gaps := storedRows(t, sqlDB)
	assert.Equal(t, 1, gaps)
}

// A range wider than the cap is refused with the sentence the contract documents,
// before a single page is read: the refusal is about the request, not the material.
func TestExplainRange_RefusesARangeWiderThanTheCap(t *testing.T) {
	ctx := t.Context()

	pages := &fakeRangeText{pages: rangePages()}
	finder := &fakeGapFinder{}
	useCase, sqlDB := newExplainRange(t, pages, finder)

	input := validInput()
	input.ToPage = firstPage + explainedPages
	input.Text = anExplanation(explainedPages + 1)

	_, err := useCase.Execute(ctx, input)

	require.Error(t, err)
	assert.Equal(t,
		"this range is 13 pages and explaining a range is limited to 12: explain one section at a time",
		err.Error())
	assert.ErrorIs(t, err, apperror.ErrInvalidInput, "a refused range must map to 400")

	assert.Zero(t, pages.calls, "a refused range must never be read")
	assert.Zero(t, finder.sources, "a refused range must not call the model")

	explanations, gaps := storedRows(t, sqlDB)
	assert.Zero(t, explanations, "a refused range stores nothing")
	assert.Zero(t, gaps, "a refused range stores no gap")
}
