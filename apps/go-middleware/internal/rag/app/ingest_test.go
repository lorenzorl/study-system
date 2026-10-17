package app_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
	studysqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// promptPagePattern reads back the page a prompt was rendered for, so a fake can
// fail one specific page without parsing anything private to the use case.
var promptPagePattern = regexp.MustCompile(`This is page (\d+) of (\d+)`)

// longText is a page's worth of text, above the sparse-text threshold, so the
// heuristic leaves the page to the extractor. It carries no trailing whitespace,
// so a test can compare the recorded text with what the renderer emits verbatim.
func longText(page int) string {
	return strings.TrimRight(fmt.Sprintf("Page %d %s", page, strings.Repeat("content ", 40)), " ")
}

// shortText is deliberately below domain.MinExtractableText.
func shortText() string { return "Chapter 4" }

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func pngBytes() []byte {
	return []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x01, 0x02, 0x03}
}

// --- fakes ---

type fakeExtractor struct {
	pageCount    int
	pageCountErr error
	pages        map[int]string
	errs         map[int]error
	calls        []int
}

func (f *fakeExtractor) PageCount(context.Context) (int, error) {
	if f.pageCountErr != nil {
		return 0, f.pageCountErr
	}
	return f.pageCount, nil
}

func (f *fakeExtractor) PageText(_ context.Context, page int) (string, error) {
	f.calls = append(f.calls, page)
	if err, ok := f.errs[page]; ok {
		return "", err
	}
	return f.pages[page], nil
}

type fakeFigures struct {
	counts map[int]int
	err    error
	calls  int
}

func (f *fakeFigures) ImageCounts(context.Context) (map[int]int, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.counts, nil
}

type fakeRenderer struct {
	image []byte
	errs  map[int]error
	calls []int
}

func (f *fakeRenderer) RenderPage(_ context.Context, page int) ([]byte, error) {
	f.calls = append(f.calls, page)
	if err, ok := f.errs[page]; ok {
		return nil, err
	}
	return f.image, nil
}

type transcribeCall struct {
	prompt string
	image  []byte
}

type fakeTranscriber struct {
	fn    func(page int) (string, error)
	calls []transcribeCall
}

func (f *fakeTranscriber) Transcribe(_ context.Context, prompt string, imagePNG []byte) (string, error) {
	f.calls = append(f.calls, transcribeCall{prompt: prompt, image: imagePNG})

	page := 0
	if match := promptPagePattern.FindStringSubmatch(prompt); match != nil {
		page, _ = strconv.Atoi(match[1])
	}
	if f.fn != nil {
		return f.fn(page)
	}
	return fmt.Sprintf("vision text for page %d", page), nil
}

type fakeSource struct {
	ports     domain.PagePorts
	openErr   error
	openCalls []string
}

func (f *fakeSource) Open(pdfPath, workDir string) (domain.PagePorts, error) {
	f.openCalls = append(f.openCalls, pdfPath+"|"+workDir)
	if f.openErr != nil {
		return domain.PagePorts{}, f.openErr
	}
	return f.ports, nil
}

// --- harness ---

type harness struct {
	sqlDB   *sql.DB
	store   *storage.Store
	docs    domain.DocumentRepository
	jobs    domain.JobRepository
	pages   domain.PageOutcomeRepository
	pdf     []byte
	pdfPath string
	outPath string

	extractor   *fakeExtractor
	figures     *fakeFigures
	renderer    *fakeRenderer
	source      *fakeSource
	transcriber *fakeTranscriber
	useCase     *app.IngestDocumentUseCase
}

// newHarness wires the real SQLite repositories over an in-memory database, the
// real store rooted at a temp directory, and fakes for the two things that would
// otherwise need poppler and the network.
func newHarness(t *testing.T, pageCount int, pdf []byte) *harness {
	t.Helper()
	ctx := t.Context()

	sqlDB, err := db.Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })

	require.NoError(t, db.Migrate(ctx, sqlDB,
		slices.Concat(studysqlite.Migrations(), ragsqlite.Migrations())),
		"the ingestion use case runs over the same schema the server applies")

	store, err := storage.New(t.TempDir(), 1<<20)
	require.NoError(t, err)

	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "book.pdf")
	require.NoError(t, os.WriteFile(pdfPath, pdf, 0o600))

	h := &harness{
		sqlDB:   sqlDB,
		store:   store,
		docs:    ragsqlite.NewDocumentRepository(sqlDB),
		jobs:    ragsqlite.NewJobRepository(sqlDB),
		pages:   ragsqlite.NewPageOutcomeRepository(sqlDB),
		pdf:     pdf,
		pdfPath: pdfPath,
		outPath: filepath.Join(dir, "book.md"),

		extractor:   &fakeExtractor{pageCount: pageCount, pages: map[int]string{}, errs: map[int]error{}},
		figures:     &fakeFigures{counts: map[int]int{}},
		renderer:    &fakeRenderer{image: pngBytes(), errs: map[int]error{}},
		transcriber: &fakeTranscriber{},
	}
	h.source = &fakeSource{ports: domain.PagePorts{
		Extractor: h.extractor,
		Figures:   h.figures,
		Renderer:  h.renderer,
	}}
	h.useCase = app.NewIngestDocumentUseCase(
		h.docs, h.jobs, h.pages, h.source, h.transcriber, h.store,
	)

	return h
}

