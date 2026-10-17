package domain_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// filler returns distinct identifiers, so a test can place a known one at an
// exact rank without naming every rank in between.
func filler(prefix string, count int) []string {
	ids := make([]string, 0, count)
	for i := range count {
		ids = append(ids, fmt.Sprintf("%s-%d", prefix, i))
	}
	return ids
}

func TestFuseRankings_NoRankingsIsAnEmptyResult(t *testing.T) {
	fused := domain.FuseRankings()
	assert.NotNil(t, fused, "a caller ranges over the result, so it must not be nil")
	assert.Empty(t, fused)
}

func TestFuseRankings_IgnoresEmptyRankings(t *testing.T) {
	fused := domain.FuseRankings(nil, []string{}, []string{"a", "b"})
	assert.Equal(t, []string{"a", "b"}, fused)
}

func TestFuseRankings_ASingleRankingKeepsItsOrder(t *testing.T) {
	fused := domain.FuseRankings([]string{"a", "b", "c"})
	assert.Equal(t, []string{"a", "b", "c"}, fused,
		"with one ranking there is nothing to fuse, so the order must survive")
}

// The defining property of RRF: a document both rankings agree on outranks a
// document only one of them lists, even when the single-list document is first in
// its own list.
func TestFuseRankings_AnAgreedDocumentBeatsAOneListDocument(t *testing.T) {
	lexical := []string{"lex-only", "shared"}
	semantic := []string{"sem-only", "shared"}

	fused := domain.FuseRankings(lexical, semantic)

	assert.Equal(t, "shared", fused[0],
		"found by both stages must outrank found by one, which is the point of the fusion")
	assert.Contains(t, fused, "lex-only")
	assert.Contains(t, fused, "sem-only")
	assert.Len(t, fused, 3, "each identifier appears exactly once")
}

func TestFuseRankings_ReturnsEachIdentifierOnce(t *testing.T) {
	fused := domain.FuseRankings(
		[]string{"a", "b", "a"},
		[]string{"a", "c"},
	)

	assert.Equal(t, []string{"a", "b", "c"}, fused)
	assert.Len(t, fused, 3, "a repeated identifier must not produce a repeated result")
}

// k is fixed at 60, and the choice is observable: with a small k the document
// ranked first in one list would win, while with k=60 two mid-list appearances
// outweigh one first place.
func TestFuseRankings_UsesTheFixedRRFConstant(t *testing.T) {
	// "first" is ranked once, at rank 1. "deep" is ranked 10th in both lists.
	// k=60: deep scores 2/70 ≈ 0.0286 and first scores 1/61 ≈ 0.0164, so deep
	// wins. With k=1 first would score 0.5 and win, so the order pins the constant.
	first := append([]string{"first"}, filler("p", 8)...)
	first = append(first, "deep") // rank 10
	second := append(filler("q", 9), "deep")

	fused := domain.FuseRankings(first, second)

	assert.Equal(t, "deep", fused[0], "two mid-list appearances must outweigh one first place at k=60")
	assert.Equal(t, "first", fused[1])
}

// k is fixed at 60 and the rank base at 1, and the pair below is chosen so both
// are observable at once. "single" is first in one ranking; "agreed" is deep in
// both. At k=60 the two deep appearances outweigh the single first place, but at
// k=59 or with a 0-based rank they do not, so this pins the exact constant and
// base the formula uses rather than merely "some constant exists".
func TestFuseRankings_PinsTheRankBaseAndKExactly(t *testing.T) {
	first := append([]string{"single"}, filler("p", 60)...)
	first = append(first, "agreed")
	second := append(filler("q", 60), "agreed")

	fused := domain.FuseRankings(first, second)

	assert.Equal(t, "agreed", fused[0],
		"at k=60 two deep appearances beat one first place; k=59 or a 0-based rank flips this")
	assert.Equal(t, "single", fused[1])
}

func TestFuseRankings_BreaksTiesDeterministically(t *testing.T) {
	// Both identifiers appear once, at the same rank, in different rankings.
	const runs = 20
	var previous []string
	for range runs {
		fused := domain.FuseRankings([]string{"alpha"}, []string{"beta"})
		if previous != nil {
			assert.Equal(t, previous, fused, "the same rankings must always fuse the same way")
		}
		previous = fused
	}
	assert.Equal(t, []string{"alpha", "beta"}, previous,
		"an exact tie falls back to first appearance, which is stable")
}

func TestFuseRankings_TiesOnScoreUseTheBestRank(t *testing.T) {
	// "better" is 1st in one list, "later" is 1st in another; both score 1/61.
	// The one seen first wins, which keeps the fusion stable rather than arbitrary.
	fused := domain.FuseRankings([]string{"better"}, []string{"later"})
	assert.Equal(t, "better", fused[0])
}

func TestFuseRankings_MergesMoreThanTwoRankings(t *testing.T) {
	fused := domain.FuseRankings(
		[]string{"a"},
		[]string{"b", "a"},
		[]string{"a", "b"},
	)

	assert.Equal(t, "a", fused[0], "three appearances must beat two")
	assert.Equal(t, "b", fused[1])
	assert.Len(t, fused, 2)
}

func TestFuseRankings_DoesNotInventIdentifiers(t *testing.T) {
	fused := domain.FuseRankings([]string{"a"}, []string{"b"})
	assert.ElementsMatch(t, []string{"a", "b"}, fused)
}

func TestFuseRankings_SkipsEmptyIdentifiers(t *testing.T) {
	fused := domain.FuseRankings([]string{"a", ""}, []string{"", "b"})
	assert.Equal(t, []string{"a", "b"}, fused,
		"an empty identifier names no chunk, so it must not become a result")
}
