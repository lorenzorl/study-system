package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// HeadingsSource is one reading of a document's own structure.
//
// The port exists so the cascade can be exercised without poppler, without a PDF
// and without a database: production satisfies it with the two pdf extractors,
// and a test with a fixed list and a fixed failure.
type HeadingsSource interface {
	Headings(ctx context.Context) ([]domain.Heading, error)
}

// StructureSources are one document's two readings of the same stored payload.
type StructureSources struct {
	// Index is the printed table of contents. A document with no index answers
	// with an empty list and no error.
	Index HeadingsSource

	// Typographic is every line the document sets above its body size. It is
	// read on every path because it is what confirms either source.
	Typographic HeadingsSource
}

// StructureSourceFactory builds those readings for one stored PDF.
//
// It is a factory rather than two sources on the use case because a reader is
// bound to a path and the path belongs to the document, which the pipeline only
// knows once the artifact is resolved. A failure here — the tool is missing, the
// payload will not open — is the first thing the cascade has to survive.
type StructureSourceFactory func(pdfPath string) (StructureSources, error)

// StructureRequest is one document's structure run.
type StructureRequest struct {
	// DocumentID is the document the sections belong to.
	DocumentID string

	// PDFPath is the stored payload the readings are taken from. The pipeline
	// resolves it from the document's artifact; this layer never touches the
	// store, which is what keeps it runnable over a temp directory in a test.
	PDFPath string
}

// StructureDocumentReport says what a structure run stored, or why it stored
// nothing.
type StructureDocumentReport struct {
	// DocumentID is the document that was read.
	DocumentID string

	// Sections is how many sections the document now has. Zero is a normal
	// answer, not a failure: a document nobody can structure is stored with
	// none (S5).
	Sections int

	// Structure is what BuildSections decided. It is nil only when the run never
	// reached the decision, because a reading it needed could not be taken.
	Structure *StructureReport

	// Reason explains a zero result in one sentence, and is empty when sections
	// were stored.
	Reason string
}

// StructureDocumentUseCase produces and stores one document's sections.
//
// It runs the cascade of S3 in order: the printed index, the numbered headings
// of a document with no index, and the typography that confirms either of them.
// The decision of what becomes a section is not made here — BuildSections owns
// it, including the gate — so this layer adds only what a pure function cannot
// do: read the document, ask for its pages, and store the answer.
type StructureDocumentUseCase struct {
	sections domain.SectionRepository
	jobs     domain.JobRepository
	pages    domain.PageOutcomeRepository
	sources  StructureSourceFactory

	// now is a seam for tests, not a configuration knob.
	now func() time.Time
}