// withLongText gives every page above the sparse-text threshold.
func (h *harness) withLongText(pageCount int) *harness {
	for page := 1; page <= pageCount; page++ {
		h.extractor.pages[page] = longText(page)
	}
	return h
}

func (h *harness) execute(ctx context.Context) (*app.Report, []app.PageEvent, error) {
	var events []app.PageEvent

	report, err := h.useCase.Execute(ctx, app.IngestRequest{
		PDFPath:   h.pdfPath,
		Title:     "Test Book",
		SourceURI: h.pdfPath,
		OutPath:   h.outPath,
	}, func(event app.PageEvent) {
		events = append(events, event)
	})

	return report, events, err
}

// run executes in the test's context, which is the ordinary case.
func (h *harness) run(t *testing.T) (*app.Report, []app.PageEvent) {
	t.Helper()

	report, events, err := h.execute(t.Context())
	require.NoError(t, err)
	require.NotNil(t, report)

	return report, events
}

func (h *harness) outcomes(t *testing.T) []domain.PageOutcome {
	t.Helper()

	job, err := h.jobs.FindLatestByDocumentID(t.Context(), h.onlyDocument(t).ID)
	require.NoError(t, err)

	recorded, err := h.pages.ListOutcomes(t.Context(), job.ID)
	require.NoError(t, err)

	return recorded
}

func (h *harness) onlyDocument(t *testing.T) *domain.Document {
	t.Helper()

	document, err := h.docs.FindByContentHash(t.Context(), sha256Hex(h.pdf))
	require.NoError(t, err)

	return document
}

func (h *harness) countDocuments(t *testing.T) int {
	t.Helper()

	var count int
	require.NoError(t, h.sqlDB.QueryRow(`SELECT count(*) FROM documents`).Scan(&count))
	return count
}

func (h *harness) countJobs(t *testing.T) int {
	t.Helper()

	var count int
	require.NoError(t, h.sqlDB.QueryRow(`SELECT count(*) FROM ingestion_jobs`).Scan(&count))
	return count
}

func (h *harness) readOutput(t *testing.T) string {
	t.Helper()

	content, err := os.ReadFile(h.outPath)
	require.NoError(t, err)
	return string(content)
}

func outcomeFor(t *testing.T, outcomes []domain.PageOutcome, page int) domain.PageOutcome {
	t.Helper()

	for _, outcome := range outcomes {
		if outcome.PageNumber == page {
			return outcome
		}
	}

	t.Fatalf("no recorded outcome for page %d", page)
	return domain.PageOutcome{}
}

// headerLines counts the lines that start with a Markdown H1 marker.
func headerLines(document string) int {
	count := 0
	for _, line := range strings.Split(document, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "# ") {
			count++
		}
	}
	return count
}

// markersFor returns the recorded page markers, in file order.
func markersFor(document string) []string {
	var markers []string
	for _, line := range strings.Split(document, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "<!-- page") {
			markers = append(markers, strings.TrimSpace(line))
		}
	}
	return markers
}

// --- a text-only job ---

func TestIngestDocumentUseCase_ExtractsEveryPageWithoutCallingTheModel(t *testing.T) {
	h := newHarness(t, 3, []byte("text-only book")).withLongText(3)

	report, events := h.run(t)

	assert.True(t, report.Settled)
	assert.Equal(t, 3, report.Extracted)
	assert.Equal(t, 0, report.Transcribed)
	assert.Equal(t, 0, report.Failed)
	assert.Equal(t, 3, report.TotalPages)
	assert.Equal(t, 0, report.Skipped)

	assert.Equal(t, []int{1, 2, 3}, h.extractor.calls)
	assert.Empty(t, h.transcriber.calls, "a legible page must never cost a model call")
	assert.Empty(t, h.renderer.calls, "rendering is only for pages the model has to see")
	assert.Equal(t, 1, h.figures.calls, "the image inventory is read once for the whole book, not per page")

	assert.Equal(t, 3, len(h.outcomes(t)))
	assert.Len(t, events, 3)
}

