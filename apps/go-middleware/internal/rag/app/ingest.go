// Package app orchestrates the ingestion context's use cases.
//
// It owns the running order of one conversion: which bytes are acquired, which
// document and job they belong to, which pages still need work, and when the
// output file may be written. The repositories, the PDF tooling, and the model
// client are all ports, so the order can be tested with a real database and no
// binary or network.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/llm"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// The transcriber port belongs to the context that needs it, and platform/llm
// satisfies it implicitly. This assertion lives at the only seam that may name
// both packages, so neither platform/llm nor rag/domain imports the other.
var _ domain.PageTranscriber = (*llm.Client)(nil)

// IngestRequest is one conversion run's input.
type IngestRequest struct {
	// PDFPath is the local file to convert.
	PDFPath string

	// Title is the human-readable name recorded on the document and emitted in
	// the Markdown header.
	Title string

	// SourceURI is informational: it is recorded on first sight of the bytes and
	// never used to decide identity.
	SourceURI string

	// OutPath is where the Markdown goes. It is supplied by the caller; the
	// backend never guesses a vault path.
	OutPath string
}

// PageEvent reports one processed page to the caller, for progress output. It is
// reported after the outcome is stored, so a printed line never claims work that
// was not recorded.
type PageEvent struct {
	Page    int
	Total   int
	Status  domain.PageStatus
	Method  domain.PageMethod
	Chars   int
	Elapsed time.Duration
	Err     string
}

// Report summarises a run.
//
// Extracted, Transcribed, and Failed describe the job, not this run: on a resume
// they include the pages an earlier run recorded. That is what makes a re-run of
// an already-finished job report the same success counts instead of zero.
type Report struct {
	DocumentID string
	JobID      string
	OutPath    string

	TotalPages  int
	Extracted   int
	Transcribed int
	Failed      int

	// Skipped is the number of pages that already had an outcome and were not
	// attempted again.
	Skipped int

	// Settled is true when every page reached a terminal outcome and the
	// Markdown was written.
	Settled bool

	Duration time.Duration
}

// IngestDocumentUseCase converts one document to Markdown, resuming where an
// earlier run stopped.
type IngestDocumentUseCase struct {
	documents   domain.DocumentRepository
	jobs        domain.JobRepository
	pages       domain.PageOutcomeRepository
	sources     domain.PageSourceFactory
	transcriber domain.PageTranscriber
	store       *storage.Store

	// now and newID are seams for tests. They are not configuration knobs.
	now   func() time.Time
	newID func() string
}

// NewIngestDocumentUseCase wires a conversion run.
//
// The use case takes *storage.Store rather than a new port: app is allowed to
// import platform, the store is the real thing in tests too, and an interface
// hiding two method calls would be a seam nothing needs.
func NewIngestDocumentUseCase(
	documents domain.DocumentRepository,
	jobs domain.JobRepository,
	pages domain.PageOutcomeRepository,
	sources domain.PageSourceFactory,
	transcriber domain.PageTranscriber,
	store *storage.Store,
) *IngestDocumentUseCase {
	return &IngestDocumentUseCase{
		documents:   documents,
		jobs:        jobs,
		pages:       pages,
		sources:     sources,
		transcriber: transcriber,
		store:       store,
		now:         func() time.Time { return time.Now().UTC() },
		newID:       uuid.NewString,
	}
}

