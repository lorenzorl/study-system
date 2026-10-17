package pdf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
)

// renderDPI is the resolution page renders are produced at.
//
// It is a constant, not a configuration knob: the vision path needs one
// resolution that is legible without being enormous, and 150 dpi satisfies both.
// Making it configurable would invite tuning that nothing measures.
const renderDPI = 150

// Renderer is the poppler implementation of domain.PageRenderer. It renders one
// page at a time with pdftoppm.
type Renderer struct {
	pdfPath string
	workDir string
	runner  Runner
}

// NewRenderer builds a renderer over pdfPath, using workDir as the parent of the
// per-render scratch directory.
//
// A nil runner selects the real poppler binaries and verifies pdftoppm is
// installed, so a missing tool fails here rather than mid-run.
func NewRenderer(pdfPath, workDir string, runner Runner) (*Renderer, error) {
	resolved, err := resolveRunner(runner, "pdftoppm")
	if err != nil {
		return nil, err
	}
	return &Renderer{pdfPath: pdfPath, workDir: workDir, runner: resolved}, nil
}

// RenderPage returns PNG bytes of one page.
//
// It renders into a fresh temporary directory under workDir and removes that
// directory on every path, including errors. pdftoppm is run with -singlefile so
// it writes exactly "<prefix>.png"; without it the produced name is padded
// differently for different page counts, which is not something a caller should
// have to know. The temporary directory is what keeps a 359-page book from
// leaving hundreds of orphan images behind.
//
// A page below 1 is apperror.ErrInvalidInput and is rejected before anything is
// spawned.
func (r *Renderer) RenderPage(ctx context.Context, page int) ([]byte, error) {
	if page < 1 {
		return nil, fmt.Errorf("page must be at least 1, got %d: %w", page, apperror.ErrInvalidInput)
	}

	dir, err := os.MkdirTemp(r.workDir, "render-")
	if err != nil {
		return nil, fmt.Errorf("create render directory under %q: %w", r.workDir, err)
	}
	defer os.RemoveAll(dir)

	prefix := filepath.Join(dir, "page")
	_, err = r.runner(ctx, "pdftoppm",
		"-f", strconv.Itoa(page),
		"-l", strconv.Itoa(page),
		"-r", strconv.Itoa(renderDPI),
		"-png",
		"-singlefile",
		r.pdfPath, prefix,
	)
	if err != nil {
		return nil, fmt.Errorf("render page %d: %w", page, err)
	}

	rendered, err := os.ReadFile(prefix + ".png")
	if err != nil {
		return nil, fmt.Errorf("read rendered page %d: %w", page, err)
	}
	return rendered, nil
}
