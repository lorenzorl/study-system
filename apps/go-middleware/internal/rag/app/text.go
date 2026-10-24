package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// textPageCount is how many pages a pasted text occupies.
//
// The pipeline's unit of text is the page, and a paste has exactly one: it is
// not paginated by anything, so a chunk of it carries page 1 and the citation a
// search result prints is exact rather than a guess.
const textPageCount = 1

// pastedTextExtension names the stored artifact. The bytes are Markdown, exactly
// as the pipeline writes a processed book (markdownFileName), so the metadata
// says what the payload is; it is never a path component.
const pastedTextExtension = ".md"

// IngestTextRequest is one pasted material's input.
type IngestTextRequest struct {
	// Title is the material's human-readable name. It is trimmed before it is
	// recorded, and a blank one is refused: a corpus list of unnameable rows is
	// a list nobody can choose from.
	Title string

	// Text is the pasted material. It is trimmed at the edges before it is
	// stored, which is also what makes identity ignore a trailing newline: a
	// paste and the same paste with a stray blank line are one material.
	Text string

	// SourceURI is where the material was read from, and it is informational
	// only: identity is the content hash, so two URLs that serve the same bytes
	// are one document. It is empty for a paste, which was read from nowhere, and
	// the URL route passes the URL that was finally fetched after any redirects,
	// so a document's origin records where its bytes actually came from rather
	// than the address the user typed.
	SourceURI string
}

// IngestTextReport is what one pasted material produced.
type IngestTextReport struct {
	// DocumentID identifies the document the text became.
	DocumentID string

	// State is the derived processing state, computed by the same rule the
	// status read uses, so an ingest response and a later status poll never
	// disagree. Without an embedding client every chunk is indexed and unembedded
	// and the state is indexing; with one, and when every vector was written, it
	// is completed.
	State DocumentState

	// Chunks is how many chunks the lexical index now holds for the document.
	// It is at least one: a blank paste never reaches the index.
	Chunks int

	// Embedded is how many of those chunks have a vector. Zero means the corpus
	// has no embedding client and the document is lexically searchable only,
	// which is a normal outcome and not a failure.
	Embedded int

	// Sections is how many sections the structure stage stored. A pasted text
	// carries no PDF, so the stage reads nothing and this stays zero.
	Sections int
}

// IngestTextUseCase turns a pasted text into a corpus document and makes it
// searchable.
//
// It is the text counterpart of the upload endpoint's path — store the bytes,
// register the document, queue the pipeline — collapsed into one synchronous
// run, because every stage after ingestion reads only what the paste already
// provides: the text is available, so nothing has to be rendered, transcribed,
// or polled for. The stages themselves are the same use cases the PDF path runs,
// so a pasted document and an uploaded one are indexed by the same code.
type IngestTextUseCase struct {
	documents domain.DocumentRepository
	jobs      domain.JobRepository
	pages     domain.PageOutcomeRepository
	store     *storage.Store
	index     *IndexDocumentUseCase

	// embed is the optional embedding stage. It is nil in a server with no
	// embedding client, and then the document is indexed and left searchable
	// lexically rather than refused: the material is the user's and the corpus
	// must accept it whether or not a key is configured.
	embed *EmbedDocumentUseCase

	// structure is the optional structure stage. It is wired for symmetry with the
	// PDF pipeline and reads nothing here, because a paste has no stored PDF to
	// read a table of contents from.
	structure *StructureDocumentUseCase

	// now and newID are seams for tests, not configuration knobs.
	now   func() time.Time
	newID func() string
}

// NewIngestTextUseCase wires the pasted-text path.
func NewIngestTextUseCase(
	documents domain.DocumentRepository,
	jobs domain.JobRepository,
	pages domain.PageOutcomeRepository,
	store *storage.Store,
	index *IndexDocumentUseCase,
) *IngestTextUseCase {
	return &IngestTextUseCase{
		documents: documents,
		jobs:      jobs,
		pages:     pages,
		store:     store,
		index:     index,
		now:       func() time.Time { return time.Now().UTC() },
		newID:     uuid.NewString,
	}
}

// WithEmbed adds the embedding stage, which the run then executes after
// indexing. A use case built without it is the no-credential shape, not a
// degraded one.
func (uc *IngestTextUseCase) WithEmbed(embed *EmbedDocumentUseCase) *IngestTextUseCase {
	uc.embed = embed
	return uc
}

// WithStructure adds the structure stage. It runs last and, for a paste, stores
// nothing: the cascade reads a PDF's printed index and typography, and there is
// no PDF behind a pasted text.
func (uc *IngestTextUseCase) WithStructure(structure *StructureDocumentUseCase) *IngestTextUseCase {
	uc.structure = structure
	return uc
}

