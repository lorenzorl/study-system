package pdf

import (
	"fmt"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
)

// Source is the production implementation of domain.PageSourceFactory. It opens
// the text extractor, the figure index, and the page renderer over one file.
type Source struct {
	runner Runner
}

// NewSource builds a source factory over one runner.
//
// A nil runner is the production choice: it is passed to the three constructors
// unchanged, so each of them keeps its up-front "is poppler installed?" check and
// a missing binary fails at Open rather than at the page that first needs it.
// Passing an already-resolved runner would skip that check.
//
// A test supplies its own runner in this one call and gets all three ports over
// fake process output, with no binary required.
func NewSource(runner Runner) *Source {
	return &Source{runner: runner}
}

// Open returns the three page ports over pdfPath, using workDir as the parent of
// the renderer's transient scratch directories.
//
// A failure to resolve any of the poppler binaries is reported here. The renderer
// is opened last because its constructor is the only one that can fail for a
// reason other than a missing binary.
func (s *Source) Open(pdfPath, workDir string) (domain.PagePorts, error) {
	extractor, err := NewExtractor(pdfPath, s.runner)
	if err != nil {
		return domain.PagePorts{}, fmt.Errorf("open text extractor for %q: %w", pdfPath, err)
	}

	figures, err := NewFigureIndex(pdfPath, s.runner)
	if err != nil {
		return domain.PagePorts{}, fmt.Errorf("open figure index for %q: %w", pdfPath, err)
	}

	renderer, err := NewRenderer(pdfPath, workDir, s.runner)
	if err != nil {
		return domain.PagePorts{}, fmt.Errorf("open page renderer for %q: %w", pdfPath, err)
	}

	return domain.PagePorts{Extractor: extractor, Figures: figures, Renderer: renderer}, nil
}
