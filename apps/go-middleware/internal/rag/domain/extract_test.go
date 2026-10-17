package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

func TestNeedsModel(t *testing.T) {
	exactly := func(runes int) string { return repeat("a", runes) }

	cases := []struct {
		name      string
		extracted string
		images    int
		wantModel bool
		reasoning string
	}{
		{
			name:      "empty text needs the model",
			extracted: "",
			images:    0,
			wantModel: true,
			reasoning: "a page that produced nothing is the clearest case for a second look",
		},
		{
			name:      "whitespace only needs the model",
			extracted: " \n\t\f  ",
			images:    0,
			wantModel: true,
			reasoning: "trimming happens before counting, so whitespace is empty text",
		},
		{
			name:      "one below the threshold needs the model",
			extracted: exactly(domain.MinExtractableText - 1),
			images:    0,
			wantModel: true,
			reasoning: "199 runes is not a page's worth of content",
		},
		{
			name:      "exactly at the threshold is trusted",
			extracted: exactly(domain.MinExtractableText),
			images:    0,
			wantModel: false,
			reasoning: "the comparison is strict: the threshold itself is enough",
		},
		{
			name:      "long text with no image is trusted",
			extracted: repeat("word ", 400),
			images:    0,
			wantModel: false,
			reasoning: "the common case: a normal text page stays with the extractor",
		},
		{
			name:      "long text with an image still needs the model",
			extracted: repeat("word ", 400),
			images:    1,
			wantModel: true,
			reasoning: "a figure is worth looking at regardless of how much text surrounds it",
		},
		{
			name:      "surrounding whitespace does not count toward the threshold",
			extracted: "   " + exactly(domain.MinExtractableText-1) + "   ",
			images:    0,
			wantModel: true,
			reasoning: "the trimmed length is 199 runes even though the raw length passes it",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantModel, domain.NeedsModel(tc.extracted, tc.images), tc.reasoning)
		})
	}
}

// TestNeedsModel_CountsRunesNotBytes pins the multibyte case: a page of accented
// or CJK text whose byte length already passes the threshold must not be sent to
// the model on that basis alone.
func TestNeedsModel_CountsRunesNotBytes(t *testing.T) {
	short := repeat("é", domain.MinExtractableText-1)
	assert.Greater(t, len(short), domain.MinExtractableText, "the fixture must exceed the threshold in bytes")
	assert.True(t, domain.NeedsModel(short, 0), "199 runes is below the threshold even though its byte length is not")

	atThreshold := repeat("é", domain.MinExtractableText)
	assert.False(t, domain.NeedsModel(atThreshold, 0), "200 runes reaches the threshold regardless of byte length")
}

func repeat(value string, count int) string {
	out := ""
	for range count {
		out += value
	}
	return out
}
