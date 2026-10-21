package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// sectionInstant is a fixed instant: a test pinned to "now" would pass on a
// machine whose zone handling is wrong.
var sectionInstant = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// sectionDraft is a valid section. Every rejection test varies exactly one
// field, so a failure names the rule it broke.
func sectionDraft() domain.Section {
	return domain.Section{
		ID:         "section-1",
		DocumentID: "doc-1",
		Level:      2,
		Title:      "Crunching Knowledge",
		StartPage:  15,
		EndPage:    23,
		Source:     domain.SourceTOC,
	}
}

func TestNewSection_StampsTheInstantAndTrimsTheTitle(t *testing.T) {
	draft := sectionDraft()
	draft.ParentID = "section-0"
	draft.Title = "  Crunching Knowledge  "

	section, err := domain.NewSection(draft, sectionInstant)
	require.NoError(t, err)

	assert.Equal(t, "section-1", section.ID)
	assert.Equal(t, "section-0", section.ParentID)
	assert.Equal(t, "Crunching Knowledge", section.Title, "a title is stored trimmed")
	assert.True(t, section.CreatedAt.Equal(sectionInstant), "the instant must survive")
	assert.Equal(t, time.UTC, section.CreatedAt.Location(), "instants are stored in UTC")
}

func TestNewSection_AcceptsEverySourceTheExtractorCanReport(t *testing.T) {
	sources := []domain.SectionSource{
		domain.SourceTOC,
		domain.SourceOutline,
		domain.SourceTypography,
		domain.SourcePattern,
		domain.SourceMixed,
	}

	for _, source := range sources {
		draft := sectionDraft()
		draft.Source = source

		_, err := domain.NewSection(draft, sectionInstant)
		assert.NoError(t, err, "source %q must be accepted", source)
	}
}

func TestNewSection_AcceptsASectionThatOwnsOnePage(t *testing.T) {
	draft := sectionDraft()
	draft.EndPage = draft.StartPage

	_, err := domain.NewSection(draft, sectionInstant)
	assert.NoError(t, err, "a section that owns one page is a valid range")
}

func TestNewSection_RejectsAnImpossibleSection(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*domain.Section)
	}{
		{"no id", func(section *domain.Section) { section.ID = "" }},
		{"no document", func(section *domain.Section) { section.DocumentID = "" }},
		{"blank title", func(section *domain.Section) { section.Title = "   " }},
		{"level below one", func(section *domain.Section) { section.Level = 0 }},
		{"page below one", func(section *domain.Section) { section.StartPage = 0 }},
		{"range ends before it starts", func(section *domain.Section) { section.EndPage = section.StartPage - 1 }},
		{"its own parent", func(section *domain.Section) { section.ParentID = section.ID }},
		{"no source", func(section *domain.Section) { section.Source = "" }},
		{"unknown source", func(section *domain.Section) { section.Source = "guessed" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			draft := sectionDraft()
			test.mutate(&draft)

			_, err := domain.NewSection(draft, sectionInstant)
			assert.ErrorIs(t, err, apperror.ErrInvalidInput)
		})
	}
}