// NewStructureDocumentUseCase wires a structure run.
func NewStructureDocumentUseCase(
	sections domain.SectionRepository,
	jobs domain.JobRepository,
	pages domain.PageOutcomeRepository,
	sources StructureSourceFactory,
) *StructureDocumentUseCase {
	return &StructureDocumentUseCase{
		sections: sections,
		jobs:     jobs,
		pages:    pages,
		sources:  sources,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// Execute reads the document's structure and replaces the stored sections with
// it.
//
// The error asymmetry is deliberate, and it is the whole safety of the feature
// (S5). Only a database failure while storing the sections is returned: that is
// the one failure the caller can act on — the read succeeded, the answer is
// known, and retrying is the fix. Every failure to *read* the document is a
// document with no sections, not a failed document, so it is reported in the
// report and never returned; a paper nobody can structure must still index and
// still answer.
//
// A run that never obtained a reading writes nothing at all, instead of storing
// an empty list. Storing zero sections is how "this document has no confirmable
// structure" is said (S4), and a reader that never ran has not decided that: the
// sections an earlier run read from this payload are still the last thing the
// document actually printed.
func (uc *StructureDocumentUseCase) Execute(
	ctx context.Context, request StructureRequest,
) (StructureDocumentReport, error) {
	documentID := strings.TrimSpace(request.DocumentID)
	report := StructureDocumentReport{DocumentID: documentID}

	if documentID == "" {
		return report, fmt.Errorf("structure a document without an id: %w", apperror.ErrInvalidInput)
	}

	if strings.TrimSpace(request.PDFPath) == "" {
		report.Reason = "the document has no stored payload to read its structure from"
		return report, nil
	}

	// The page count comes from the job because it bounds every range, and the
	// job is also what owns the recorded page text the fallback reads.
	job, err := uc.jobs.FindLatestByDocumentID(ctx, documentID)
	if err != nil {
		report.Reason = fmt.Sprintf("read the latest ingestion job of the document: %v", err)
		return report, nil
	}

	sources, err := uc.sources(request.PDFPath)
	if err != nil {
		report.Reason = fmt.Sprintf("build the structure readers: %v", err)
		return report, nil
	}

	// The cascade's first two steps. A document with no index — or one whose
	// index the reader could not read — is read from its own page text, which
	// needs no poppler at all and also covers a page a vision model transcribed.
	entries, indexErr := sourceHeadings(ctx, sources.Index)
	provenance := domain.SourceTOC

	// An index that names no chapter is not an index: three leader-dot rows on a
	// table page look exactly like an index to the reader, and falling back here is
	// what keeps that from becoming the document's only structure source.
	if indexErr != nil || !namesAChapter(entries) {
		pattern, patternErr := uc.numberedHeadings(ctx, job.ID)
		if patternErr != nil {
			report.Reason = fmt.Sprintf("read the recorded page text: %v", patternErr)
			return report, nil
		}
		entries, provenance = pattern, domain.SourcePattern
	}

	typographic, err := sourceHeadings(ctx, sources.Typographic)
	if err != nil {
		// Nothing confirms anything without this reading, so the gate could only
		// fail. Reporting the real reason is the honest answer, and writing an
		// empty list here would blame the document for a broken tool.
		report.Reason = fmt.Sprintf("read the typography: %v", err)
		return report, nil
	}

	// absence is kept because BuildSections' own sentence speaks of an index, and
	// a document that prints none would be explained by a claim it never made.
	absence := ""
	if len(entries) == 0 {
		absence = "the document prints no index and no numbered heading"
		if indexErr != nil {
			absence = fmt.Sprintf("read the printed index: %v", indexErr)
		}
	}

	sections, structure := BuildSections(StructureInput{
		DocumentID:  documentID,
		PageCount:   job.TotalPages,
		TOC:         entries,
		Typographic: typographic,
		Source:      provenance,
	}, uc.now())
	report.Structure = &structure

	// The empty list is stored too: it is how a document with no confirmable
	// structure is represented, and it is what clears a structure a later reading
	// proved wrong.
	if err := uc.sections.ReplaceDocumentSections(ctx, documentID, sections); err != nil {
		return report, fmt.Errorf("store the sections of document %s: %w", documentID, err)
	}
	report.Sections = len(sections)

	report.Reason = absence
	if report.Sections > 0 {
		report.Reason = ""
		return report, nil
	}

	if report.Reason == "" {
		report.Reason = structure.Reason
	}
	if report.Reason == "" {
		report.Reason = "no entry the document prints became a section"
	}
	return report, nil
}

// numberedHeadings reads the document's recorded page text as a document with no
// index: every page's numbered headings, in page order.
//
// The text comes from job_pages and never from the file, which is what makes this
// step cost nothing — no poppler, no PDF — and what lets it read a page the model
// transcribed rather than extracted.
func (uc *StructureDocumentUseCase) numberedHeadings(ctx context.Context, jobID string) ([]domain.Heading, error) {
	outcomes, err := uc.pages.ListOutcomes(ctx, jobID)
	if err != nil {
		return nil, err
	}

	headings := []domain.Heading{}
	for _, outcome := range outcomes {
		// A failed page recorded no text, so it contributes nothing and needs no
		// special case: HeadingsFromPageText answers with an empty list.
		headings = append(headings, domain.HeadingsFromPageText(outcome.Text, outcome.PageNumber)...)
	}
	return headings, nil
}

// namesAChapter reports whether the entries name at least one chapter.
func namesAChapter(entries []domain.Heading) bool {
	for _, entry := range entries {
		if entry.Level == 2 {
			return true
		}
	}
	return false
}

// sourceHeadings asks one reading for its entries, treating an absent reading as
// the failure it is: a factory that built none must not panic the pipeline.
func sourceHeadings(ctx context.Context, source HeadingsSource) ([]domain.Heading, error) {
	if source == nil {
		return nil, errors.New("no structure reader was built")
	}
	return source.Headings(ctx)
}
