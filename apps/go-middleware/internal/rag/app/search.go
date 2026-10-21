package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// DefaultSearchLimit is how many results a search returns when the caller does
// not choose otherwise. It bounds one printed page of output.
const DefaultSearchLimit = 10

// semanticSnippetRunes bounds a semantic result's snippet.
//
// The lexical stage gets a snippet from FTS5 with the matched terms marked. A
// semantic hit matched no term, so there is nothing to mark and the snippet is
// just a bounded window of the chunk's text, folded onto one line for the same
// reason: the command prints one line per result.
const semanticSnippetRunes = 240

// ChunkSearchRepository is the slice of the chunk index the search use case
// reads: the whole-corpus ranking, and the same ranking narrowed to one
// document.
//
// The document filter belongs to the port rather than being applied to the
// results in Go, because only the index can apply it before the limit. A filter
// applied here would let the limit trim the unfiltered result set first and would
// silently change what "the first N results" means.
type ChunkSearchRepository interface {
	// Search returns the chunks matching a query over the whole corpus, best
	// first.
	Search(ctx context.Context, query string, limit int) ([]domain.SearchResult, error)

	// SearchInDocument returns the chunks of one document matching a query, best
	// first. An empty documentID is the whole corpus.
	SearchInDocument(ctx context.Context, query, documentID string, limit int) ([]domain.SearchResult, error)
}

// SectionLookup answers which section owns a page.
//
// It is a separate port from the search repository on purpose: the ranking is the
// index's answer and the attribution is the document's, and a search built without
// one still ranks exactly as it did before this feature existed.
type SectionLookup interface {
	SectionAt(ctx context.Context, documentID string, page int) (*domain.Section, error)
}

// SearchUseCase answers a query over the chunk index.
//
// The lexical stage is always available and reads only the database. The semantic
// stage is optional: it needs stored vectors and an embedder, and it is the only
// stage that can cross the language boundary between a Spanish query and an
// English book.
type SearchUseCase struct {
	chunks ChunkSearchRepository

	// embeddings and embedder are nil in a lexical-only search. Keeping them
	// separate from chunks is what lets the same use case work on a machine with
	// no embedding credential and no network.
	embeddings domain.EmbeddingRepository
	embedder   domain.Embedder

	// model filters the stored vectors to the model the query was embedded with.
	model string

	// sections attributes a result to the section that owns its page. It is nil
	// when no structure is wired in, and then a result carries no section and
	// answers exactly as it did before.
	sections SectionLookup
}

// NewSearchUseCase wires a lexical-only search.
func NewSearchUseCase(chunks ChunkSearchRepository) *SearchUseCase {
	return &SearchUseCase{chunks: chunks}
}

// NewHybridSearchUseCase wires a search that fuses the lexical and semantic
// rankings.
//
// A nil embeddings repository or a nil embedder degrades to a lexical-only search
// rather than failing, which is what lets the command stay usable without an API
// key.
func NewHybridSearchUseCase(
	chunks ChunkSearchRepository,
	embeddings domain.EmbeddingRepository,
	embedder domain.Embedder,
	model string,
) *SearchUseCase {
	return &SearchUseCase{
		chunks:     chunks,
		embeddings: embeddings,
		embedder:   embedder,
		model:      model,
	}
}

// WithSections adds attribution: every result then names the section that owns its
// page, or nothing when the document has no confirmed structure.
//
// It is a builder rather than a constructor argument so the two existing
// constructors keep their shape: a caller that does not want attribution writes
// the same line it wrote before.
func (uc *SearchUseCase) WithSections(sections SectionLookup) *SearchUseCase {
	uc.sections = sections
	return uc
}

// semanticEnabled reports whether the semantic stage has everything it needs.
func (uc *SearchUseCase) semanticEnabled() bool {
	return uc.embeddings != nil && uc.embedder != nil
}