// --- the vision path ---

func TestIngestDocumentUseCase_FigurePageGoesToTheModelAndIsRecordedTranscribed(t *testing.T) {
	h := newHarness(t, 3, []byte("illustrated book")).withLongText(3)
	h.figures.counts[2] = 1

	report, _ := h.run(t)

	assert.True(t, report.Settled)
	assert.Equal(t, 2, report.Extracted)
	assert.Equal(t, 1, report.Transcribed)

	recorded := h.outcomes(t)
	assert.Equal(t, domain.PageExtracted, outcomeFor(t, recorded, 1).Status)
	assert.Equal(t, domain.PageTranscribed, outcomeFor(t, recorded, 2).Status)
	assert.Equal(t, domain.MethodVision, outcomeFor(t, recorded, 2).Method)
	assert.Equal(t, domain.PageExtracted, outcomeFor(t, recorded, 3).Status)

	require.Len(t, h.transcriber.calls, 1)
	assert.Contains(t, h.transcriber.calls[0].prompt, "This is page 2 of 3")
	assert.Equal(t, pngBytes(), h.transcriber.calls[0].image, "the render's bytes reach the transcriber")
	assert.Equal(t, []int{2}, h.renderer.calls)
}

// --- resume ---

// This is acceptance criterion 2 at the use-case level: pages an earlier run
// recorded are not touched again, and the output file still contains every page
// exactly once.
func TestIngestDocumentUseCase_ResumesFromRecordedOutcomesWithoutRedoingPages(t *testing.T) {
	ctx := t.Context()
	h := newHarness(t, 5, []byte("half-converted book")).withLongText(5)

	// Seed the state a first run that died after page 2 would have left behind,
	// through the real repositories.
	artifact, err := h.store.Put(ctx, "book.pdf", bytes.NewReader(h.pdf))
	require.NoError(t, err)

	document := &domain.Document{
		ID:            uuid.NewString(),
		Title:         "Test Book",
		ArtifactID:    artifact.ID,
		SourceURI:     h.pdfPath,
		ContentSHA256: sha256Hex(h.pdf),
		CreatedAt:     time.Now().UTC(),
	}
	require.NoError(t, h.docs.Create(ctx, document))

	job, err := domain.NewJob(uuid.NewString(), document.ID, 5, time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, job.MarkRunning(time.Now().UTC()))
	require.NoError(t, h.jobs.Create(ctx, job))

	for page := 1; page <= 2; page++ {
		require.NoError(t, h.pages.SaveOutcome(ctx, domain.PageOutcome{
			JobID:      job.ID,
			PageNumber: page,
			Status:     domain.PageExtracted,
			Method:     domain.MethodExtract,
			Text:       fmt.Sprintf("recorded page %d", page),
			UpdatedAt:  time.Now().UTC(),
		}))
	}

	report, events := h.run(t)

	assert.True(t, report.Settled)
	assert.Equal(t, 2, report.Skipped, "the recorded pages are reported as skipped, not redone")
	assert.Equal(t, 5, report.Extracted)
	assert.Equal(t, job.ID, report.JobID, "the run continues the existing job")
	assert.Equal(t, 1, h.countJobs(t))

	assert.Equal(t, []int{3, 4, 5}, h.extractor.calls, "only the owed pages are extracted")
	assert.Empty(t, h.transcriber.calls)
	assert.Len(t, events, 3, "only the pages this run processed are reported")

	rendered := h.readOutput(t)
	assert.Equal(t, 1, headerLines(rendered))

	for page := 1; page <= 5; page++ {
		text := fmt.Sprintf("recorded page %d", page)
		if page > 2 {
			text = longText(page)
		}
		assert.Equal(t, 1, strings.Count(rendered, text),
			"page %d's text must appear exactly once in the written file", page)
	}
}

