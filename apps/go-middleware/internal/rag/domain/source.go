package domain

// PagePorts bundles the three page-level ports a conversion needs over one
// opened file.
//
// They are grouped rather than passed separately because they describe one
// source: a caller that opens a PDF opens all three over the same file, and a
// factory is what guarantees that. The three ports stay distinct in this package
// because the pipeline fake for each one is different.
type PagePorts struct {
	// Extractor reads the text layer.
	Extractor TextExtractor

	// Figures reports which pages carry a raster image.
	Figures FigureIndex

	// Renderer turns one page into PNG bytes for the vision path.
	Renderer PageRenderer
}

// PageSourceFactory opens one document's page ports.
//
// It is the seam that keeps the pipeline testable without poppler: production
// opens the real binaries, a test returns fakes, and the use case names neither.
//
// A missing poppler installation must fail here, at Open, rather than at the page
// that first needs it. On a long book that difference is hundreds of pages of
// work and a partially written output file.
type PageSourceFactory interface {
	// Open returns the page ports over pdfPath. workDir is the parent of the
	// transient per-render scratch space.
	Open(pdfPath, workDir string) (PagePorts, error)
}
