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

	// Plan is the plan this run computed, including the counts a caller would
	// otherwise have to re-derive from the page events.
	Plan Plan

	Duration time.Duration
}

// Plan describes what a run will do, computed by reading the local PDF and the
// database before any model work starts.
//
// ToExtract and ToModel count only the pages this run still owes; pages an
// earlier run recorded are counted separately in AlreadyRecorded and are not
// touched.
type Plan struct {
	// TotalPages is the job's page count.
	TotalPages int

	// AlreadyRecorded is how many pages an earlier run recorded and this run
	// will not touch.
	AlreadyRecorded int

	// ToExtract is how many owed pages will be taken from the text layer.
	ToExtract int

	// ToModel is how many owed pages will be rendered and sent to the model.
	ToModel int

	// EstimatedVision is ToModel times one observed mean. It is a rough
	// estimate for the printed plan, not a promise.
	EstimatedVision time.Duration
}

// Options controls one run.
type Options struct {
	// DryRun plans the run and stops before doing any of it: nothing is stored,
	// no document or job is created, no page outcome is recorded, and no file is
	// written. It reads the local PDF and the database only.
	DryRun bool

	// Force allows the output file to replace one this pipeline did not generate.
	Force bool

	// MaxModelPages refuses the run when the plan would send more pages to the
	// model than this. Zero means no limit.
	MaxModelPages int
}

// Hooks are the caller's callbacks for observing a run.
type Hooks struct {
	// OnPlan is called once, after planning and before any model work, so a
	// caller can print what the run will cost before it starts spending.
	OnPlan func(Plan)

	// OnPage is called once per page this run processes, in ascending order.
	// Pages an earlier run recorded are not reported again.
	OnPage func(PageEvent)
}

// meanVisionPageDuration estimates one vision page's wall time.
//
// It is an observed mean: seven real pages measured between 31s and 1m41s. It is
// a rough estimate for the printed plan, not a promise — page complexity, model
// latency, and retries all move it — and it is deliberately a constant rather
// than a knob so plans stay comparable between runs.
const meanVisionPageDuration = 70 * time.Second

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

// runState is the resolved state of one run before its pages are planned.
//
// document and job are nil on a dry run of bytes the database has never seen,
// because a dry run persists neither.
type runState struct {
	document   *domain.Document
	job        *domain.IngestionJob
	totalPages int
	recorded   []domain.PageOutcome
}

// plannedPage is one owed page's planning result, reused verbatim by the
// processing loop so the text is extracted once.
type plannedPage struct {
	page       int
	text       string
	extractErr error
	needsModel bool
}

