package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

func TestNewHeading_TrimsTheTitleAndKeepsThePrintedPage(t *testing.T) {
	heading, err := domain.NewHeading(domain.Heading{Level: 2, Title: "  Crunching   Knowledge ", Page: 15})
	require.NoError(t, err)

	assert.Equal(t, domain.Heading{Level: 2, Title: "Crunching Knowledge", Page: 15}, heading)
}

func TestNewHeading_RejectsAnImpossibleHeading(t *testing.T) {
	tests := []struct {
		name    string
		heading domain.Heading
	}{
		{"blank title", domain.Heading{Level: 1, Title: "   ", Page: 1}},
		{"level below one", domain.Heading{Level: 0, Title: "Preface", Page: 1}},
		{"page below one", domain.Heading{Level: 1, Title: "Preface", Page: 0}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := domain.NewHeading(test.heading)
			assert.ErrorIs(t, err, apperror.ErrInvalidInput)
		})
	}
}