// Search returns the chunks matching query, best first.
//
// A limit below 1 means DefaultSearchLimit. A query of only punctuation and a
// query that matches nothing both return an empty slice and no error, and the
// slice is never nil: a caller ranges over the result, and "no matches" must not
// be something it has to special-case.
//
// The semantic stage runs only when vectors are stored for the configured model
// and the query holds something a model can embed. When the model has no stored
// vectors, search falls back to lexical-only rather than failing: a machine with
// no embeddings still searches.
func (uc *SearchUseCase) Search(ctx context.Context, query string, limit int) ([]domain.SearchResult, error) {
	return uc.search(ctx, query, "", limit)
}

// SearchInDocument returns the chunks of one document matching query, best
// first.
//
// The filter narrows both retrieval stages, so a scoped search answers from the
// document the caller chose or from nothing. An empty documentID is the whole
// corpus, exactly as Search. A document that matches nothing is the same empty
// slice a query with no match returns: it is not an error, and the transport
// keeps answering 200 with an empty array.
func (uc *SearchUseCase) SearchInDocument(
	ctx context.Context,
	query, documentID string,
	limit int,
) ([]domain.SearchResult, error) {
	if documentID == "" {
		return uc.Search(ctx, query, limit)
	}
	return uc.search(ctx, query, documentID, limit)
}

// search runs both stages under one optional document filter. An empty
// documentID leaves the lexical stage on the whole-corpus query and the semantic
// stage unfiltered, which is the endpoint's behaviour before the parameter
// existed.
func (uc *SearchUseCase) search(
	ctx context.Context,
	query, documentID string,
	limit int,
) ([]domain.SearchResult, error) {
	if limit < 1 {
		limit = DefaultSearchLimit
	}

	lexical, err := uc.lexical(ctx, query, documentID, limit)
	if err != nil {
		return nil, err
	}
	if lexical == nil {
		lexical = []domain.SearchResult{}
	}
	withStage(lexical, domain.StageLexical)

	// A punctuation-only query has no meaning to embed, and embedding it would
	// return a neighbour that no reader asked for. The lexical stage already
	// answered it with nothing.
	if !uc.semanticEnabled() || !domain.HasIndexableText(query) {
		return uc.attribute(ctx, lexical), nil
	}

	semantic, err := uc.semanticRanking(ctx, query, documentID, limit)
	if err != nil {
		return nil, err
	}
	if len(semantic) == 0 {
		return uc.attribute(ctx, lexical), nil
	}

	return uc.attribute(ctx, fuseResults(lexical, semantic, limit)), nil
}

// attribute names the section that owns each result's page.
//
// A page no section owns is the normal answer for a document nobody could
// structure, and a failed lookup is not: it is a broken read, and it costs the
// citation rather than the search. The same asymmetry the structure stage keeps
// (S5) — a book must still answer — applies one layer up, where the reader would
// otherwise lose a page of results to a section table that is slow or locked.
func (uc *SearchUseCase) attribute(ctx context.Context, results []domain.SearchResult) []domain.SearchResult {
	if uc.sections == nil {
		return results
	}

	for index := range results {
		section, err := uc.sections.SectionAt(ctx, results[index].DocumentID, results[index].PageNumber)
		if err != nil {
			continue
		}
		results[index].SectionTitle = section.Title
		results[index].SectionLevel = section.Level
	}
	return results
}

// lexical runs the ranked query over the whole corpus or over one document. The
// index applies the filter, so the limit bounds the same set it is applied to.
func (uc *SearchUseCase) lexical(
	ctx context.Context,
	query, documentID string,
	limit int,
) ([]domain.SearchResult, error) {
	if documentID == "" {
		return uc.chunks.Search(ctx, query, limit)
	}
	return uc.chunks.SearchInDocument(ctx, query, documentID, limit)
}

