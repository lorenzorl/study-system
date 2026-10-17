package pdf

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// FigureIndexer is the poppler implementation of domain.FigureIndex. It reads
// the image inventory with pdfimages -list.
type FigureIndexer struct {
	pdfPath string
	runner  Runner
}

// NewFigureIndex builds a figure index over pdfPath.
//
// A nil runner selects the real poppler binaries and verifies pdfimages is
// installed, so a missing tool fails here rather than mid-run.
func NewFigureIndex(pdfPath string, runner Runner) (*FigureIndexer, error) {
	resolved, err := resolveRunner(runner, "pdfimages")
	if err != nil {
		return nil, err
	}
	return &FigureIndexer{pdfPath: pdfPath, runner: resolved}, nil
}

// ImageCounts returns how many images each page carries. Pages with no image are
// absent from the map rather than mapped to zero.
func (f *FigureIndexer) ImageCounts(ctx context.Context) (map[int]int, error) {
	output, err := f.runner(ctx, "pdfimages", "-list", f.pdfPath)
	if err != nil {
		return nil, fmt.Errorf("list images of %q: %w", f.pdfPath, err)
	}

	counts, err := parseImageCounts(output)
	if err != nil {
		return nil, fmt.Errorf("list images of %q: %w", f.pdfPath, err)
	}
	return counts, nil
}

// parseImageCounts reads pdfimages -list output.
//
// The parse is header-driven: the column index of "page" and of "type" is read
// from the header line and then used on every data row. Hardcoding positions
// would break on a poppler release that reorders columns, and the break would be
// silent because every row would still parse as *something*.
//
// A row counts only when its type is exactly "image"; masks and other object
// types are reported in the same table and are not pictures. A data row that
// cannot be parsed is an error naming the line, never a silent skip: silently
// counting zero rows would look exactly like a book with no diagrams, and the
// feature would degrade with no symptom at all.
func parseImageCounts(output []byte) (map[int]int, error) {
	counts := map[int]int{}

	if strings.TrimSpace(string(output)) == "" {
		return counts, nil
	}

	lines := strings.Split(string(output), "\n")

	pageColumn, typeColumn, headerAt := -1, -1, -1
	for index, line := range lines {
		fields := strings.Fields(line)
		page := columnIndex(fields, "page")
		kind := columnIndex(fields, "type")
		if page >= 0 && kind >= 0 {
			pageColumn, typeColumn, headerAt = page, kind, index
			break
		}
	}
	if headerAt < 0 {
		return nil, errors.New("pdfimages output has no page/type header line")
	}

	for _, line := range lines[headerAt+1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.Trim(trimmed, "-") == "" {
			continue
		}

		fields := strings.Fields(trimmed)
		if pageColumn >= len(fields) || typeColumn >= len(fields) {
			return nil, fmt.Errorf("unparseable pdfimages row %q: fewer columns than the header declares", trimmed)
		}

		page, err := strconv.Atoi(fields[pageColumn])
		if err != nil {
			return nil, fmt.Errorf("unparseable pdfimages page in row %q: %w", trimmed, err)
		}

		if fields[typeColumn] != "image" {
			continue
		}
		counts[page]++
	}

	return counts, nil
}

// columnIndex returns the position of an exact field value, or -1.
func columnIndex(fields []string, name string) int {
	for index, field := range fields {
		if field == name {
			return index
		}
	}
	return -1
}
