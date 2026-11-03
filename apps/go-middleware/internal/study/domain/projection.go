package domain

// ExplainedRange is the document and the exact page range an explanation covers.
//
// Only the pair travels. The projection asks one question of an explanation — has
// this range already been distilled — and the reader's text belongs to the reading
// surface, not to a list of things to do.
type ExplainedRange struct {
	DocumentID string

	// FromPage and ToPage are 1-based and inclusive, as the reader named them.
	FromPage int
	ToPage   int
}

// PlanItemWithDocument is one plan item together with the document its plan
// belongs to, because a projection over every material needs both halves.
//
// The document travels because "what do I read next" is a question across every
// book, and an item alone cannot say which material it is a chapter of.
type PlanItemWithDocument struct {
	DocumentID string
	Item       StudyPlanItem
}