// Execute runs one conversion.
//
// onPage, when not nil, is called exactly once per page this run processes, in
// ascending page order. Pages an earlier run recorded are not reported again.
//
// The returned Report is non-nil whenever a job was reached, including on an
// abort, so a caller can print what happened. A non-nil error always means the
// run did not settle, and in that case no output file was written.
func (uc *IngestDocumentUseCase) Execute(
	ctx context.Context,
	request IngestRequest,
	onPage func(PageEvent),
) (*Report, error) {
	started := uc.now()

	if err := request.validate(); err != nil {
		return nil, err
	}
	if uc.transcriber == nil {
		return nil, fmt.Errorf("page transcriber is required: %w", apperror.ErrInvalidInput)
	}

	contentHash, err := hashFile(request.PDFPath)
	if err != nil {
		return nil, err
	}

	document, err := uc.findOrCreateDocument(ctx, request, contentHash, started)
	if err != nil {
		return nil, err
	}

	payloadPath, err := uc.store.Path(document.ArtifactID)
	if err != nil {
		return nil, fmt.Errorf("resolve stored payload of %s: %w", request.PDFPath, err)
	}
	workDir, err := uc.store.Workspace(document.ArtifactID)
	if err != nil {
		return nil, fmt.Errorf("prepare render workspace of %s: %w", request.PDFPath, err)
	}

	source, err := uc.sources.Open(payloadPath, workDir)
	if err != nil {
		return nil, fmt.Errorf("open page source of %s: %w", request.PDFPath, err)
	}

	pageCount, err := source.Extractor.PageCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("read page count of %s: %w", request.PDFPath, err)
	}

	job, err := uc.findOrStartJob(ctx, document.ID, pageCount, started)
	if err != nil {
		return nil, err
	}

	report := &Report{
		DocumentID: document.ID,
		JobID:      job.ID,
		OutPath:    request.OutPath,
		TotalPages: job.TotalPages,
	}

	recorded, err := uc.pages.ListOutcomes(ctx, job.ID)
	if err != nil {
		return report, fmt.Errorf("list recorded pages of job %s: %w", job.ID, err)
	}

	pending := domain.PendingPages(job.TotalPages, recorded)
	report.Skipped = job.TotalPages - len(pending)

	// The inventory is read once for the whole book, before the loop. A failure
	// here aborts: pdfimages failing means we do not know which pages carry
	// figures, and treating that as zero images is the silent degradation the
	// header-driven parse exists to prevent.
	imageCounts, err := source.Figures.ImageCounts(ctx)
	if err != nil {
		return report, fmt.Errorf("list images of %s: %w", request.PDFPath, err)
	}

	for _, page := range pending {
		pageStarted := uc.now()

		outcome, err := uc.processPage(ctx, job, source, imageCounts, page)
		if err != nil {
			report.Duration = uc.now().Sub(started)
			return report, fmt.Errorf("page %d of %s: %w", page, request.PDFPath, err)
		}

		if err := uc.pages.SaveOutcome(ctx, outcome); err != nil {
			report.Duration = uc.now().Sub(started)
			return report, fmt.Errorf("record page %d of %s: %w", page, request.PDFPath, err)
		}

		if onPage != nil {
			onPage(PageEvent{
				Page:    page,
				Total:   job.TotalPages,
				Status:  outcome.Status,
				Method:  outcome.Method,
				Chars:   utf8.RuneCountInString(outcome.Text),
				Elapsed: uc.now().Sub(pageStarted),
				Err:     outcome.Error,
			})
		}
	}

	// Re-read rather than trusting the loop: the output is materialized from
	// recorded outcomes, and a resumed run's earlier pages live only in the
	// repository.
	final, err := uc.pages.ListOutcomes(ctx, job.ID)
	if err != nil {
		report.Duration = uc.now().Sub(started)
		return report, fmt.Errorf("re-read recorded pages of job %s: %w", job.ID, err)
	}

	progress := domain.ProgressOf(job.TotalPages, final)
	report.Extracted = progress.Extracted
	report.Transcribed = progress.Transcribed
	report.Failed = progress.Failed
	report.Settled = progress.Settled()
	report.Duration = uc.now().Sub(started)

	if !progress.Settled() {
		// An unsettled job gets no file: a partial document whose header claims
		// the full range is the retired script's bug, and leaving the previous
		// complete file in place is the correct outcome of an interrupted run.
		// Every owed page was processed, so reaching this point means the recorded
		// state disagrees with the job's range; reporting success would hide that.
		return report, fmt.Errorf("job %s still has %d pages pending after processing them: %w",
			job.ID, progress.Pending, apperror.ErrConflict)
	}

	if err := job.Settle(progress, uc.now()); err != nil {
		return report, fmt.Errorf("settle job %s: %w", job.ID, err)
	}
	if err := uc.jobs.UpdateStatus(ctx, job); err != nil {
		return report, fmt.Errorf("persist job %s: %w", job.ID, err)
	}
	if err := uc.writeMarkdown(request.OutPath, document.Title, job.TotalPages, final); err != nil {
		return report, err
	}

	return report, nil
}