// semanticRanking embeds the query and ranks every stored vector of the model by
// cosine similarity, best first.
//
// A non-positive similarity is not a candidate: an orthogonal or opposite vector
// has no shared direction with the query, and returning it would turn an
// unrelated query into a page of weak results. The ranking is brute force over
// every stored vector, which the corpus size makes milliseconds.
//
// A non-empty documentID narrows the ranking to that document, applied before
// the top-N truncation so the limit bounds the filtered set rather than the whole
// corpus.
func (uc *SearchUseCase) semanticRanking(
	ctx context.Context,
	query, documentID string,
	limit int,
) ([]domain.SearchResult, error) {
	stored, err := uc.embeddings.ListByModel(ctx, uc.model)
	if err != nil {
		return nil, fmt.Errorf("list the stored vectors of model %s: %w", uc.model, err)
	}
	if len(stored) == 0 {
		return nil, nil
	}

	vectors, err := uc.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed the query: %w", err)
	}
	if len(vectors) != 1 || len(vectors[0]) == 0 {
		return nil, fmt.Errorf("embed the query: expected one non-empty vector, got %d: %w",
			len(vectors), apperror.ErrInvalidInput)
	}
	queryVector := vectors[0]

	ranked := make([]domain.SearchResult, 0, len(stored))
	for _, item := range stored {
		if documentID != "" && item.DocumentID != documentID {
			// The semantic stage reads every stored vector, so the filter is
			// applied here, before the similarity ranking is truncated: the limit
			// must bound the document's matches, not the whole corpus's.
			continue
		}

		similarity, err := domain.Cosine(queryVector, item.Vector)
		if err != nil {
			return nil, fmt.Errorf("compare the query with chunk %s: %w", item.ChunkID, err)
		}
		if similarity <= 0 {
			continue
		}

		ranked = append(ranked, domain.SearchResult{
			ChunkID:       item.ChunkID,
			DocumentID:    item.DocumentID,
			DocumentTitle: item.DocumentTitle,
			PageNumber:    item.PageNumber,
			Snippet:       excerpt(item.Text),
			Text:          item.Text,
			Stage:         domain.StageSemantic,
			Similarity:    similarity,
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Similarity != ranked[j].Similarity {
			return ranked[i].Similarity > ranked[j].Similarity
		}
		// A deterministic tiebreak, so the same corpus and query always answer the
		// same way.
		return ranked[i].ChunkID < ranked[j].ChunkID
	})

	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	return ranked, nil
}

// fuseResults merges the two rankings with Reciprocal Rank Fusion and assembles
// the result list.
//
// A lexical hit keeps its marked snippet; a semantic-only hit gets a plain window
// of its text. A chunk both stages found is marked hybrid and keeps the lexical
// snippet, because that is the one with the matched term visible.
func fuseResults(lexical, semantic []domain.SearchResult, limit int) []domain.SearchResult {
	lexicalByID := make(map[string]domain.SearchResult, len(lexical))
	lexicalIDs := make([]string, 0, len(lexical))
	for _, result := range lexical {
		lexicalByID[result.ChunkID] = result
		lexicalIDs = append(lexicalIDs, result.ChunkID)
	}

	semanticByID := make(map[string]domain.SearchResult, len(semantic))
	semanticIDs := make([]string, 0, len(semantic))
	for _, result := range semantic {
		semanticByID[result.ChunkID] = result
		semanticIDs = append(semanticIDs, result.ChunkID)
	}

	fused := make([]domain.SearchResult, 0, limit)
	for _, chunkID := range domain.FuseRankings(lexicalIDs, semanticIDs) {
		if len(fused) >= limit {
			break
		}

		lexicalResult, inLexical := lexicalByID[chunkID]
		semanticResult, inSemantic := semanticByID[chunkID]

		switch {
		case inLexical && inSemantic:
			lexicalResult.Stage = domain.StageHybrid
			lexicalResult.Similarity = semanticResult.Similarity
			fused = append(fused, lexicalResult)

		case inLexical:
			lexicalResult.Stage = domain.StageLexical
			fused = append(fused, lexicalResult)

		default:
			semanticResult.Stage = domain.StageSemantic
			fused = append(fused, semanticResult)
		}
	}

	return fused
}

// withStage stamps the stage on a ranking the lexical stage produced.
func withStage(results []domain.SearchResult, stage domain.MatchStage) {
	for i := range results {
		results[i].Stage = stage
	}
}

// excerpt folds text onto one line and bounds it, so a semantic snippet fits the
// one-line-per-result output the command prints.
func excerpt(text string) string {
	folded := strings.Join(strings.Fields(text), " ")

	runes := []rune(folded)
	if len(runes) <= semanticSnippetRunes {
		return folded
	}

	return strings.TrimSpace(string(runes[:semanticSnippetRunes])) + "…"
}