// Execute stores the text, registers it as a document, records its single page,
// and runs the searchable stages.
//
// The order is deliberate: the text is hashed and looked up before anything is
// written, so a duplicate paste is refused without leaving an artifact behind.
// Re-pasting is the common case — a user who is unsure whether the first attempt
// worked — and it must not cost disk or a second row.
func (uc *IngestTextUseCase) Execute(
	ctx context.Context, request IngestTextRequest,
) (IngestTextReport, error) {
	now := uc.now()
	title := strings.TrimSpace(request.Title)
	text := strings.TrimSpace(request.Text)

	// The transport validates both with its own message; this is the invariant a
	// caller other than the HTTP layer also gets.
	if title == "" {
		return IngestTextReport{}, fmt.Errorf("title is required: %w", apperror.ErrInvalidInput)
	}
	if text == "" {
		return IngestTextReport{}, fmt.Errorf("text is required: %w", apperror.ErrInvalidInput)
	}

	contentHash := sha256Hex(text)

	existing, err := uc.documents.FindByContentHash(ctx, contentHash)
	switch {
	case err == nil:
		return IngestTextReport{}, fmt.Errorf(
			"document %s already holds this text: %w", existing.ID, apperror.ErrConflict)
	case errors.Is(err, apperror.ErrNotFound):
		// Never seen: store it and create the row.
	default:
		return IngestTextReport{}, fmt.Errorf("look up the document holding this text: %w", err)
	}

	artifact, err := uc.store.Put(ctx, title+pastedTextExtension, strings.NewReader(text))
	if err != nil {
		return IngestTextReport{}, fmt.Errorf("store the pasted text: %w", err)
	}

	document := &domain.Document{
		ID:            uc.newID(),
		Title:         title,
		ArtifactID:    artifact.ID,
		SourceURI:     strings.TrimSpace(request.SourceURI),
		ContentSHA256: contentHash,
		CreatedAt:     now,
	}
	// Create is the write, and not FindOrCreateDocument: that rule returns the
	// winner of a race so an ingestion run can resume, while a second paste is a
	// conflict the caller has to see. The repository already reports the unique
	// content hash as apperror.ErrConflict, which is where that rule lives.
	if err := uc.documents.Create(ctx, document); err != nil {
		// Nothing refers to the stored bytes: the row that would name them was
		// never written, so keeping the artifact would leave an orphan per retry.
		_ = uc.store.Delete(artifact.ID)

		if errors.Is(err, apperror.ErrConflict) {
			return IngestTextReport{}, fmt.Errorf("a document with this text appeared: %w", err)
		}
		return IngestTextReport{}, fmt.Errorf("create document for the pasted text: %w", err)
	}

	job, err := uc.recordTextPage(ctx, document.ID, text, now)
	if err != nil {
		return IngestTextReport{}, err
	}

	report := IngestTextReport{DocumentID: document.ID}

	indexed, err := uc.index.Execute(ctx, document.ID)
	if err != nil {
		return report, fmt.Errorf("index the pasted text of document %s: %w", document.ID, err)
	}
	report.Chunks = indexed.Chunks

	if uc.embed != nil {
		embedded, err := uc.embed.Execute(ctx, document.ID)
		if err != nil {
			return report, fmt.Errorf("embed the pasted text of document %s: %w", document.ID, err)
		}
		report.Embedded = embedded.Embeddings
	}

	// Structure last, as in the PDF pipeline. A paste carries no PDF, so the stage
	// is asked with no payload path and answers with a reason instead of sections;
	// it is still run so a text document cannot skip a stage the corpus defines.
	if uc.structure != nil {
		structured, err := uc.structure.Execute(ctx, StructureRequest{DocumentID: document.ID})
		if err != nil {
			return report, fmt.Errorf("structure the pasted text of document %s: %w", document.ID, err)
		}
		report.Sections = structured.Sections
	}

	report.State = deriveDocumentState(DocumentStatus{
		DocumentID: document.ID,
		Chunks:     report.Chunks,
		Embedded:   report.Embedded,
	}, job, false)

	return report, nil
}

// recordTextPage creates the document's job, records its single page as text
// that is already available, and settles it.
//
// It does not reuse findOrStartJob: that one resumes a document's existing job
// and takes its page count from the PDF, and neither applies here — a paste is
// one page, and a second paste never reaches this point. The page is recorded as
// extracted by the extract method because that is the pipeline's name for text
// that needed no model; the text is not a PDF text layer, but the vocabulary is
// closed and the alternative — a third method — would say nothing a reader of
// the page row needs.
func (uc *IngestTextUseCase) recordTextPage(
	ctx context.Context, documentID, text string, now time.Time,
) (*domain.IngestionJob, error) {
	job, err := domain.NewJob(uc.newID(), documentID, textPageCount, now)
	if err != nil {
		return nil, fmt.Errorf("create job for document %s: %w", documentID, err)
	}
	// A fresh job is created already running, because its only page is recorded
	// immediately; leaving it pending would claim no page had been attempted while
	// the page exists.
	if err := job.MarkRunning(now); err != nil {
		return nil, fmt.Errorf("start job for document %s: %w", documentID, err)
	}
	if err := uc.jobs.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("persist job for document %s: %w", documentID, err)
	}

	outcome := domain.PageOutcome{
		JobID:      job.ID,
		PageNumber: 1,
		Status:     domain.PageExtracted,
		Method:     domain.MethodExtract,
		Text:       text,
		UpdatedAt:  now,
	}
	if err := uc.pages.SaveOutcome(ctx, outcome); err != nil {
		return nil, fmt.Errorf("record page 1 of document %s: %w", documentID, err)
	}

	progress := domain.ProgressOf(textPageCount, []domain.PageOutcome{outcome})
	if err := job.Settle(progress, now); err != nil {
		return nil, fmt.Errorf("settle job %s: %w", job.ID, err)
	}
	if err := uc.jobs.UpdateStatus(ctx, job); err != nil {
		return nil, fmt.Errorf("persist job %s: %w", job.ID, err)
	}

	return job, nil
}

// sha256Hex is the document identity of a text, in the shape the schema
// enforces: 64 lowercase hex characters.
func sha256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