// validate rejects a request that cannot produce a file.
func (r IngestRequest) validate() error {
	if strings.TrimSpace(r.PDFPath) == "" {
		return fmt.Errorf("pdf path is required: %w", apperror.ErrInvalidInput)
	}
	if strings.TrimSpace(r.OutPath) == "" {
		return fmt.Errorf("output path is required: %w", apperror.ErrInvalidInput)
	}
	return nil
}

// findOrCreateDocument returns the document for these bytes, storing them only
// the first time.
//
// The content hash is the identity, so a resume finds the existing document. On a
// hit the bytes are NOT stored again: the stored payload is what the page ports
// operate on, and re-storing would leave an orphan artifact per run.
func (uc *IngestDocumentUseCase) findOrCreateDocument(
	ctx context.Context,
	request IngestRequest,
	contentHash string,
	now time.Time,
) (*domain.Document, error) {
	existing, err := uc.documents.FindByContentHash(ctx, contentHash)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, apperror.ErrNotFound) {
		return nil, fmt.Errorf("look up document for %s: %w", request.PDFPath, err)
	}

	file, err := os.Open(request.PDFPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", request.PDFPath, err)
	}
	defer file.Close()

	artifact, err := uc.store.Put(ctx, filepath.Base(request.PDFPath), file)
	if err != nil {
		return nil, fmt.Errorf("store %s: %w", request.PDFPath, err)
	}

	document := &domain.Document{
		ID:            uc.newID(),
		Title:         request.Title,
		ArtifactID:    artifact.ID,
		SourceURI:     request.SourceURI,
		ContentSHA256: contentHash,
		CreatedAt:     now,
	}
	if err := uc.documents.Create(ctx, document); err != nil {
		if errors.Is(err, apperror.ErrConflict) {
			// Another run inserted the same bytes first. Reuse its document rather
			// than splitting one book across two rows.
			found, findErr := uc.documents.FindByContentHash(ctx, contentHash)
			if findErr != nil {
				return nil, fmt.Errorf("look up document for %s: %w", request.PDFPath, findErr)
			}
			return found, nil
		}
		return nil, fmt.Errorf("create document for %s: %w", request.PDFPath, err)
	}

	return document, nil
}

// findOrStartJob continues a document's job, creating one only when the document
// has none.
//
// One job per document is what makes resume cheap: the recorded outcomes describe
// this document's conversion, and a job per run would re-extract a whole book to
// resume one page.
//
// A fresh job is created already running, because its first page is attempted
// immediately; leaving it pending would make the status say no page had been tried
// while pages were being tried. An existing job is moved to running only from
// pending, because the domain refuses to move a settled job back and "continue"
// must not look like "restart".
func (uc *IngestDocumentUseCase) findOrStartJob(
	ctx context.Context,
	documentID string,
	pageCount int,
	now time.Time,
) (*domain.IngestionJob, error) {
	existing, err := uc.jobs.FindLatestByDocumentID(ctx, documentID)
	if err == nil {
		if existing.Status == domain.JobPending {
			if err := existing.MarkRunning(now); err != nil {
				return nil, fmt.Errorf("start job %s: %w", existing.ID, err)
			}
			if err := uc.jobs.UpdateStatus(ctx, existing); err != nil {
				return nil, fmt.Errorf("persist job %s: %w", existing.ID, err)
			}
		}
		return existing, nil
	}
	if !errors.Is(err, apperror.ErrNotFound) {
		return nil, fmt.Errorf("look up job for document %s: %w", documentID, err)
	}

	job, err := domain.NewJob(uc.newID(), documentID, pageCount, now)
	if err != nil {
		return nil, fmt.Errorf("create job for document %s: %w", documentID, err)
	}
	if err := job.MarkRunning(now); err != nil {
		return nil, fmt.Errorf("start job for document %s: %w", documentID, err)
	}
	if err := uc.jobs.Create(ctx, job); err != nil {
		if errors.Is(err, apperror.ErrConflict) {
			found, findErr := uc.jobs.FindLatestByDocumentID(ctx, documentID)
			if findErr != nil {
				return nil, fmt.Errorf("look up job for document %s: %w", documentID, findErr)
			}
			return found, nil
		}
		return nil, fmt.Errorf("create job for document %s: %w", documentID, err)
	}

	return job, nil
}

