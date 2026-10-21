package domain

import (
	"context"
	"time"
)

// DocumentRepository manages persistence of Document entities.
type DocumentRepository interface {
	// Create inserts a document. A content hash that is already present is a
	// conflict: the caller should have found it with FindByContentHash and
	// resumed, and silently creating a second document for the same bytes would
	// split one book's history across two rows.
	Create(ctx context.Context, document *Document) error

	// FindByID returns a document by its primary key, or apperror.ErrNotFound.
	FindByID(ctx context.Context, id string) (*Document, error)

	// FindByContentHash returns the document for an already-acquired content
	// hash, or apperror.ErrNotFound when those bytes have never been ingested.
	// This is the lookup that decides resume or start fresh.
	FindByContentHash(ctx context.Context, contentSHA256 string) (*Document, error)

	// ListIndexable returns every document that has at least one recorded page,
	// ordered by title and then id, as an empty slice when there is none.
	//
	// It is what a full reindex walks, and the "has a recorded page" filter is
	// the point: a document nobody has converted has nothing to index, and
	// rebuilding it would report work that produced no chunk.
	ListIndexable(ctx context.Context) ([]*Document, error)
}

// ChunkRepository manages the lexical index over recorded page text.
type ChunkRepository interface {
	// ReplaceDocumentChunks deletes the document's chunks and writes the given
	// ones in their place, inside one transaction.
	//
	// Replace is the only write: the index is derived data with a rebuildable
	// life cycle, so recovering from a stale or interrupted index is re-running
	// this call. An append would leave a document's old text matching alongside
	// its new text, and a caller could not tell the two apart.
	//
	// An unknown document or job is apperror.ErrNotFound.
	ReplaceDocumentChunks(ctx context.Context, documentID, jobID string, chunks []Chunk, now time.Time) error

	// Search returns the chunks matching a user's query, best first.
	//
	// An empty query, a query of only punctuation, and a query that matches
	// nothing all return an empty slice and no error. The query text is the
	// user's, not a query language: it is the repository's job to make it a valid
	// expression rather than the caller's job to sanitise it.
	Search(ctx context.Context, query string, limit int) ([]SearchResult, error)

	// ListDocumentChunks returns the document's stored chunks in index order
	// (page, then ordinal), each with its id and its text, or an empty slice when
	// the document has none.
	//
	// It is what the embedding stage is fed from: the chunk text a vector is built
	// from is the same text the lexical index holds, and taking it from here avoids
	// re-reading and re-chunking the page outcomes.
	ListDocumentChunks(ctx context.Context, documentID string) ([]StoredChunk, error)

	// CountByDocument returns how many chunks the document has.
	//
	// It is an aggregate read, not a list: the processing-status endpoint needs
	// the count, and reading the chunks to count them would load every chunk's
	// text to answer a question about how many. An unknown document returns 0, not
	// an error: "no chunk" and "no document" are the same answer here.
	CountByDocument(ctx context.Context, documentID string) (int, error)
}

// MatchStage names the retrieval stage that produced a search result, so a
// caller can tell a lexical hit from a semantic one and see when both agreed.
type MatchStage string

const (
	// StageLexical means only the FTS5 bm25 ranking found the chunk.
	StageLexical MatchStage = "lexical"

	// StageSemantic means only the cosine ranking found the chunk, which is the
	// case that crosses the language boundary.
	StageSemantic MatchStage = "semantic"

	// StageHybrid means both rankings found the chunk.
	StageHybrid MatchStage = "hybrid"
)

// SearchResult is one ranked chunk, carrying everything a caller needs to show
// it and to cite it.
type SearchResult struct {
	// ChunkID identifies the chunk.
	ChunkID string

	// DocumentID identifies the book the chunk came from.
	DocumentID string

	// DocumentTitle is the document's title, joined in so a result is
	// human-readable without a second lookup.
	DocumentTitle string

	// PageNumber is the 1-based page to open. A chunk never spans two pages, so
	// this is exact.
	PageNumber int

	// SectionTitle is the deepest section that owns PageNumber, and empty when the
	// document has no confirmed structure. It is the other half of a citation: a
	// page tells a reader where to look, a section tells them what they are
	// looking at.
	SectionTitle string

	// SectionLevel is that section's nesting, 0 when there is none. It is carried
	// so a client can render the hierarchy without another request.
	SectionLevel int

	// Snippet is a bounded window of the chunk's text with the matched terms
	// marked, folded onto one line so a caller can print one line per result. It
	// is a citation aid, not context: FTS5 builds it from twelve tokens.
	Snippet string

	// Text is the chunk's text, verbatim. It exists because a caller that feeds the
	// retrieved passage to a model needs the passage and not a twelve-token window:
	// a prompt built from snippets alone answers from a fragment. A semantic hit
	// matched no term, so for that stage Text is the whole of what there is.
	Text string

	// Score is the lexical rank as the index computes it: SMALLER (more negative)
	// is a BETTER match, and results are ordered by it ascending. A reader who
	// assumes a bigger score is better reads the ranking inverted, which is why
	// the command prints its magnitude under a name that says what it means.
	//
	// It is zero when the chunk was not in the lexical ranking.
	Score float64

	// Stage names the retrieval stage that produced this result.
	Stage MatchStage

	// Similarity is the semantic cosine similarity in [-1, 1], where LARGER is a
	// BETTER match, which is the opposite of Score. It is zero when the chunk was
	// not in the semantic ranking.
	Similarity float64
}

