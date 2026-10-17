package domain

import "sort"

// RRFK is the constant of the Reciprocal Rank Fusion used to merge rankings.
//
// k damps the influence of the very top ranks: without it a single first place
// would dominate, and with it a document several rankings agree on outranks a
// document only one ranking likes. 60 is the value the retrieval literature
// settles on and the slice-5.2 contract fixes.
const RRFK = 60.0

// FuseRankings merges ranked lists of identifiers into one ranking with
// Reciprocal Rank Fusion, best first.
//
// RRF fuses ranks and not scores on purpose: the lexical stage's bm25 is
// smaller-is-better and negative, while the semantic stage's cosine is
// larger-is-better and lives in [-1, 1]. Any score-normalising fusion would have
// to invent a common scale between two quantities that do not share one; a rank
// is a position, and positions are already comparable.
//
// An identifier's score is the sum of 1/(k + rank) over the rankings that list
// it, with rank starting at 1. An identifier repeated inside one ranking counts
// once per ranking, so a single list cannot dominate by repeating itself. Ties
// are broken by best rank and then by first appearance, which makes the fusion a
// pure function of its input: the same rankings always fuse the same way.
//
// The result contains every identifier of every ranking exactly once, and it is
// never nil: a caller ranges over it.
func FuseRankings(rankings ...[]string) []string {
	type entry struct {
		score    float64
		bestRank int
		first    int
	}

	entries := make(map[string]*entry)
	next := 0

	for _, ranking := range rankings {
		seen := make(map[string]bool, len(ranking))
		for position, id := range ranking {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true

			existing, ok := entries[id]
			if !ok {
				entries[id] = &entry{score: rankWeight(position), bestRank: position, first: next}
				next++
				continue
			}
			existing.score += rankWeight(position)
			if position < existing.bestRank {
				existing.bestRank = position
			}
		}
	}

	fused := make([]string, 0, len(entries))
	for id := range entries {
		fused = append(fused, id)
	}

	sort.SliceStable(fused, func(i, j int) bool {
		a, b := entries[fused[i]], entries[fused[j]]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.bestRank != b.bestRank {
			return a.bestRank < b.bestRank
		}
		return a.first < b.first
	})

	return fused
}

// rankWeight is 1/(k + rank) for a 0-based position, so position 0 is rank 1.
func rankWeight(position int) float64 {
	return 1.0 / (RRFK + float64(position) + 1)
}