// processPage produces one page's outcome.
//
// A page whose extraction, render, or transcription fails is recorded as failed
// and the loop continues; only an interruption aborts. The difference matters:
// one unreadable page must not throw away the other 358.
func (uc *IngestDocumentUseCase) processPage(
	ctx context.Context,
	job *domain.IngestionJob,
	source domain.PagePorts,
	imageCounts map[int]int,
	page int,
) (domain.PageOutcome, error) {
	extracted, err := source.Extractor.PageText(ctx, page)
	if err != nil {
		if interrupted(ctx, err) {
			return domain.PageOutcome{}, err
		}
		return failedOutcome(job.ID, page, domain.MethodExtract, err, uc.now()), nil
	}

	if !domain.NeedsModel(extracted, imageCounts[page]) {
		return domain.PageOutcome{
			JobID:      job.ID,
			PageNumber: page,
			Status:     domain.PageExtracted,
			Method:     domain.MethodExtract,
			Text:       extracted,
			UpdatedAt:  uc.now(),
		}, nil
	}

	render, err := source.Renderer.RenderPage(ctx, page)
	if err != nil {
		if interrupted(ctx, err) {
			return domain.PageOutcome{}, err
		}
		return failedOutcome(job.ID, page, domain.MethodVision, err, uc.now()), nil
	}

	transcribed, err := uc.transcriber.Transcribe(ctx, TranscriptionPrompt(page, job.TotalPages), render)
	if err != nil {
		if interrupted(ctx, err) {
			return domain.PageOutcome{}, err
		}
		return failedOutcome(job.ID, page, domain.MethodVision, err, uc.now()), nil
	}

	return domain.PageOutcome{
		JobID:      job.ID,
		PageNumber: page,
		Status:     domain.PageTranscribed,
		Method:     domain.MethodVision,
		Text:       transcribed,
		UpdatedAt:  uc.now(),
	}, nil
}

// failedOutcome records a page that produced nothing usable. Its text stays
// empty and the next run retries it, which is what resuming means.
//
// The instant is passed in rather than left zero: updated_at is the audit column
// that says when this page was attempted, and the zero value would claim the
// attempt happened in year 1.
func failedOutcome(jobID string, page int, method domain.PageMethod, cause error, at time.Time) domain.PageOutcome {
	return domain.PageOutcome{
		JobID:      jobID,
		PageNumber: page,
		Status:     domain.PageFailed,
		Method:     method,
		Error:      cause.Error(),
		UpdatedAt:  at,
	}
}

// interrupted reports whether an error means the run must stop rather than
// record the page and continue.
func interrupted(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// writeMarkdown materializes the settled outcomes and commits them atomically.
//
// The temporary file lives in the target directory so the rename stays on one
// filesystem and is therefore atomic, and it is renamed into place only once it
// is complete. A vault file is read by other tools while the run is going, and it
// must never be observed half-written.
func (uc *IngestDocumentUseCase) writeMarkdown(
	outPath, title string,
	totalPages int,
	outcomes []domain.PageOutcome,
) error {
	markdown := domain.RenderMarkdown(domain.MarkdownHeader{
		Title:      title,
		TotalPages: totalPages,
		Generated:  uc.now(),
	}, outcomes)

	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("prepare output directory %s: %w", dir, err)
	}

	temp, err := os.CreateTemp(dir, ".ingest-*.md")
	if err != nil {
		return fmt.Errorf("create temporary output in %s: %w", dir, err)
	}
	tempPath := temp.Name()
	// A no-op after a successful rename, and the cleanup for every error path.
	defer os.Remove(tempPath)

	if _, err := temp.WriteString(markdown); err != nil {
		temp.Close()
		return fmt.Errorf("write %s: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, outPath); err != nil {
		return fmt.Errorf("move %s into place at %s: %w", tempPath, outPath, err)
	}

	return nil
}

// hashFile streams a file through sha256, so identity does not depend on holding
// a book in memory.
func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}

	return hex.EncodeToString(digest.Sum(nil)), nil
}
