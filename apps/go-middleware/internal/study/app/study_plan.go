package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// Documents is the ingestion context's catalogue behind a port.
//
// The study context stores a document id as an identifier and never reads the rag
// tables, so naming a material — the plan's 404, the projection's title — goes
// through this interface, which internal/composition implements over the document
// repository. Study never imports ingestion.
type Documents interface {
	// Title returns the document's title, or apperror.ErrNotFound when no document
	// has that id. Its use as an existence check is deliberate: a caller that can
	// name the material is a caller that may plan against it.
	Title(ctx context.Context, documentID string) (string, error)
}

// PlanItemInput is one recommended range, as the agent proposed it.
type PlanItemInput struct {
	Title    string
	FromPage int
	ToPage   int
	Note     string

	// State is `pending` or `skipped`; empty means pending, because an item the
	// plan proposes is pending until the reader says otherwise.
	State string
}

// SavePlanInput is one plan, as the agent produced it.
type SavePlanInput struct {
	DocumentID string

	// ReadingMode is one of linear, project, survey or consult.
	ReadingMode string

	// Objective is the reader's own words, stored verbatim.
	Objective string

	// Source names the skeleton the page ranges came from.
	Source string

	Items []PlanItemInput
}

// PlanItemView is one stored item with the one fact that is derived rather than
// stored: whether its exact range was marked read.
type PlanItemView struct {
	domain.StudyPlanItem

	Read bool
}

// StudyPlanView is a stored plan as a caller reads it.
type StudyPlanView struct {
	ID          string
	DocumentID  string
	ReadingMode domain.ReadingMode
	Objective   string
	Source      string
	CreatedAt   time.Time
	Items       []PlanItemView
}

// SavePlanUseCase stores the plan the agent produced for one document.
//
// The classification is not here and should not be: what a book is and which
// chapters are its core is judgment, and that judgment lives in the skill. This use
// case decides only whether the plan is well-formed — ranges inside the document,
// one state per item, positions in order — so a malformed plan is refused instead of
// stored as a schedule nobody can follow.
type SavePlanUseCase struct {
	documents Documents
	plans     domain.StudyPlanRepository
	progress  domain.ReadingProgressRepository
}

// NewSavePlanUseCase creates a SavePlanUseCase.
func NewSavePlanUseCase(
	documents Documents,
	plans domain.StudyPlanRepository,
	progress domain.ReadingProgressRepository,
) *SavePlanUseCase {
	return &SavePlanUseCase{documents: documents, plans: plans, progress: progress}
}

// Execute validates and stores one plan, superseding the document's active plan.
func (uc *SavePlanUseCase) Execute(
	ctx context.Context,
	input SavePlanInput,
) (StudyPlanView, error) {
	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return StudyPlanView{}, fmt.Errorf("document_id: %w", apperror.ErrInvalidInput)
	}

	mode, known := domain.ParseReadingMode(strings.TrimSpace(input.ReadingMode))
	if !known {
		return StudyPlanView{}, fmt.Errorf("reading_mode %q: %w", input.ReadingMode, apperror.ErrInvalidInput)
	}

	if len(input.Items) == 0 {
		return StudyPlanView{}, fmt.Errorf("items: %w", apperror.ErrInvalidInput)
	}

	items := make([]domain.StudyPlanItem, 0, len(input.Items))
	seenRanges := make(map[string]bool, len(input.Items))

	for index, item := range input.Items {
		position := index + 1

		title := strings.TrimSpace(item.Title)
		if title == "" {
			return StudyPlanView{}, fmt.Errorf("item %d: title: %w", position, apperror.ErrInvalidInput)
		}

		if item.FromPage < 1 {
			return StudyPlanView{}, fmt.Errorf(
				"item %d: from_page %d: %w", position, item.FromPage, apperror.ErrInvalidInput,
			)
		}

		if item.ToPage < item.FromPage {
			return StudyPlanView{}, fmt.Errorf(
				"item %d: to_page %d before from_page %d: %w",
				position, item.ToPage, item.FromPage, apperror.ErrInvalidInput,
			)
		}

		state := domain.PlanItemStatePending
		if raw := strings.TrimSpace(item.State); raw != "" {
			parsed, known := domain.ParsePlanItemState(raw)
			if !known {
				return StudyPlanView{}, fmt.Errorf(
					"item %d: state %q: %w", position, raw, apperror.ErrInvalidInput,
				)
			}
			state = parsed
		}

		// The same range twice is one item the reader would have to act on twice.
		// The transport names the pair; here it is only a refusal.
		key := rangeKey(item.FromPage, item.ToPage)
		if seenRanges[key] {
			return StudyPlanView{}, fmt.Errorf(
				"item %d: duplicate range %s: %w", position, key, apperror.ErrInvalidInput,
			)
		}
		seenRanges[key] = true

		items = append(items, domain.StudyPlanItem{
			ID:       uuid.NewString(),
			Position: position,
			Title:    title,
			FromPage: item.FromPage,
			ToPage:   item.ToPage,
			Note:     strings.TrimSpace(item.Note),
			State:    state,
		})
	}

	if _, err := uc.documents.Title(ctx, documentID); err != nil {
		return StudyPlanView{}, fmt.Errorf("look up document %q: %w", documentID, err)
	}

	plan := &domain.StudyPlan{
		ID:          uuid.NewString(),
		DocumentID:  documentID,
		ReadingMode: mode,
		Objective:   strings.TrimSpace(input.Objective),
		Source:      strings.TrimSpace(input.Source),
		CreatedAt:   domain.Now(),
		Items:       items,
	}

	stored, err := uc.plans.CreateActive(ctx, plan)
	if err != nil {
		return StudyPlanView{}, fmt.Errorf("save study plan: %w", err)
	}

	read, err := readRanges(ctx, uc.progress, documentID)
	if err != nil {
		return StudyPlanView{}, err
	}

	return planView(stored, read), nil
}

