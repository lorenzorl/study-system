package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// IndexReport summarises one document's index rebuild.
type IndexReport struct {
	// DocumentID is the document that was indexed.
	DocumentID string

	// JobID is the job whose recorded pages were chunked, empty when the
	// document has no job and therefore no page to index.
	JobID string

	// Chunks is how many chunks were written.
	Chunks int
}

// ReindexReport summarises a full rebuild.
type ReindexReport struct {
	// Documents is how many documents were walked.
	Documents int

	// Chunks is how many chunks those documents produced in total.
	Chunks int
}

// IndexDocumentUseCase rebuilds the lexical index of a document from its
// recorded page text.
//
// The index is derived data: the page text in job_pages is the source of truth,
// and everything here is a pure function of it. That is what makes a rebuild the
// answer to both staleness and a half-written index, and what makes indexing the
// same document twice produce the same index rather than a longer one.
type IndexDocumentUseCase struct {
	documents domain.DocumentRepository
	jobs      domain.JobRepository
	pages     domain.PageOutcomeRepository
	chunks    domain.ChunkRepository

	// options is the specified chunking, not a knob: the sizes were decided in
	// the spec (S4), and a per-call choice would make two indexes of the same
	// corpus incomparable.
	options domain.ChunkOptions

	// now is a seam for tests, not a configuration knob.
	now func() time.Time
}

// NewIndexDocumentUseCase wires an index rebuild.
func NewIndexDocumentUseCase(
	documents domain.DocumentRepository,
	jobs domain.JobRepository,
	pages domain.PageOutcomeRepository,
	chunks domain.ChunkRepository,
) *IndexDocumentUseCase {
	return &IndexDocumentUseCase{
		documents: documents,
		jobs:      jobs,
		pages:     pages,
		chunks:    chunks,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

// Execute rebuilds one document's chunks. An unknown document is
// apperror.ErrNotFound.
//
// A document that has never been run is a no-op rather than an error: its latest
// job does not exist, so there is no recorded page to index and nothing to
// repair, and -reindex walks every document including ones nobody has converted.
func (uc *IndexDocumentUseCase) Execute(ctx context.Context, documentID string) (IndexReport, error) {
	report := IndexReport{DocumentID: documentID}

	document, err := uc.documents.FindByID(ctx, documentID)
	if err != nil {
		return report, err
	}

	return uc.index(ctx, document)
}

// ReindexAll rebuilds every document that has at least one recorded page.
//
// The document list is read once and then indexed one document at a time, so a
// failure names the document it happened on instead of leaving the caller to
// guess how far the walk got. It is idempotent: each document is replaced, not
// appended to.
func (uc *IndexDocumentUseCase) ReindexAll(ctx context.Context) (ReindexReport, error) {
	documents, err := uc.documents.ListIndexable(ctx)
	if err != nil {
		return ReindexReport{}, fmt.Errorf("list documents with recorded pages: %w", err)
	}

	report := ReindexReport{}
	for _, document := range documents {
		indexed, err := uc.index(ctx, document)
		if err != nil {
			return report, fmt.Errorf("index document %s: %w", document.ID, err)
		}
		report.Documents++
		report.Chunks += indexed.Chunks
	}

	return report, nil
}

// index reads the document's latest job and its recorded pages, chunks them in
// page order, and replaces the document's chunks.
//
// The latest job is the one a re-run resumes, so it holds the pages this document
// currently has. Every recorded outcome is passed to the chunker, including a
// failed page: such a page records an empty text and yields no chunk on its own,
// which is the chunker's decision to make and not one duplicated here.
func (uc *IndexDocumentUseCase) index(ctx context.Context, document *domain.Document) (IndexReport, error) {
	report := IndexReport{DocumentID: document.ID}

	job, err := uc.jobs.FindLatestByDocumentID(ctx, document.ID)
	if errors.Is(err, apperror.ErrNotFound) {
		return report, nil
	}
	if err != nil {
		return report, fmt.Errorf("look up the latest job of document %s: %w", document.ID, err)
	}
	report.JobID = job.ID

	outcomes, err := uc.pages.ListOutcomes(ctx, job.ID)
	if err != nil {
		return report, fmt.Errorf("list the recorded pages of job %s: %w", job.ID, err)
	}

	// Chunking in page order is this use case's requirement, so it is stated
	// here rather than left to whichever adapter implements the port.
	domain.SortOutcomes(outcomes)

	chunks := make([]domain.Chunk, 0, len(outcomes))
	for _, outcome := range outcomes {
		chunks = append(chunks, domain.ChunkPage(outcome.Text, outcome.PageNumber, uc.options)...)
	}

	if err := uc.chunks.ReplaceDocumentChunks(ctx, document.ID, job.ID, chunks, uc.now()); err != nil {
		return report, fmt.Errorf("replace the chunks of document %s: %w", document.ID, err)
	}
	report.Chunks = len(chunks)

	return report, nil
}
