package app

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// The two kinds of work the projection can name.
const (
	// TodayKindReadNotDistilled is a range the reader marked read that has no
	// explanation of the exact same range. Running the explanation is the
	// distillation act of the chosen path, so nothing else marks a range distilled.
	TodayKindReadNotDistilled = "read_not_distilled"

	// TodayKindPlanNext is the first pending item of a document's active plan whose
	// exact range is not marked read: the chapter the plan recommends next.
	TodayKindPlanNext = "plan_next"
)

// TodayEntry is one thing to do today.
//
// One shape carries both kinds because the page asks one question — what do I do —
// and each kind leaves the other's fields zero: a reading has a ReadAt and no plan
// title, a plan item has a title and no ReadAt.
type TodayEntry struct {
	Kind          string
	DocumentID    string
	DocumentTitle string
	Title         string
	FromPage      int
	ToPage        int
	Note          string
	ReadAt        time.Time
}

// Today is the projection: how many reviews are due, and the work that has a range
// to name.
//
// Due reviews are a count and not an entry because there is no range to name, and
// the entries are a list rather than a map because their order is half the answer:
// what to do first.
type Today struct {
	DueCount int
	Entries  []TodayEntry
}

// TodayUseCase projects what the reader has to do today over data other routes
// already stored.
//
// It writes nothing and decides nothing. The plan said which chapter comes next,
// coverage said which ranges are read, and the explanations said which of those
// were distilled; this use case only puts those answers in one order.
type TodayUseCase struct {
	due          domain.FlashcardRepository
	plans        domain.StudyPlanRepository
	progress     domain.ReadingProgressRepository
	explanations domain.RangeExplanationRepository
	documents    Documents
}

// NewTodayUseCase creates a TodayUseCase.
func NewTodayUseCase(
	due domain.FlashcardRepository,
	plans domain.StudyPlanRepository,
	progress domain.ReadingProgressRepository,
	explanations domain.RangeExplanationRepository,
	documents Documents,
) *TodayUseCase {
	return &TodayUseCase{
		due:          due,
		plans:        plans,
		progress:     progress,
		explanations: explanations,
		documents:    documents,
	}
}

// Execute returns the due count and the ordered entries.
func (uc *TodayUseCase) Execute(ctx context.Context) (Today, error) {
	// The due count reuses the review route's query instead of a COUNT of its own:
	// "how many are due" must answer the number the review page will show, and one
	// query cannot drift from the other.
	due, err := uc.due.FindDueWithContext(ctx, domain.Now())
	if err != nil {
		return Today{}, fmt.Errorf("find due cards: %w", err)
	}

	reads, err := uc.progress.ListRead(ctx)
	if err != nil {
		return Today{}, fmt.Errorf("list read ranges: %w", err)
	}

	explained, err := uc.explanations.ListExplainedRanges(ctx)
	if err != nil {
		return Today{}, fmt.Errorf("list explained ranges: %w", err)
	}

	items, err := uc.plans.ListActiveItems(ctx)
	if err != nil {
		return Today{}, fmt.Errorf("list active plan items: %w", err)
	}

	distilled := make(map[string]bool, len(explained))
	for _, covered := range explained {
		distilled[documentRangeKey(covered.DocumentID, covered.FromPage, covered.ToPage)] = true
	}

	read := make(map[string]bool, len(reads))
	for _, marked := range reads {
		read[documentRangeKey(marked.DocumentID, marked.FromPage, marked.ToPage)] = true
	}

	titles := newTitleLookup(uc.documents)

	readings := oldestUndistilledReading(reads, distilled)
	entries := make([]TodayEntry, 0, len(readings)+len(items))

	for _, reading := range readings {
		entries = append(entries, TodayEntry{
			Kind:          TodayKindReadNotDistilled,
			DocumentID:    reading.DocumentID,
			DocumentTitle: titles.title(ctx, reading.DocumentID),
			FromPage:      reading.FromPage,
			ToPage:        reading.ToPage,
			ReadAt:        reading.UpdatedAt,
		})
	}

	// Oldest reading first: the range that has waited longest is the one most
	// likely to be forgotten. Title and id settle a tie, so the same request twice
	// answers in the same order.
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].ReadAt.Equal(entries[j].ReadAt) {
			return entries[i].ReadAt.Before(entries[j].ReadAt)
		}
		if entries[i].DocumentTitle != entries[j].DocumentTitle {
			return entries[i].DocumentTitle < entries[j].DocumentTitle
		}
		return entries[i].DocumentID < entries[j].DocumentID
	})

	plans := nextPlanItems(ctx, items, read, titles)
	sort.SliceStable(plans, func(i, j int) bool {
		if plans[i].entry.DocumentTitle != plans[j].entry.DocumentTitle {
			return plans[i].entry.DocumentTitle < plans[j].entry.DocumentTitle
		}
		if plans[i].position != plans[j].position {
			return plans[i].position < plans[j].position
		}
		return plans[i].entry.DocumentID < plans[j].entry.DocumentID
	})

	for _, candidate := range plans {
		entries = append(entries, candidate.entry)
	}

	return Today{DueCount: len(due), Entries: entries}, nil
}