func TestIngestDocumentUseCase_SecondRunCreatesNoNewJobAndRedoesNothing(t *testing.T) {
	h := newHarness(t, 3, []byte("unchanged book")).withLongText(3)

	_, events := h.run(t)
	require.Len(t, events, 3)

	report, secondEvents := h.run(t)

	assert.Empty(t, secondEvents, "a settled job has no owed pages to report")
	assert.Equal(t, 3, report.Skipped)
	assert.Equal(t, 3, report.Extracted, "the report describes the job, not only this run")
	assert.Equal(t, []int{1, 2, 3}, h.extractor.calls, "the second run must not extract anything")
	assert.Equal(t, 1, h.countJobs(t), "one job per document, continued across runs")
	assert.Equal(t, 1, h.countDocuments(t))

	artifacts, err := h.store.List()
	require.NoError(t, err)
	assert.Len(t, artifacts, 1, "the bytes are stored once, not once per run")
}

// --- failure handling ---

func TestIngestDocumentUseCase_RecordsAFailedPageAndContinues(t *testing.T) {
	h := newHarness(t, 3, []byte("book with a bad page")).withLongText(3)
	h.extractor.errs[2] = errors.New("pdftotext exited 1")

	report, events := h.run(t)

	assert.True(t, report.Settled)
	assert.Equal(t, 2, report.Extracted)
	assert.Equal(t, 1, report.Failed, "one bad page must not throw away the other two")

	recorded := h.outcomes(t)
	assert.Equal(t, domain.PageFailed, outcomeFor(t, recorded, 2).Status)
	assert.Empty(t, outcomeFor(t, recorded, 2).Text, "a failed page carries no text")
	assert.Contains(t, outcomeFor(t, recorded, 2).Error, "pdftotext exited 1")
	assert.False(t, outcomeFor(t, recorded, 2).UpdatedAt.IsZero(),
		"updated_at is the audit column for when the page was attempted, so a failed outcome must carry one")

	require.Len(t, events, 3)
	assert.Equal(t, domain.PageFailed, events[1].Status)
	assert.Contains(t, events[1].Err, "pdftotext exited 1")

	document := h.readOutput(t)
	assert.Contains(t, document, "<!-- page 2 (failed) -->")
	assert.Contains(t, document, longText(1))
	assert.Contains(t, document, longText(3))
}

func TestIngestDocumentUseCase_RecordsAFailedTranscriptionAndContinues(t *testing.T) {
	h := newHarness(t, 3, []byte("book with an unreadable figure")).withLongText(3)
	h.figures.counts[2] = 1
	h.transcriber.fn = func(page int) (string, error) {
		if page == 2 {
			return "", errors.New("openrouter returned 402: insufficient credits")
		}
		return fmt.Sprintf("vision text for page %d", page), nil
	}

	report, _ := h.run(t)

	assert.True(t, report.Settled)
	assert.Equal(t, 1, report.Failed)
	assert.Equal(t, 2, report.Extracted)

	recorded := h.outcomes(t)
	assert.Equal(t, domain.PageFailed, outcomeFor(t, recorded, 2).Status)
	assert.Equal(t, domain.MethodVision, outcomeFor(t, recorded, 2).Method)
	assert.Contains(t, outcomeFor(t, recorded, 2).Error, "insufficient credits")

	assert.Contains(t, h.readOutput(t), "<!-- page 2 (failed) -->")
}

// --- interruption ---

func TestIngestDocumentUseCase_InterruptedRunWritesNoFile(t *testing.T) {
	h := newHarness(t, 4, []byte("interrupted book")).withLongText(4)
	h.extractor.errs[3] = context.Canceled

	report, _, err := h.execute(t.Context())

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, report)
	assert.False(t, report.Settled)

	_, statErr := os.Stat(h.outPath)
	assert.True(t, os.IsNotExist(statErr),
		"an interrupted run must leave the previous complete output in place, not a partial file")

	recorded := h.outcomes(t)
	assert.Len(t, recorded, 2, "only the pages completed before the interruption are recorded")
	assert.Equal(t, domain.JobRunning, h.job(t).Status,
		"a run that recorded pages is running, not pending: pending means no page has been attempted")
}

// job reads the document's current job from the real repository, so a test can
// assert the lifecycle state the database actually holds rather than the one the
// use case intended.
func (h *harness) job(t *testing.T) *domain.IngestionJob {
	t.Helper()

	job, err := h.jobs.FindLatestByDocumentID(t.Context(), h.onlyDocument(t).ID)
	require.NoError(t, err)

	return job
}

