package app

import (
	"context"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// DefaultSearchLimit is how many results a search returns when the caller does
// not choose otherwise. It bounds one printed page of output.
const DefaultSearchLimit = 10

// SearchUseCase answers a lexical query over the chunk index.
//
// It is read-only and has no other ports: search reads the index and nothing
// else, so it cannot need an API key, a PDF, or a storage root.
type SearchUseCase struct {
	chunks domain.ChunkRepository
}

// NewSearchUseCase wires a search.
func NewSearchUseCase(chunks domain.ChunkRepository) *SearchUseCase {
	return &SearchUseCase{chunks: chunks}
}

// Search returns the chunks matching query, best first.
//
// A limit below 1 means DefaultSearchLimit, so a caller that passes a zero value
// gets the default rather than a database asking for an unlimited result set. A
// query of only punctuation and a query that matches nothing both return an empty
// slice and no error, and the slice is never nil: a caller ranges over the result,
// and "no matches" must not be something it has to special-case.
func (uc *SearchUseCase) Search(ctx context.Context, query string, limit int) ([]domain.SearchResult, error) {
	if limit < 1 {
		limit = DefaultSearchLimit
	}

	results, err := uc.chunks.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	if results == nil {
		results = []domain.SearchResult{}
	}

	return results, nil
}
