package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// fakeReadingProgressRepository records what the use case asked it to store, so
// a refusal can prove nothing was written.
type fakeReadingProgressRepository struct {
	stored []domain.ReadingProgress
	ranges []domain.ReadingProgress
	err    error
}

func (f *fakeReadingProgressRepository) Upsert(
	ctx context.Context,
	progress *domain.ReadingProgress,
) (*domain.ReadingProgress, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.stored = append(f.stored, *progress)
	return progress, nil
}

func (f *fakeReadingProgressRepository) ListByDocument(ctx context.Context, documentID string) ([]domain.ReadingProgress, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ranges, nil
}

func (f *fakeReadingProgressRepository) ListRead(ctx context.Context) ([]domain.ReadingProgress, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ranges, nil
}

// S2 and S4: what the application refuses, it refuses before writing.
func TestSetReadingProgress_RefusesWhatTheContractNames(t *testing.T) {
	valid := app.SetReadingProgressInput{DocumentID: "doc-1", FromPage: 24, ToPage: 36, State: "read"}

	cases := []struct {
		name  string
		input app.SetReadingProgressInput
	}{
		{name: "empty document", input: func() app.SetReadingProgressInput { in := valid; in.DocumentID = "  "; return in }()},
		{name: "page zero", input: func() app.SetReadingProgressInput { in := valid; in.FromPage = 0; return in }()},
		{name: "page below zero", input: func() app.SetReadingProgressInput { in := valid; in.FromPage = -3; return in }()},
		{name: "backwards range", input: func() app.SetReadingProgressInput { in := valid; in.FromPage = 40; return in }()},
		{name: "unknown state", input: func() app.SetReadingProgressInput { in := valid; in.State = "reading"; return in }()},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			repo := &fakeReadingProgressRepository{}
			useCase := app.NewSetReadingProgressUseCase(repo)

			_, err := useCase.Execute(t.Context(), testCase.input)

			assert.ErrorIs(t, err, apperror.ErrInvalidInput)
			assert.Empty(t, repo.stored, "a refused range must not be stored")
		})
	}
}

// S3: marking a range read stores exactly the range and state it was given.
func TestSetReadingProgress_StoresTheRangeItWasGiven(t *testing.T) {
	repo := &fakeReadingProgressRepository{}
	useCase := app.NewSetReadingProgressUseCase(repo)

	progress, err := useCase.Execute(t.Context(), app.SetReadingProgressInput{
		DocumentID: " doc-1 ", FromPage: 24, ToPage: 36, State: "read",
	})
	require.NoError(t, err)

	require.Len(t, repo.stored, 1)
	assert.Equal(t, "doc-1", repo.stored[0].DocumentID, "the identifier is trimmed, not stored with spaces")
	assert.Equal(t, 24, repo.stored[0].FromPage)
	assert.Equal(t, 36, repo.stored[0].ToPage)
	assert.Equal(t, domain.ReadingStateRead, repo.stored[0].State)
	assert.False(t, progress.CreatedAt.IsZero())
	assert.Equal(t, progress.CreatedAt, progress.UpdatedAt, "a first marking has one instant")
	assert.WithinDuration(t, domain.Now(), progress.CreatedAt, time.Minute)
}

// S5: the list read answers the document it was asked about and never invents
// an error for an empty result.
func TestListReadingProgress_ReturnsWhatTheRepositoryHolds(t *testing.T) {
	repo := &fakeReadingProgressRepository{ranges: []domain.ReadingProgress{
		{DocumentID: "doc-1", FromPage: 24, ToPage: 36, State: domain.ReadingStateRead},
	}}
	useCase := app.NewListReadingProgressUseCase(repo)

	ranges, err := useCase.Execute(t.Context(), "doc-1")
	require.NoError(t, err)
	assert.Len(t, ranges, 1)
}
