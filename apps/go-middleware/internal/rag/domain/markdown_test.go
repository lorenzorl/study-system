package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// generated is the instant the header is rendered at. Pinning it keeps the
// expected document comparable as text.
var generated = time.Date(2026, 10, 17, 15, 0, 0, 0, time.UTC)

func markdownOutcome(page int, status domain.PageStatus, text string) domain.PageOutcome {
	method := domain.MethodExtract
	if status == domain.PageTranscribed {
		method = domain.MethodVision
	}

	return domain.PageOutcome{
		PageNumber: page,
		Status:     status,
		Method:     method,
		Text:       text,
		UpdatedAt:  generated,
	}
}

func markdownHeader(totalPages int) domain.MarkdownHeader {
	return domain.MarkdownHeader{Title: "DDD", TotalPages: totalPages, Generated: generated}
}

// linesStartingWith counts lines whose trimmed content starts with prefix. It is
// how "exactly one header" and "exactly one marker per page" are checked without
// depending on the exact surrounding whitespace.
func linesStartingWith(document, prefix string) []string {
	var found []string
	for _, line := range strings.Split(document, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			found = append(found, strings.TrimSpace(line))
		}
	}
	return found
}

func TestRenderMarkdown_HasExactlyOneHeaderLine(t *testing.T) {
	outcomes := []domain.PageOutcome{
		markdownOutcome(1, domain.PageExtracted, "first page"),
		markdownOutcome(2, domain.PageExtracted, "second page"),
	}

	document := domain.RenderMarkdown(markdownHeader(2), outcomes)

	headers := linesStartingWith(document, "# ")
	assert.Equal(t, []string{"# Processed Book for RAG"}, headers,
		"a second header in the middle of the file is a poisoned chunk in a retrieval index")
}

func TestRenderMarkdown_EmitsOneMarkerPerPageAscending(t *testing.T) {
	outcomes := []domain.PageOutcome{
		markdownOutcome(1, domain.PageExtracted, "one"),
		markdownOutcome(2, domain.PageTranscribed, "two"),
		markdownOutcome(3, domain.PageExtracted, "three"),
	}

	document := domain.RenderMarkdown(markdownHeader(3), outcomes)

	markers := linesStartingWith(document, "<!-- page")
	assert.Equal(t, []string{
		"<!-- page 1 -->",
		"<!-- page 2 -->",
		"<!-- page 3 -->",
	}, markers)
}

// A resumed run appends the pages it processed to the ones an earlier run
// recorded. The header is still singular, which is the defect this rendering
// function exists to prevent.
func TestRenderMarkdown_MarkersReflectEveryRecordedPageAfterAResume(t *testing.T) {
	outcomes := []domain.PageOutcome{
		markdownOutcome(1, domain.PageExtracted, "alpha recorded by the first run"),
		markdownOutcome(2, domain.PageExtracted, "bravo recorded by the first run"),
		markdownOutcome(3, domain.PageExtracted, "charlie appended by the resumed run"),
		markdownOutcome(4, domain.PageTranscribed, "delta appended by the resumed run"),
	}

	document := domain.RenderMarkdown(markdownHeader(4), outcomes)

	assert.Len(t, linesStartingWith(document, "# "), 1)
	assert.Equal(t, []string{
		"<!-- page 1 -->",
		"<!-- page 2 -->",
		"<!-- page 3 -->",
		"<!-- page 4 -->",
	}, linesStartingWith(document, "<!-- page"))

	for page, text := range map[int]string{
		1: "alpha recorded by the first run",
		2: "bravo recorded by the first run",
		3: "charlie appended by the resumed run",
		4: "delta appended by the resumed run",
	} {
		assert.Equal(t, 1, strings.Count(document, text),
			"page %d's text must appear exactly once", page)
	}
}

func TestRenderMarkdown_MarksFailedPagesAndOmitsTheirText(t *testing.T) {
	failed := markdownOutcome(2, domain.PageFailed, "")
	failed.Error = "pdftotext exited 1"

	outcomes := []domain.PageOutcome{
		markdownOutcome(1, domain.PageExtracted, "good page"),
		failed,
		markdownOutcome(3, domain.PageExtracted, "another good page"),
	}

	document := domain.RenderMarkdown(markdownHeader(3), outcomes)

	assert.Contains(t, document, "<!-- page 2 (failed) -->")
	assert.Equal(t, []string{
		"<!-- page 1 -->",
		"<!-- page 2 (failed) -->",
		"<!-- page 3 -->",
	}, linesStartingWith(document, "<!-- page"))

	// The failure reason belongs in the database, not in the corpus.
	assert.NotContains(t, document, "pdftotext exited 1")
}