// Execute runs one conversion.
//
// The plan is computed first, from the local file and the database only: the
// owed pages' text is extracted once, each page is classified with
// domain.NeedsModel, and hooks.OnPlan reports the result. A dry run and a
// refusal stop there, before the database or the store is written to, so neither
// can leave a document, an artifact, or a job behind. Only a run that passes
// every refusal acquires the bytes, and its processing loop reuses the plan's
// decisions and text.
//
// The returned Report is non-nil whenever a plan was reached, including on an
// abort, so a caller can print what happened. A non-nil error always means the
// run did not settle, and in that case no output file was written.
func (uc *IngestDocumentUseCase) Execute(
	ctx context.Context,
	request IngestRequest,
	options Options,
	hooks Hooks,
) (*Report, error) {
	started := uc.now()

	if err := request.validate(); err != nil {
		return nil, err
	}

	contentHash, err := hashFile(request.PDFPath)
	if err != nil {
		return nil, err
	}

	state, plannedPages, plan, err := uc.plan(ctx, request, contentHash)
	if err != nil {
		return nil, err
	}

	report := &Report{
		OutPath:    request.OutPath,
		TotalPages: plan.TotalPages,
		Plan:       plan,
	}
	if state.document != nil {
		report.DocumentID = state.document.ID
	}
	if state.job != nil {
		report.JobID = state.job.ID
	}
	report.Skipped = plan.AlreadyRecorded

	if hooks.OnPlan != nil {
		hooks.OnPlan(plan)
	}

	if options.MaxModelPages > 0 && plan.ToModel > options.MaxModelPages {
		return report, fmt.Errorf("plan would send %d pages to the model, above the limit of %d: %w",
			plan.ToModel, options.MaxModelPages, apperror.ErrInvalidInput)
	}

	// A dry run never calls the model, so it needs no transcriber even when the
	// plan reaches the model. A real run needs one only when the plan actually
	// does: a text-only book needs no key.
	if !options.DryRun && plan.ToModel > 0 && uc.transcriber == nil {
		return report, fmt.Errorf("page transcriber is required: %w", apperror.ErrInvalidInput)
	}

	if options.DryRun {
		report.Duration = uc.now().Sub(started)
		return report, nil
	}

	// Every refusal has passed. Only now are the bytes stored and the document
	// and job created; processing renders from the stored payload, not the local
	// file.
	document, job, source, err := uc.acquire(ctx, request, contentHash, plan.TotalPages, started)
	if err != nil {
		report.Duration = uc.now().Sub(started)
		return report, err
	}
	report.DocumentID = document.ID
	report.JobID = job.ID

	for _, planned := range plannedPages {
		pageStarted := uc.now()

		outcome, err := uc.processPage(ctx, job, source, planned)
		if err != nil {
			report.Duration = uc.now().Sub(started)
			return report, fmt.Errorf("page %d of %s: %w", planned.page, request.PDFPath, err)
		}

		if err := uc.pages.SaveOutcome(ctx, outcome); err != nil {
			report.Duration = uc.now().Sub(started)
			return report, fmt.Errorf("record page %d of %s: %w", planned.page, request.PDFPath, err)
		}

		if hooks.OnPage != nil {
			hooks.OnPage(PageEvent{
				Page:    planned.page,
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
	if err := uc.writeMarkdown(request.OutPath, document.Title, job.TotalPages, final, options.Force); err != nil {
		return report, err
	}

	return report, nil
}

// acquire stores the bytes and creates the document and job on first sight, then
// opens the stored payload the processing loop renders from.
//
// It is called only after the plan has passed every refusal, so a refused run
// never reaches it and leaves nothing behind. Processing reads the stored payload
// rather than the local file: the archived bytes are what the page ports operate
// on, and the content hash guarantees they are the bytes the plan was computed
// from.
func (uc *IngestDocumentUseCase) acquire(
	ctx context.Context,
	request IngestRequest,
	contentHash string,
	pageCount int,
	started time.Time,
) (*domain.Document, *domain.IngestionJob, domain.PagePorts, error) {
	document, err := uc.findOrCreateDocument(ctx, request, contentHash, started)
	if err != nil {
		return nil, nil, domain.PagePorts{}, err
	}

	payloadPath, err := uc.store.Path(document.ArtifactID)
	if err != nil {
		return nil, nil, domain.PagePorts{}, fmt.Errorf("resolve stored payload of %s: %w", request.PDFPath, err)
	}
	workDir, err := uc.store.Workspace(document.ArtifactID)
	if err != nil {
		return nil, nil, domain.PagePorts{}, fmt.Errorf("prepare render workspace of %s: %w", request.PDFPath, err)
	}

	source, err := uc.sources.Open(payloadPath, workDir)
	if err != nil {
		return nil, nil, domain.PagePorts{}, fmt.Errorf("open page source of %s: %w", request.PDFPath, err)
	}

	job, err := uc.findOrStartJob(ctx, document.ID, pageCount, started)
	if err != nil {
		return nil, nil, domain.PagePorts{}, err
	}

	return document, job, source, nil
}

// plan reads the database and the local PDF and decides what the run will do,
// without writing to either.
//
// It is the only planning path, used by a dry run and a real run alike. It opens
// the file the caller named rather than a stored payload, because the payload may
// not exist yet and the content hash is what guarantees the local bytes are the
// ones identity was computed from. Nothing here writes, so a dry run or a refusal
// leaves the database and the store untouched.
func (uc *IngestDocumentUseCase) plan(
	ctx context.Context,
	request IngestRequest,
	contentHash string,
) (*runState, []plannedPage, Plan, error) {
	// The renderer's work directory is only used when a page is rendered, which
	// planning never does; pointing it at the system temp directory keeps any
	// hypothetical scratch out of the vault and the repository.
	source, err := uc.sources.Open(request.PDFPath, os.TempDir())
	if err != nil {
		return nil, nil, Plan{}, fmt.Errorf("open page source of %s: %w", request.PDFPath, err)
	}

	state := &runState{}

	document, err := uc.documents.FindByContentHash(ctx, contentHash)
	switch {
	case err == nil:
		state.document = document
	case errors.Is(err, apperror.ErrNotFound):
		// Those bytes have never been ingested; every page is owed and nothing
		// is stored or created.
	default:
		return nil, nil, Plan{}, fmt.Errorf("look up document for %s: %w", request.PDFPath, err)
	}

	if state.document != nil {
		job, err := uc.jobs.FindLatestByDocumentID(ctx, state.document.ID)
		switch {
		case err == nil:
			recorded, listErr := uc.pages.ListOutcomes(ctx, job.ID)
			if listErr != nil {
				return nil, nil, Plan{}, fmt.Errorf("list recorded pages of job %s: %w", job.ID, listErr)
			}
			state.job = job
			state.recorded = recorded
		case errors.Is(err, apperror.ErrNotFound):
			// The document exists but was never run; plan the whole book.
		default:
			return nil, nil, Plan{}, fmt.Errorf("look up job for document %s: %w", state.document.ID, err)
		}
	}

	pageCount, err := source.Extractor.PageCount(ctx)
	if err != nil {
		return nil, nil, Plan{}, fmt.Errorf("read page count of %s: %w", request.PDFPath, err)
	}
	state.totalPages = pageCount

	pending := domain.PendingPages(state.totalPages, state.recorded)

	// The inventory is read once for the whole book, before the plan. A failure
	// here aborts: pdfimages failing means we do not know which pages carry
	// figures, and treating that as zero images is the silent degradation the
	// header-driven parse exists to prevent.
	imageCounts, err := source.Figures.ImageCounts(ctx)
	if err != nil {
		return nil, nil, Plan{}, fmt.Errorf("list images of %s: %w", request.PDFPath, err)
	}

	plannedPages, plan := uc.planPages(ctx, source, state.totalPages, imageCounts, pending)
	return state, plannedPages, plan, nil
}

// planPages extracts each owed page's text once and classifies it.
//
// The extracted text and the domain.NeedsModel decision are returned so the
// processing loop can reuse them: extracting again would double the only free
// part of the run and make the printed plan a lie.
//
// A page whose extraction fails is captured rather than aborted. The loop decides
// whether the failure is an interruption that must stop the run or a page that is
// recorded as failed; such a page is counted in ToExtract because it will not
// reach the model.
func (uc *IngestDocumentUseCase) planPages(
	ctx context.Context,
	source domain.PagePorts,
	totalPages int,
	imageCounts map[int]int,
	pending []int,
) ([]plannedPage, Plan) {
	pages := make([]plannedPage, 0, len(pending))
	plan := Plan{
		TotalPages:      totalPages,
		AlreadyRecorded: totalPages - len(pending),
	}

	for _, page := range pending {
		text, err := source.Extractor.PageText(ctx, page)
		planned := plannedPage{page: page, text: text, extractErr: err}
		if err == nil {
			planned.needsModel = domain.NeedsModel(text, imageCounts[page])
		}

		if planned.needsModel {
			plan.ToModel++
		} else {
			plan.ToExtract++
		}
		pages = append(pages, planned)
	}

	plan.EstimatedVision = time.Duration(plan.ToModel) * meanVisionPageDuration
	return pages, plan
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
// operate on, and re-storing would leave an orphan artifact per run. The rule
// itself lives in FindOrCreateDocument, shared with the HTTP upload path.
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

	document, err := FindOrCreateDocument(ctx, uc.documents, artifact, contentHash,
		request.Title, request.SourceURI, now, uc.newID)
	if err != nil {
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
	planned plannedPage,
) (domain.PageOutcome, error) {
	if planned.extractErr != nil {
		if interrupted(ctx, planned.extractErr) {
			return domain.PageOutcome{}, planned.extractErr
		}
		return failedOutcome(job.ID, planned.page, domain.MethodExtract, planned.extractErr, uc.now()), nil
	}

	if !planned.needsModel {
		return domain.PageOutcome{
			JobID:      job.ID,
			PageNumber: planned.page,
			Status:     domain.PageExtracted,
			Method:     domain.MethodExtract,
			Text:       planned.text,
			UpdatedAt:  uc.now(),
		}, nil
	}

	render, err := source.Renderer.RenderPage(ctx, planned.page)
	if err != nil {
		if interrupted(ctx, err) {
			return domain.PageOutcome{}, err
		}
		return failedOutcome(job.ID, planned.page, domain.MethodVision, err, uc.now()), nil
	}

	transcribed, err := uc.transcriber.Transcribe(ctx, TranscriptionPrompt(planned.page, job.TotalPages), render)
	if err != nil {
		if interrupted(ctx, err) {
			return domain.PageOutcome{}, err
		}
		return failedOutcome(job.ID, planned.page, domain.MethodVision, err, uc.now()), nil
	}

	return domain.PageOutcome{
		JobID:      job.ID,
		PageNumber: planned.page,
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
//
// When force is false, an existing target is replaced only if it carries the
// generated header: a vault path may hold a note a human wrote, and the pipeline
// must not silently destroy it (backlog P3). The guard runs before the temporary
// file is created, so a refusal leaves nothing behind.
func (uc *IngestDocumentUseCase) writeMarkdown(
	outPath, title string,
	totalPages int,
	outcomes []domain.PageOutcome,
	force bool,
) error {
	if !force {
		if err := refuseForeignTarget(outPath); err != nil {
			return err
		}
	}

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
	// The mode is set explicitly rather than negotiated through the umask: the
	// file lands in a shared vault rather than a private directory, and a sync
	// process running as another user must be able to read it. os.CreateTemp
	// would otherwise leave it 0600.
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return fmt.Errorf("set mode of %s: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, outPath); err != nil {
		return fmt.Errorf("move %s into place at %s: %w", tempPath, outPath, err)
	}

	return nil
}

// outputHeadBytes bounds how much of an existing target is read to decide who
// wrote it. The generated header is one short line, so a few kilobytes are more
// than enough and a large book is never read just to be refused.
const outputHeadBytes = 4 * 1024

// refuseForeignTarget returns an error when outPath holds a file this pipeline
// did not generate.
//
// A missing target is fine: there is nothing to protect. An unreadable target is
// reported rather than overwritten.
func refuseForeignTarget(outPath string) error {
	file, err := os.Open(outPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect output path %s: %w", outPath, err)
	}
	defer file.Close()

	head := make([]byte, outputHeadBytes)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return fmt.Errorf("inspect output path %s: %w", outPath, err)
	}

	if domain.IsGeneratedMarkdown(head[:n]) {
		return nil
	}

	return fmt.Errorf("refusing to overwrite %s: it is not generated Markdown; pass -force to replace it: %w",
		outPath, apperror.ErrConflict)
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
