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

	// SectionTitle is the deepest section that owns PageNumber, empty when the
	// document has no confirmed structure.
	SectionTitle string `json:"section_title,omitempty"`

	// SectionLevel is that section's nesting, absent when there is none.
	SectionLevel int `json:"section_level,omitempty"`

	// Snippet is the bounded window of the chunk's text with the matched terms
	// marked, folded onto one line.
	Snippet string `json:"snippet"`

	// Text is the chunk's text, verbatim, for a client that builds a prompt from
	// it. It is longer than the snippet by design: the snippet is what a reader
	// scans, the text is what a model reads.
	Text string `json:"text"`

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

// UploadDocumentResponse is the body of the 202 returned by
// POST /api/rag/documents.
type UploadDocumentResponse struct {
	// DocumentID identifies the document the bytes belong to. The same bytes
	// always resolve to the same document, so a re-upload resumes it.
	DocumentID string `json:"document_id"`

	// State is the processing state the document starts in. It is always
	// "queued": the request returns before any page is attempted.
	State string `json:"state"`
}

// IngestTextRequest is the body of POST /api/rag/documents/text.
//
// Both fields are trimmed before they are validated, so a paste that carries a
// stray leading newline is not refused for being blank.
type IngestTextRequest struct {
	// Title is the material's name in the corpus list.
	Title string `json:"title"`

	// Text is the pasted material itself.
	Text string `json:"text"`
}

// IngestTextResponse is the body of the 201 returned by
// POST /api/rag/documents/text.
//
// The first two fields are the ones the upload endpoint returns, so a client
// treats both creation paths the same: DocumentID names the document and State
// is the derived processing state. The two counts are what say which of the two
// outcomes happened — Chunks > 0 with Embedded == 0 means the corpus has no
// embedding client and the document is searchable lexically only — and they are
// the same names with the same meanings the status endpoint reports, so the two
// surfaces cannot disagree about them.
type IngestTextResponse struct {
	// DocumentID identifies the document the pasted text became.
	DocumentID string `json:"document_id"`

	// State is the derived processing state: "completed" once every chunk has a
	// vector, "indexing" while some do not — which is the permanent state of a
	// corpus built without an embedding client.
	State string `json:"state"`

	// Chunks is how many chunks the lexical index holds for the document.
	Chunks int `json:"chunks"`

	// Embedded is how many of those chunks have a vector.
	Embedded int `json:"embedded"`
}

// IngestURLRequest is the body of POST /api/rag/documents/url.
//
// Both fields are trimmed before they are validated, so a URL copied with a
// stray newline is not refused for being blank.
type IngestURLRequest struct {
	// Title is the material's name in the corpus list. It is optional: a fetch
	// always has a URL to name the material after, where a paste has nothing.
	Title string `json:"title"`

	// URL is the absolute http(s) address of the text to fetch.
	URL string `json:"url"`
}

// IngestURLResponse is the body of the 201 returned by
// POST /api/rag/documents/url.
//
// It carries the paste route's fields unchanged — the same document, state and
// counts, because the same use case produced them — plus the URL that was
// actually fetched. That last field is the server's answer to "what did you read
// on my behalf": after a redirect it is not the URL the caller sent, and it is
// what the document records as its origin.
type IngestURLResponse struct {
	IngestTextResponse

	// SourceURI is the URL the fetched bytes came from, after any redirects.
	SourceURI string `json:"source_uri"`
}

// DocumentStatusResponse is the status object returned by
// GET /api/rag/documents/{documentID}/status.
//
// The error and the timestamps are omitted when empty, so a queued document does
// not carry a null error or a zero instant that reads as year 1.
type DocumentStatusResponse struct {
	DocumentID string               `json:"document_id"`
	Title      string               `json:"title"`
	State      string               `json:"state"`
	Pages      PageProgressResponse `json:"pages"`
	Chunks     int                  `json:"chunks"`
	Embedded   int                  `json:"embedded"`
	Error      string               `json:"error,omitempty"`
	StartedAt  string               `json:"started_at,omitempty"`
	UpdatedAt  string               `json:"updated_at,omitempty"`
	FinishedAt string               `json:"finished_at,omitempty"`
}

// PageProgressResponse is the per-page breakdown of the status object.
type PageProgressResponse struct {
	Total       int `json:"total"`
	Extracted   int `json:"extracted"`
	Transcribed int `json:"transcribed"`
	Failed      int `json:"failed"`
	Pending     int `json:"pending"`
}

// DocumentSummaryResponse is one entry in the bare JSON array returned by
// GET /api/rag/documents.
//
// The artifact id, the source URI and the content hash are deliberately absent:
// they are the server's internals, not what a caller chooses a document by.
type DocumentSummaryResponse struct {
	// ID identifies the document, which is what a search scopes to.
	ID string `json:"id"`

	// Title is the document's human-readable name.
	Title string `json:"title"`

	// CreatedAt is when the document was registered, as RFC3339.
	CreatedAt string `json:"created_at"`

	// Pages is how many page outcomes the document has recorded, so a caller can
	// see whether it finished converting.
	Pages int `json:"pages"`

	// Chunks is how many chunks the document has in the lexical index.
	Chunks int `json:"chunks"`
}