func TestRenderMarkdown_EmitsTextVerbatimApartFromTrailingWhitespace(t *testing.T) {
	text := "Aggregate root\n  indented detail with trailing spaces   \n"

	document := domain.RenderMarkdown(markdownHeader(1),
		[]domain.PageOutcome{markdownOutcome(1, domain.PageExtracted, text)})

	assert.Contains(t, document, "Aggregate root\n  indented detail with trailing spaces\n",
		"internal whitespace is content and must survive; only trailing whitespace is trimmed")
	assert.NotContains(t, document, "trailing spaces   \n")
}

func TestRenderMarkdown_HeaderCountsRecordedAndFailedPages(t *testing.T) {
	outcomes := []domain.PageOutcome{
		markdownOutcome(1, domain.PageExtracted, "one"),
		markdownOutcome(2, domain.PageTranscribed, "two"),
		markdownOutcome(3, domain.PageFailed, ""),
	}

	document := domain.RenderMarkdown(markdownHeader(3), outcomes)

	// Two pages produced text and one failed; the range describes the job, not
	// how far this particular run got.
	assert.Contains(t, document, "- pages: 1-3 (2 recorded, 1 failed)")
}

func TestRenderMarkdown_HeaderReflectsOnlyWhatWasRecorded(t *testing.T) {
	outcomes := []domain.PageOutcome{
		markdownOutcome(1, domain.PageExtracted, "one"),
		markdownOutcome(2, domain.PageExtracted, "two"),
	}

	document := domain.RenderMarkdown(markdownHeader(5), outcomes)

	assert.Contains(t, document, "- pages: 1-5 (2 recorded, 0 failed)")
	assert.Equal(t, 5, len(linesStartingWith(document, "<!-- page")),
		"a marker is emitted for every page in the job's range, recorded or not")
}

func TestRenderMarkdown_UnrecordedPagesCarryNoText(t *testing.T) {
	outcomes := []domain.PageOutcome{markdownOutcome(1, domain.PageExtracted, "only page")}

	document := domain.RenderMarkdown(markdownHeader(3), outcomes)

	assert.Contains(t, document, "<!-- page 2 -->")
	assert.NotContains(t, document, "(failed)", "an unrecorded page has not failed, it is simply not done")
}

func TestRenderMarkdown_IncludesTheSourceAndGenerationInstant(t *testing.T) {
	document := domain.RenderMarkdown(domain.MarkdownHeader{
		Title:      "Domain-Driven Design",
		TotalPages: 1,
		Generated:  generated,
	}, []domain.PageOutcome{markdownOutcome(1, domain.PageExtracted, "text")})

	assert.Contains(t, document, "- source: Domain-Driven Design")
	assert.Contains(t, document, "- generated: 2026-10-17T15:00:00Z",
		"the instant is RFC3339 UTC, so the header is unambiguous")
}

func TestRenderMarkdown_NoOutcomesStillProducesOneHeader(t *testing.T) {
	document := domain.RenderMarkdown(markdownHeader(2), nil)

	assert.Len(t, linesStartingWith(document, "# "), 1)
	assert.Equal(t, 2, len(linesStartingWith(document, "<!-- page")))
}

func TestRenderMarkdown_IgnoresOutcomesOutsideTheJobRange(t *testing.T) {
	outcomes := []domain.PageOutcome{
		markdownOutcome(1, domain.PageExtracted, "one"),
		markdownOutcome(9, domain.PageExtracted, "a stale row from another job"),
	}

	document := domain.RenderMarkdown(markdownHeader(2), outcomes)

	assert.NotContains(t, document, "a stale row from another job",
		"a page number outside the job's range must not leak into the output")
	assert.Equal(t, []string{
		"<!-- page 1 -->",
		"<!-- page 2 -->",
	}, linesStartingWith(document, "<!-- page"))
}
