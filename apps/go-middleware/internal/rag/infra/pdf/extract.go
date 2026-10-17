package pdf

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
)

// Extractor is the poppler implementation of domain.TextExtractor. It reads the
// page count with pdfinfo and page text with pdftotext.
type Extractor struct {
	pdfPath string
	runner  Runner
}

// NewExtractor builds an extractor over pdfPath.
//
// A nil runner selects the real poppler binaries and verifies pdfinfo and
// pdftotext are installed, so a missing tool fails here rather than at the page
// that first needs it.
func NewExtractor(pdfPath string, runner Runner) (*Extractor, error) {
	resolved, err := resolveRunner(runner, "pdfinfo", "pdftotext")
	if err != nil {
		return nil, err
	}
	return &Extractor{pdfPath: pdfPath, runner: resolved}, nil
}

// PageCount returns the document's page count from pdfinfo's Pages line.
func (e *Extractor) PageCount(ctx context.Context) (int, error) {
	output, err := e.runner(ctx, "pdfinfo", e.pdfPath)
	if err != nil {
		return 0, fmt.Errorf("read page count of %q: %w", e.pdfPath, err)
	}

	pages, err := parsePages(output)
	if err != nil {
		return 0, fmt.Errorf("read page count of %q: %w", e.pdfPath, err)
	}
	return pages, nil
}

// PageText returns one page's text. Trailing whitespace and form feeds are
// stripped, so a page that carries no text returns "" with a nil error.
//
// A page below 1 is apperror.ErrInvalidInput and is rejected here, before the
// runner is called: nothing is spawned for a number the domain already knows is
// impossible.
func (e *Extractor) PageText(ctx context.Context, page int) (string, error) {
	if page < 1 {
		return "", fmt.Errorf("page must be at least 1, got %d: %w", page, apperror.ErrInvalidInput)
	}

	output, err := e.runner(ctx, "pdftotext",
		"-f", strconv.Itoa(page),
		"-l", strconv.Itoa(page),
		e.pdfPath, "-",
	)
	if err != nil {
		return "", fmt.Errorf("extract text of page %d: %w", page, err)
	}

	// pdftotext ends every page with "\n\n\f"; the form feed is a page separator,
	// not page content, and must never reach the stored text.
	return strings.TrimRight(string(output), " \t\r\n\f\v"), nil
}

// parsePages reads the value of pdfinfo's "Pages:" line. A missing or
// unparseable value is an error: guessing a page count would silently truncate
// or overrun the document.
func parsePages(output []byte) (int, error) {
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Pages:") {
			continue
		}

		value := strings.TrimSpace(strings.TrimPrefix(trimmed, "Pages:"))
		pages, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("unparseable Pages value %q: %w", value, err)
		}
		return pages, nil
	}

	return 0, errors.New("pdfinfo output has no Pages line")
}
