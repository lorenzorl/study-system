package domain

import "time"

// RangeExplanation is the reader's own account of one page range, stored
// verbatim, together with the gaps that account left out.
//
// The text is not summarised, scored, or rewritten. It is later the reader's own
// retrieval material: re-reading their words weeks afterwards is itself the test
// of whether the range was understood, which is why there is no comprehension
// score anywhere near it.
type RangeExplanation struct {
	ID string

	// DocumentID is the ingested document the range belongs to. It is a document
	// of the ingestion context, so it is an identifier here and not a foreign key:
	// the two contexts share a database file, not a schema.
	DocumentID string

	// FromPage and ToPage are 1-based and inclusive, as the reader named them.
	FromPage int
	ToPage   int

	// Text is the explanation, verbatim.
	Text string

	CreatedAt time.Time

	// Gaps are what the explanation left out, accepted and capped. Empty is a
	// complete explanation, not a failure.
	Gaps []RangeGap
}

// RangeGapState is where a gap stands in its journey to a card.
//
// The stored column is TEXT with no CHECK, following ingestion_jobs.status, so a
// vocabulary that grows does not need a migration for every new word.
type RangeGapState string

const (
	// RangeGapStateOpen is a gap nobody has acted on yet. It is the state every
	// stored gap has in v1.
	RangeGapStateOpen RangeGapState = "open"

	// RangeGapStateCarded is a gap a card now covers, written through the existing
	// flashcards endpoint.
	//
	// The word belongs to the vocabulary but this slice never writes it: turning a
	// gap into a card is the next slice, and naming it here is what lets a stored
	// row carry it without a schema change.
	RangeGapStateCarded RangeGapState = "carded"
)

// RangeGap is one point the explanation left out, with the page that states it
// and a short quote as evidence.
//
// Page and Quote are what make the gap actionable: without a page the reader
// cannot open the book, and without a quote nobody can check the claim. A gap
// missing either is refused rather than stored.
type RangeGap struct {
	ID            string
	ExplanationID string

	// Point names, in one sentence, the idea the explanation does not cover.
	Point string

	// Page is the 1-based page that states the point. It must lie inside the
	// explained range.
	Page int

	// Quote is the short verbatim passage from Page that evidences the gap.
	Quote string

	// State is the gap's lifecycle state.
	State RangeGapState
}