// oldestUndistilledReading keeps, per document, the oldest read range that no
// explanation covers.
//
// Per document because the page asks one thing per material: two undistilled
// chapters of the same book are one task — go explain a range — and naming both
// would turn the projection into a backlog. Which one is decided by the instant,
// not by the order the rows arrived in.
func oldestUndistilledReading(
	reads []domain.ReadingProgress,
	distilled map[string]bool,
) []domain.ReadingProgress {
	oldest := make(map[string]domain.ReadingProgress, len(reads))
	for _, reading := range reads {
		if distilled[documentRangeKey(reading.DocumentID, reading.FromPage, reading.ToPage)] {
			continue
		}

		kept, seen := oldest[reading.DocumentID]
		if !seen || olderReading(reading, kept) {
			oldest[reading.DocumentID] = reading
		}
	}

	readings := make([]domain.ReadingProgress, 0, len(oldest))
	for _, reading := range oldest {
		readings = append(readings, reading)
	}

	return readings
}

// olderReading answers which of two readings of one document comes first: the older
// instant, and the id when both were marked in the same second.
func olderReading(candidate, kept domain.ReadingProgress) bool {
	if !candidate.UpdatedAt.Equal(kept.UpdatedAt) {
		return candidate.UpdatedAt.Before(kept.UpdatedAt)
	}

	return candidate.ID < kept.ID
}

// planCandidate is a plan entry with the position the order needs: two documents
// whose titles are equal are still ordered by each plan's own sequence.
type planCandidate struct {
	entry    TodayEntry
	position int
}

// nextPlanItems keeps, per document, the first item in position order that is
// pending and whose exact range is not read.
//
// Only pending items: a skipped one is a chapter the reader decided against, and
// recommending it again would be the plan arguing with its reader.
func nextPlanItems(
	ctx context.Context,
	items []domain.PlanItemWithDocument,
	read map[string]bool,
	titles *titleLookup,
) []planCandidate {
	next := make(map[string]planCandidate, len(items))

	for _, item := range items {
		if item.Item.State != domain.PlanItemStatePending {
			continue
		}

		if read[documentRangeKey(item.DocumentID, item.Item.FromPage, item.Item.ToPage)] {
			continue
		}

		if kept, seen := next[item.DocumentID]; seen && kept.position <= item.Item.Position {
			continue
		}

		next[item.DocumentID] = planCandidate{
			position: item.Item.Position,
			entry: TodayEntry{
				Kind:          TodayKindPlanNext,
				DocumentID:    item.DocumentID,
				DocumentTitle: titles.title(ctx, item.DocumentID),
				Title:         item.Item.Title,
				FromPage:      item.Item.FromPage,
				ToPage:        item.Item.ToPage,
				Note:          item.Item.Note,
			},
		}
	}

	candidates := make([]planCandidate, 0, len(next))
	for _, candidate := range next {
		candidates = append(candidates, candidate)
	}

	return candidates
}

// titleLookup resolves a document's title once per document.
//
// Once, because one document can own both entries and the catalogue should not be
// asked the same question twice in one projection. A failure is cached too: a
// catalogue that cannot answer is not retried for every entry.
type titleLookup struct {
	documents Documents
	titles    map[string]string
}

func newTitleLookup(documents Documents) *titleLookup {
	return &titleLookup{documents: documents, titles: make(map[string]string)}
}

// title returns the document's title, or an empty string when it cannot be
// resolved.
//
// The entry survives that failure on purpose: an unnamed material is still a range
// the reader can mark read or explain, and failing the whole projection because one
// title is gone would hide every other entry.
func (l *titleLookup) title(ctx context.Context, documentID string) string {
	if title, resolved := l.titles[documentID]; resolved {
		return title
	}

	title, err := l.documents.Title(ctx, documentID)
	if err != nil {
		title = ""
	}

	l.titles[documentID] = title

	return title
}

// documentRangeKey is how the projection matches a range: the document and the
// exact range, and nothing else.
//
// The document is part of the key because two books can both have a chapter on
// pages 24-36. The range is exact for the reason the plan derives `read` this way:
// overlap would call a chapter read, or distilled, because part of it was.
func documentRangeKey(documentID string, from, to int) string {
	return documentID + "/" + rangeKey(from, to)
}