// GetPlanUseCase reads the active plan of one document.
type GetPlanUseCase struct {
	plans    domain.StudyPlanRepository
	progress domain.ReadingProgressRepository
}

// NewGetPlanUseCase creates a GetPlanUseCase.
func NewGetPlanUseCase(
	plans domain.StudyPlanRepository,
	progress domain.ReadingProgressRepository,
) *GetPlanUseCase {
	return &GetPlanUseCase{plans: plans, progress: progress}
}

// Execute returns the active plan with each item's derived read flag.
func (uc *GetPlanUseCase) Execute(ctx context.Context, documentID string) (StudyPlanView, error) {
	documentID = strings.TrimSpace(documentID)
	if documentID == "" {
		return StudyPlanView{}, fmt.Errorf("document_id: %w", apperror.ErrInvalidInput)
	}

	plan, err := uc.plans.FindActiveByDocument(ctx, documentID)
	if err != nil {
		return StudyPlanView{}, fmt.Errorf("find active plan: %w", err)
	}

	read, err := readRanges(ctx, uc.progress, documentID)
	if err != nil {
		return StudyPlanView{}, err
	}

	return planView(plan, read), nil
}

// readRanges returns the ranges of one document marked read, keyed by range.
//
// Only `read` rows are collected: an explicit `unread` is the default answer and
// needs no entry.
func readRanges(
	ctx context.Context,
	progress domain.ReadingProgressRepository,
	documentID string,
) (map[string]bool, error) {
	ranges, err := progress.ListByDocument(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("list reading progress: %w", err)
	}

	read := make(map[string]bool, len(ranges))
	for _, marked := range ranges {
		if marked.State == domain.ReadingStateRead {
			read[rangeKey(marked.FromPage, marked.ToPage)] = true
		}
	}

	return read, nil
}

// rangeKey is how coverage matches a plan item: the exact range, and nothing else.
//
// A wider or narrower read row is not this item. Overlap matching would claim a
// chapter was read because half of it was, which is the page-counter problem this
// model exists to avoid.
func rangeKey(from, to int) string {
	return strconv.Itoa(from) + "-" + strconv.Itoa(to)
}

func planView(plan *domain.StudyPlan, read map[string]bool) StudyPlanView {
	view := StudyPlanView{
		ID:          plan.ID,
		DocumentID:  plan.DocumentID,
		ReadingMode: plan.ReadingMode,
		Objective:   plan.Objective,
		Source:      plan.Source,
		CreatedAt:   plan.CreatedAt,
		Items:       make([]PlanItemView, 0, len(plan.Items)),
	}

	for _, item := range plan.Items {
		view.Items = append(view.Items, PlanItemView{
			StudyPlanItem: item,
			Read:          read[rangeKey(item.FromPage, item.ToPage)],
		})
	}

	return view
}
