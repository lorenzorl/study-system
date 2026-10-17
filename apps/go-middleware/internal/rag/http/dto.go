package http

// SearchResultResponse is one result in the bare JSON array returned by
// GET /api/rag/search.
//
// The wire shape carries the citation and the two scores the command prints, so
// the two surfaces do not disagree about what a result is or about which
// direction a score points.
type SearchResultResponse struct {
	// ChunkID identifies the chunk.
	ChunkID string `json:"chunk_id"`

	// DocumentID identifies the book the chunk came from.
	DocumentID string `json:"document_id"`

	// DocumentTitle is the document's title.
	DocumentTitle string `json:"document_title"`

	// PageNumber is the 1-based page to open. A chunk never spans two pages, so
	// this is exact.
	PageNumber int `json:"page_number"`

	// Snippet is the bounded window of the chunk's text with the matched terms
	// marked, folded onto one line.
	Snippet string `json:"snippet"`

	// Stage names the retrieval stage that produced the result: lexical,
	// semantic, or hybrid.
	Stage string `json:"stage"`

	// BM25 is the lexical score's MAGNITUDE: bigger is better. The index's bm25 is
	// smaller-is-better, so reporting the magnitude keeps this field's direction
	// the same as Similarity's and the same as the command's output. It is 0 when
	// the lexical stage did not rank the chunk.
	BM25 float64 `json:"bm25"`

	// Similarity is the semantic cosine in [-1, 1]: bigger is better. It is 0 when
	// the semantic stage did not rank the chunk.
	Similarity float64 `json:"similarity"`
}
