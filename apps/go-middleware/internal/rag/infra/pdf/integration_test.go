package pdf_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/infra/pdf"
)

// These tests run the real poppler binaries over the committed fixture and skip
// when a tool is absent. The fixture's contents and its generator are documented
// in helpers_test.go and testdata/make_fixture.go.

func TestExtractor_RealPDF(t *testing.T) {
	requirePoppler(t, "pdfinfo", "pdftotext")

	extractor, err := pdf.NewExtractor(fixturePath(t), nil)
	require.NoError(t, err)

	pages, err := extractor.PageCount(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, pages)

	page1, err := extractor.PageText(t.Context(), 1)
	require.NoError(t, err)
	assert.Equal(t, "Page one text", page1)

	page2, err := extractor.PageText(t.Context(), 2)
	require.NoError(t, err)
	assert.Contains(t, page2, "Page two text")

	page3, err := extractor.PageText(t.Context(), 3)
	require.NoError(t, err)
	assert.Equal(t, "", page3, "the image-only page has no text layer")

	_, err = extractor.PageText(t.Context(), 4)
	require.Error(t, err, "a page beyond the last one must be reported, not silently swallowed")

	_, err = extractor.PageText(t.Context(), 0)
	assert.ErrorIs(t, err, apperror.ErrInvalidInput)
}

func TestFigureIndexer_RealPDF(t *testing.T) {
	requirePoppler(t, "pdfimages")

	index, err := pdf.NewFigureIndex(fixturePath(t), nil)
	require.NoError(t, err)

	counts, err := index.ImageCounts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[int]int{2: 1, 3: 1}, counts)
}

func TestRenderer_RealPDF(t *testing.T) {
	requirePoppler(t, "pdftoppm")

	workDir := t.TempDir()
	renderer, err := pdf.NewRenderer(fixturePath(t), workDir, nil)
	require.NoError(t, err)

	data, err := renderer.RenderPage(t.Context(), 3)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(data, pngSignature), "a render must be a PNG")
	assert.Greater(t, len(data), 1024, "a real render is far larger than 1 KB")

	assert.Empty(t, entries(t, workDir), "a render must not accumulate orphan images in the workspace")

	_, err = renderer.RenderPage(t.Context(), 0)
	assert.ErrorIs(t, err, apperror.ErrInvalidInput)
}
