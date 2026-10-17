package domain_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// numberedWords builds n space-separated words of a fixed width, so a test can
// predict a paragraph's length and see exactly where a chunk boundary fell.
func numberedWords(prefix string, n int) string {
	words := make([]string, n)
	for i := range words {
		words[i] = fmt.Sprintf("%s%05d", prefix, i)
	}
	return strings.Join(words, " ")
}

// chunkOptions is small enough that a handful of numbered words crosses the
// target, and its overlap is 30% of the target so a carry is easy to see.
var chunkOptions = domain.ChunkOptions{TargetChars: 100, OverlapChars: 30}

// paragraphLength is the length of numberedWords(prefix, n): n six-character
// words joined by single spaces.
func paragraphLength(n int) int { return 6*n + n - 1 }

// pageWithParagraphs joins paragraphs the way stored page text separates them.
func pageWithParagraphs(paragraphs ...string) string {
	return strings.Join(paragraphs, "\n\n")
}

func TestChunkPage_AccumulatesParagraphsUpToTheTarget(t *testing.T) {
	first := numberedWords("a", 6)
	second := numberedWords("b", 6)
	third := numberedWords("c", 6)

	require.Less(t, len(first), chunkOptions.TargetChars, "precondition: one paragraph fits alone")
	require.LessOrEqual(t, len(first)+2+len(second), chunkOptions.TargetChars,
		"precondition: two paragraphs still fit together")
	require.Greater(t, len(first)+2+len(second)+2+len(third), chunkOptions.TargetChars,
		"precondition: three do not")

	chunks := domain.ChunkPage(pageWithParagraphs(first, second, third), 1, chunkOptions)
	require.Len(t, chunks, 2, "the page must break where the next paragraph would exceed the target")

	assert.Equal(t, pageWithParagraphs(first, second), chunks[0].Text,
		"a chunk packs every paragraph that still fits instead of stopping at the first")
	assert.Contains(t, chunks[1].Text, third)
	assert.LessOrEqual(t, len(chunks[0].Text), chunkOptions.TargetChars)
}

func TestChunkPage_CarriesAnOverlapIntoTheNextChunk(t *testing.T) {
	first := numberedWords("a", 6)
	second := numberedWords("b", 6)
	third := numberedWords("c", 6)
	require.Greater(t, paragraphLength(6), chunkOptions.OverlapChars,
		"precondition: the first chunk must be longer than the carry")

	chunks := domain.ChunkPage(pageWithParagraphs(first, second, third), 1, chunkOptions)
	require.Len(t, chunks, 2)

	carried, remainder, found := strings.Cut(chunks[1].Text, "\n\n")
	require.True(t, found, "the carry is separated from the chunk's own text")
	require.NotEmpty(t, carried, "the second chunk must repeat the tail of the first")
	assert.LessOrEqual(t, len(carried), chunkOptions.OverlapChars,
		"the carry is about OverlapChars, not the whole previous chunk")
	assert.Less(t, len(carried), len(chunks[0].Text),
		"a carry as long as the previous chunk would index the same text twice")
	assert.Contains(t, chunks[0].Text, carried,
		"the carried text must come from the previous chunk")
	assert.Equal(t, third, remainder, "the chunk's own text must survive the carry intact")
}

func TestChunkPage_EveryChunkBelongsToThePageItWasGiven(t *testing.T) {
	text := pageWithParagraphs(numberedWords("a", 6), numberedWords("b", 6), numberedWords("c", 6))

	chunks := domain.ChunkPage(text, 7, chunkOptions)
	require.NotEmpty(t, chunks)

	for _, chunk := range chunks {
		assert.Equal(t, 7, chunk.PageNumber,
			"a chunk that could name two pages could not be cited")
	}
}

func TestChunkPage_SplitsAParagraphLongerThanTheTarget(t *testing.T) {
	giant := numberedWords("g", 60)
	require.Greater(t, len(giant), chunkOptions.TargetChars,
		"precondition: the paragraph alone exceeds the target")

	chunks := domain.ChunkPage(giant, 1, chunkOptions)
	require.Greater(t, len(chunks), 1, "an over-long paragraph must not become one over-long chunk")

	for _, chunk := range chunks {
		assert.LessOrEqual(t, len(chunk.Text), chunkOptions.TargetChars+chunkOptions.OverlapChars,
			"the paragraph's own text stays under the target; only the carry is added on top")
		assert.NotEmpty(t, chunk.Text)
	}
	assert.Contains(t, chunks[0].Text, "g00000")
	assert.Contains(t, chunks[len(chunks)-1].Text, "g00059",
		"the paragraph's last words must be reachable")
}

func TestChunkPage_APageShorterThanTheTargetIsOneChunk(t *testing.T) {
	chunks := domain.ChunkPage("Chapter 4", 2, chunkOptions)

	require.Len(t, chunks, 1, "a short page must still be searchable")
	assert.Equal(t, "Chapter 4", chunks[0].Text)
	assert.Equal(t, 2, chunks[0].PageNumber)
	assert.Equal(t, 0, chunks[0].Ordinal)
}

func TestChunkPage_AnEmptyOrWhitespaceOnlyPageYieldsNothing(t *testing.T) {
	for name, text := range map[string]string{
		"empty":            "",
		"blank lines":      "\n\n\n",
		"spaces and tabs":  "   \n\t\t\n   \n",
		"windows newlines": "\r\n \r\n\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			chunks := domain.ChunkPage(text, 1, chunkOptions)
			assert.Empty(t, chunks, "a page with no text must produce no chunk to index")
		})
	}
}

func TestChunkPage_OrdinalsStartAtZeroAndAreContiguous(t *testing.T) {
	text := pageWithParagraphs(
		numberedWords("a", 6),
		numberedWords("b", 6),
		numberedWords("c", 6),
		numberedWords("d", 6),
		numberedWords("e", 6),
	)

	chunks := domain.ChunkPage(text, 3, chunkOptions)
	require.Greater(t, len(chunks), 2, "precondition: the page must produce several chunks")

	for ordinal, chunk := range chunks {
		assert.Equal(t, ordinal, chunk.Ordinal,
			"ordinals must be contiguous and start at 0, because they are the chunk's position on the page")
	}
}

func TestChunkPage_IsDeterministic(t *testing.T) {
	text := pageWithParagraphs(
		numberedWords("a", 6),
		numberedWords("b", 6),
		numberedWords("c", 6),
		numberedWords("d", 6),
		numberedWords("e", 6),
	)

	first := domain.ChunkPage(text, 5, chunkOptions)
	second := domain.ChunkPage(text, 5, chunkOptions)

	assert.Equal(t, first, second, "the same page must always yield the same chunks in the same order")
}

func TestChunkPage_ZeroOptionsUseTheSpecifiedDefaults(t *testing.T) {
	require.Equal(t, 1200, domain.DefaultChunkTargetChars)
	require.Equal(t, 180, domain.DefaultChunkOverlapChars)

	text := strings.TrimSpace(strings.Repeat("palabra ", 300))
	chunks := domain.ChunkPage(text, 1, domain.ChunkOptions{})
	require.Greater(t, len(chunks), 1, "300 short words exceed the 1200-character default target")

	for _, chunk := range chunks {
		assert.LessOrEqual(t, len(chunk.Text), domain.DefaultChunkTargetChars+domain.DefaultChunkOverlapChars)
	}
}
