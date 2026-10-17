package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// GeneratedHeaderLine is the first non-empty line of every file this package
// materializes.
//
// It is the marker that tells a previously generated book from a Markdown note a
// human wrote at the same path (backlog P3): the pipeline may overwrite its own
// output on a re-run, but it must never silently destroy someone else's note.
//
// The constant is exported and RenderMarkdown writes this exact value, so the
// marker a caller checks with IsGeneratedMarkdown can never drift from the one
// the writer emits.
const GeneratedHeaderLine = "# Processed Book for RAG"

// IsGeneratedMarkdown reports whether content was produced by RenderMarkdown.
//
// It is true only when the first non-empty line equals GeneratedHeaderLine. A
// handwritten note, an empty file, and a generated file that someone prepended a
// line to are all false. The leading-line check is deliberately strict: a file
// that merely mentions the phrase somewhere is still the caller's file.
func IsGeneratedMarkdown(content []byte) bool {
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		return trimmed == GeneratedHeaderLine
	}
	return false
}

// MarkdownHeader carries the document-level facts of one materialized run.
type MarkdownHeader struct {
	// Title is the document's human-readable name, shown as the source line.
	Title string

	// TotalPages is the job's page count. Markers are emitted for every page
	// from 1 to this number, so the output describes the whole book even when an
	// earlier run recorded the pages this run did not touch.
	TotalPages int

	// Generated is the instant the document was materialized.
	Generated time.Time
}

// RenderMarkdown materializes recorded page outcomes as one Markdown document.
//
// It is a pure function: it decides the output shape and does no I/O, so the
// written file's structure can be tested without a database or a filesystem.
//
// Exactly one "# " header is emitted, and exactly one page marker per page from 1
// to TotalPages, ascending. That is the whole point of the function. The retired
// script wrote its range before processing, so a run that died at page 179 left a
// header claiming "Pages 1 to 359" and the book was finished by concatenating a
// second file, which put a second header in the middle of the corpus. Collapsing
// the shape into one function is what makes "one header" a property instead of a
// habit.
//
// Text is emitted verbatim apart from trailing whitespace, so the output still
// matches pdftotext character for character on a born-digital book. A failed page
// is marked and carries no text: it is persisted as a failure, and the next run
// retries it rather than shipping a guess.
func RenderMarkdown(header MarkdownHeader, outcomes []PageOutcome) string {
	progress := ProgressOf(header.TotalPages, outcomes)

	// Keep the latest outcome per page, exactly as ProgressOf does, so a page
	// recorded twice cannot leak two texts into the output.
	latest := make(map[int]PageOutcome, len(outcomes))
	for _, outcome := range outcomes {
		if outcome.PageNumber < 1 || outcome.PageNumber > header.TotalPages {
			continue
		}
		existing, seen := latest[outcome.PageNumber]
		if !seen || !outcome.UpdatedAt.Before(existing.UpdatedAt) {
			latest[outcome.PageNumber] = outcome
		}
	}

	var builder strings.Builder
	builder.WriteString(GeneratedHeaderLine)
	builder.WriteString("\n\n")
	fmt.Fprintf(&builder, "- source: %s\n", header.Title)
	fmt.Fprintf(&builder, "- pages: 1-%d (%d recorded, %d failed)\n",
		header.TotalPages, progress.Succeeded(), progress.Failed)
	fmt.Fprintf(&builder, "- generated: %s\n", header.Generated.UTC().Format(time.RFC3339))

	for page := 1; page <= header.TotalPages; page++ {
		// The blank line before each marker is what separates page blocks in the
		// rendered Markdown.
		builder.WriteString("\n")

		outcome, recorded := latest[page]
		if recorded && outcome.Status == PageFailed {
			fmt.Fprintf(&builder, "<!-- page %d (failed) -->\n", page)
			continue
		}

		fmt.Fprintf(&builder, "<!-- page %d -->\n", page)
		if recorded {
			if text := strings.TrimRightFunc(outcome.Text, unicode.IsSpace); text != "" {
				builder.WriteString(text)
				builder.WriteString("\n")
			}
		}
	}

	return builder.String()
}
