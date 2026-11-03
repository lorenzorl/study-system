package domain

import "time"

// ReadingMode is how the plan recommends working through a book.
//
// The vocabulary is the skill's (skills/study-plan/references/plan.md) and not the
// app's invention: the classification is judgment, and it lives in the skill. The
// app stores one of these four tokens and projects it; it never derives one.
type ReadingMode string

const (
	// ReadingModeLinear reads the book in its own order.
	ReadingModeLinear ReadingMode = "linear"

	// ReadingModeProject reads what answers a decision the reader has to take.
	ReadingModeProject ReadingMode = "project"

	// ReadingModeSurvey builds a map of the book, with low retention by design.
	ReadingModeSurvey ReadingMode = "survey"

	// ReadingModeConsult keeps the book as a reference and reads nothing in order.
	ReadingModeConsult ReadingMode = "consult"
)

// ParseReadingMode maps a wire value to a mode, and reports whether it is one this
// application knows.
func ParseReadingMode(value string) (ReadingMode, bool) {
	switch ReadingMode(value) {
	case ReadingModeLinear:
		return ReadingModeLinear, true
	case ReadingModeProject:
		return ReadingModeProject, true
	case ReadingModeSurvey:
		return ReadingModeSurvey, true
	case ReadingModeConsult:
		return ReadingModeConsult, true
	}

	return "", false
}

// PlanItemState is where one recommended range stands in the plan.
//
// `read` is deliberately absent: whether a range was read is coverage, stored once
// in reading_progress and derived for the item. Two places holding the same fact is
// how they come to disagree.
type PlanItemState string

const (
	// PlanItemStatePending is an item the reader has not dealt with.
	PlanItemStatePending PlanItemState = "pending"

	// PlanItemStateSkipped is an item the reader decided not to read. The plan
	// recommends skipping chapters, so a plan needs a way to say "not this one"
	// that is not a deletion.
	PlanItemStateSkipped PlanItemState = "skipped"
)

// ParsePlanItemState maps a wire value to an item state, and reports whether it is
// one this application knows.
func ParsePlanItemState(value string) (PlanItemState, bool) {
	switch PlanItemState(value) {
	case PlanItemStatePending:
		return PlanItemStatePending, true
	case PlanItemStateSkipped:
		return PlanItemStateSkipped, true
	}

	return "", false
}

// StudyPlan is one agent-produced plan for one document.
//
// It is partial and disposable by design: the skill recommends skipping chapters, so
// a plan is not required to cover every page, and a new plan supersedes the active
// one instead of asking permission. It declares no foreign key to documents for the
// same reason reading_progress does not: the two contexts share a file, not a schema.
type StudyPlan struct {
	ID         string
	DocumentID string

	ReadingMode ReadingMode

	// Objective is the decision the reader wants to be able to take, in their own
	// words. Empty is a survey plan, which is a legitimate outcome.
	Objective string

	// Source names the skeleton the plan was built from — `sections`, the printed
	// contents, or nothing — so a reader can tell later what the page ranges were
	// derived from.
	Source string

	CreatedAt time.Time

	// SupersededAt is when a newer plan replaced this one. Nil means this is the
	// document's active plan; a superseded plan keeps its rows and its items.
	SupersededAt *time.Time

	Items []StudyPlanItem
}

// StudyPlanItem is one recommended page range, in the order the plan proposes.
type StudyPlanItem struct {
	ID     string
	PlanID string

	// Position is 1-based and assigned by the server from the request order.
	Position int

	// Title is the chapter or section as the book prints it.
	Title string

	// FromPage and ToPage are 1-based and inclusive.
	FromPage int
	ToPage   int

	// Note is the agent's reason or condition for this item, verbatim.
	Note string

	State PlanItemState
}