func TestIngestDocumentUseCase_InterruptedRunResumesIntoExactlyOneHeader(t *testing.T) {
	h := newHarness(t, 4, []byte("interrupted then resumed")).withLongText(4)
	h.extractor.errs[3] = context.Canceled

	_, _, err := h.execute(t.Context())
	require.Error(t, err)
	require.Empty(t, h.readOutputOrEmpty(t), "the interrupted run wrote nothing")

	delete(h.extractor.errs, 3)
	h.extractor.calls = nil

	report, events := h.run(t)

	assert.True(t, report.Settled)
	assert.Equal(t, 2, report.Skipped)
	assert.Equal(t, []int{3, 4}, h.extractor.calls, "the resumed run only extracts the owed pages")
	assert.Len(t, events, 2)

	document := h.readOutput(t)
	assert.Equal(t, 1, headerLines(document),
		"appending a resumed run must not put a second header in the middle of the book")
	assert.Equal(t, []string{
		"<!-- page 1 -->",
		"<!-- page 2 -->",
		"<!-- page 3 -->",
		"<!-- page 4 -->",
	}, markersFor(document))

	for page := 1; page <= 4; page++ {
		assert.Equal(t, 1, strings.Count(document, longText(page)),
			"page %d's text must appear exactly once after the resume", page)
	}
}

// --- aborting conditions ---

func TestIngestDocumentUseCase_ImageInventoryFailureAbortsBeforeAnyPage(t *testing.T) {
	h := newHarness(t, 3, []byte("unknown figures")).withLongText(3)
	h.figures.err = errors.New("pdfimages exited 1")

	report, _, err := h.execute(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pdfimages exited 1")
	require.NotNil(t, report)
	assert.False(t, report.Settled)

	assert.Empty(t, h.extractor.calls, "the run must not guess which pages carry figures")
	assert.Empty(t, h.outcomes(t))

	_, statErr := os.Stat(h.outPath)
	assert.True(t, os.IsNotExist(statErr))
}

func TestIngestDocumentUseCase_SourceOpenFailureAbortsBeforeAnyPage(t *testing.T) {
	h := newHarness(t, 3, []byte("no poppler")).withLongText(3)
	h.source.openErr = errors.New("pdftoppm: executable file not found in $PATH")

	_, _, err := h.execute(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pdftoppm")
	assert.Empty(t, h.extractor.calls)
}

// --- progress and validation ---

func TestIngestDocumentUseCase_ReportsEveryProcessedPageThroughTheCallback(t *testing.T) {
	h := newHarness(t, 3, []byte("progress book")).withLongText(3)

	_, events := h.run(t)

	require.Len(t, events, 3, "exactly one event per processed page")

	for index, event := range events {
		assert.Equal(t, index+1, event.Page)
		assert.Equal(t, 3, event.Total)
		assert.Equal(t, domain.PageExtracted, event.Status)
		assert.Equal(t, domain.MethodExtract, event.Method)
		assert.Equal(t, len([]rune(longText(event.Page))), event.Chars)
		assert.GreaterOrEqual(t, event.Elapsed, time.Duration(0))
	}
}

func TestIngestDocumentUseCase_SparsePageReachesTheModel(t *testing.T) {
	h := newHarness(t, 2, []byte("sparse book"))
	h.extractor.pages[1] = longText(1)
	h.extractor.pages[2] = shortText()

	report, _ := h.run(t)

	assert.Equal(t, 1, report.Extracted)
	assert.Equal(t, 1, report.Transcribed)
	assert.Equal(t, []int{2}, h.renderer.calls)
}

func TestIngestDocumentUseCase_RequiresAPDFAndAnOutputPath(t *testing.T) {
	h := newHarness(t, 1, []byte("book"))

	_, err := h.useCase.Execute(t.Context(), app.IngestRequest{OutPath: h.outPath}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperror.ErrInvalidInput)

	_, err = h.useCase.Execute(t.Context(), app.IngestRequest{PDFPath: h.pdfPath}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperror.ErrInvalidInput)
}

func TestIngestDocumentUseCase_MissingPDFIsReported(t *testing.T) {
	h := newHarness(t, 1, []byte("book"))

	_, err := h.useCase.Execute(t.Context(), app.IngestRequest{
		PDFPath: filepath.Join(t.TempDir(), "absent.pdf"),
		OutPath: h.outPath,
	}, nil)

	require.Error(t, err)
}

// readOutputOrEmpty returns the output file, or "" when it does not exist yet.
func (h *harness) readOutputOrEmpty(t *testing.T) string {
	t.Helper()

	content, err := os.ReadFile(h.outPath)
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	require.NoError(t, err)

	return string(content)
}
