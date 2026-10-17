package http

import (
	"context"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// The HTTP layer declares the use case contracts it needs, rather than
// importing concrete use case types. That keeps handlers mockable and lets the
// application layer change shape without breaking transport tests. This mirrors
// study/http/contracts.go.

// SearchUseCase answers a query over the chunk index.
//
// Both the lexical and the hybrid use cases satisfy it: the difference is which
// stages they run, not the contract the transport sees.
type SearchUseCase interface {
	// Search returns the chunks matching query, best first. A query of only
	// punctuation and a query that matches nothing both return an empty slice and
	// no error, so they are a 200 with an empty array and never a 400.
	Search(ctx context.Context, query string, limit int) ([]domain.SearchResult, error)
}

// MaxSearchLimit bounds the `limit` query parameter.
//
// A request is a caller this process does not control, so the limit is bounded
// here rather than passed through: without a bound, one client could ask for the
// whole index and make every search answer with the corpus. Absent or empty uses
// app.DefaultSearchLimit.
const MaxSearchLimit = 50