// JobRepository manages persistence of IngestionJob entities.
type JobRepository interface {
	// Create inserts a job. An unknown document is apperror.ErrNotFound.
	Create(ctx context.Context, job *IngestionJob) error

	// FindByID returns a job by its primary key, or apperror.ErrNotFound.
	FindByID(ctx context.Context, id string) (*IngestionJob, error)

	// FindLatestByDocumentID returns the most recently created job for a
	// document, which is the one a re-run resumes. It returns
	// apperror.ErrNotFound when the document has never been run.
	FindLatestByDocumentID(ctx context.Context, documentID string) (*IngestionJob, error)

	// UpdateStatus persists the lifecycle fields after a domain transition
	// (MarkRunning, Settle). Progress is deliberately not persisted: it is
	// derived from page outcomes, so there is no counter to drift.
	UpdateStatus(ctx context.Context, job *IngestionJob) error
}

// PageCounts is the number of recorded page outcomes of a job, grouped by
// terminal status. It carries no total: the job owns that, and Pending is
// derived from the two, exactly as domain.Progress derives it.
type PageCounts struct {
	// Extracted is how many pages came from the PDF's own text layer.
	Extracted int

	// Transcribed is how many pages came from the model.
	Transcribed int

	// Failed is how many pages produced nothing usable.
	Failed int
}

// PageOutcomeRepository manages the recorded per-page outcomes of a job.
type PageOutcomeRepository interface {
	// SaveOutcome records a page's outcome, replacing any earlier outcome for the
	// same page.
	//
	// The replace is the point, not an implementation detail: retrying a failed
	// page is what resuming means, so the page's row is updated rather than
	// appended. The primary key (job_id, page_number) is what keeps exactly one
	// outcome per page, which is what makes the derived progress honest.
	//
	// An unknown job is apperror.ErrNotFound.
	SaveOutcome(ctx context.Context, outcome PageOutcome) error

	// ListOutcomes returns every recorded outcome of a job, ordered by page
	// number ascending. Reading them all is the point: the output document is
	// materialized from recorded text, so a resumed run needs the pages it did
	// not touch just as much as the ones it did.
	ListOutcomes(ctx context.Context, jobID string) ([]PageOutcome, error)

	// CountByJob returns how many recorded outcomes the job has, by status.
	//
	// It is what the processing-status endpoint reads instead of ListOutcomes:
	// the status needs counts, and ListOutcomes would load every page's text to
	// answer it. A job with no recorded outcome returns a zero PageCounts and no
	// error.
	CountByJob(ctx context.Context, jobID string) (PageCounts, error)
}

// SectionRepository manages a document's structure: the parts, chapters and
// sections that own page ranges.
type SectionRepository interface {
	// ReplaceDocumentSections deletes the document's sections and writes the
	// given ones in their place, inside one transaction.
	//
	// Replace is the only write, for the same reason it is for chunks: the
	// structure is derived data with a rebuildable life cycle, so recovering from
	// a stale or half-written list is re-running the extractor. An empty list is
	// meaningful and clears the document — that is how a document nobody can
	// structure is stored.
	//
	// Insertion order is the storage's concern: the adapter writes parents before
	// children, so the caller does not have to sort its tree.
	//
	// An unknown document is apperror.ErrNotFound.
	ReplaceDocumentSections(ctx context.Context, documentID string, sections []*Section) error

	// ListByDocument returns the document's sections in reading order (start
	// page, then level), as an empty slice when it has none.
	ListByDocument(ctx context.Context, documentID string) ([]*Section, error)

	// SectionAt returns the deepest section that owns the given page, or
	// apperror.ErrNotFound when no section does.
	//
	// It is the one question the index asks: a chunk carries a page number and
	// nothing else about the structure, so attribution is a lookup and never a
	// guess. A page below 1 is apperror.ErrInvalidInput.
	SectionAt(ctx context.Context, documentID string, page int) (*Section, error)
}
