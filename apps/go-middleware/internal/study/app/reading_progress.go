package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// SetReadingProgressInput is one request to mark a page range.
//
// State travels as the wire value, not as the domain type: deciding whether a
// string names a state this application knows is part of validating the request,
// and the transport only decides whether the body was JSON with those fields.
type SetReadingProgressInput struct {
	DocumentID string

	// FromPage and ToPage are 1-based and inclusive.
	FromPage int
	ToPage   int

	State string
}

// SetReadingProgressUseCase records whether the reader read one page range.
//
// Marking a range is a claim, not a proof, and this use case treats it that way:
// one call writes the state, and there is no second step, no confirmation, and no
// verification of having read. See the backlog's W4 note on why a gate here would
// be the wrong instrument.
type SetReadingProgressUseCase struct {
	progress domain.ReadingProgressRepository
}

// NewSetReadingProgressUseCase creates a SetReadingProgressUseCase.
func NewSetReadingProgressUseCase(progress domain.ReadingProgressRepository) *SetReadingProgressUseCase {
	return &SetReadingProgressUseCase{progress: progress}
}

// Execute stores the state of one range and returns the row as it was stored.
//
// The returned row is the repository's, not the one built here, because created_at
// belongs to the first marking of the range: a caller that echoed its own instant
// would report a created_at that moves on every write.
func (uc *SetReadingProgressUseCase) Execute(
	ctx context.Context,
	input SetReadingProgressInput,
) (domain.ReadingProgress, error) {
	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return domain.ReadingProgress{}, fmt.Errorf("document_id: %w", apperror.ErrInvalidInput)
	}

	if input.FromPage < 1 {
		return domain.ReadingProgress{}, fmt.Errorf("from_page %d: %w", input.FromPage, apperror.ErrInvalidInput)
	}

	if input.ToPage < input.FromPage {
		return domain.ReadingProgress{}, fmt.Errorf(
			"to_page %d before from_page %d: %w", input.ToPage, input.FromPage, apperror.ErrInvalidInput,
		)
	}

	state, known := domain.ParseReadingState(input.State)
	if !known {
		return domain.ReadingProgress{}, fmt.Errorf("state %q: %w", input.State, apperror.ErrInvalidInput)
	}

	now := domain.Now()
	progress := &domain.ReadingProgress{
		ID:         uuid.NewString(),
		DocumentID: documentID,
		FromPage:   input.FromPage,
		ToPage:     input.ToPage,
		State:      state,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	stored, err := uc.progress.Upsert(ctx, progress)
	if err != nil {
		return domain.ReadingProgress{}, fmt.Errorf("set reading progress: %w", err)
	}

	return *stored, nil
}

// ListReadingProgressUseCase reads the ranges of one document.
type ListReadingProgressUseCase struct {
	progress domain.ReadingProgressRepository
}

// NewListReadingProgressUseCase creates a ListReadingProgressUseCase.
func NewListReadingProgressUseCase(progress domain.ReadingProgressRepository) *ListReadingProgressUseCase {
	return &ListReadingProgressUseCase{progress: progress}
}

// Execute returns one document's ranges in page order, as an empty slice when the
// document has none: a book nobody marked is a normal state, not an error.
func (uc *ListReadingProgressUseCase) Execute(
	ctx context.Context,
	documentID string,
) ([]domain.ReadingProgress, error) {
	documentID = strings.TrimSpace(documentID)
	if documentID == "" {
		return nil, fmt.Errorf("document_id: %w", apperror.ErrInvalidInput)
	}

	ranges, err := uc.progress.ListByDocument(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("list reading progress: %w", err)
	}

	return ranges, nil
}
