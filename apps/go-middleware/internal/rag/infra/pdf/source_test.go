package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/infra/pdf"
)

// A source is built in one call and opens all three ports over one file, with a
// fake runner standing in for poppler. This is what lets the pipeline run in
// tests with no binary installed.
func TestSource_OpenWiresTheThreePortsOverOneFile(t *testing.T) {
	runner := &stubRunner{fn: func(name string, args []string) ([]byte, error) {
		switch name {
		case "pdfinfo":
			return []byte("Pages:          3\n"), nil
		case "pdftotext":
			return []byte("Page one text\n\f"), nil
		case "pdfimages":
			return []byte("page   num  type   width height\n1      0    image  10    10\n"), nil
		default:
			return nil, nil
		}
	}}

	source := pdf.NewSource(runner.runner())

	ports, err := source.Open(fixturePath(t), t.TempDir())
	require.NoError(t, err)

	pages, err := ports.Extractor.PageCount(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, pages)

	text, err := ports.Extractor.PageText(t.Context(), 1)
	require.NoError(t, err)
	assert.Equal(t, "Page one text", text)

	counts, err := ports.Figures.ImageCounts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[int]int{1: 1}, counts)

	assert.NotNil(t, ports.Renderer)
}

// The production path passes a nil runner, which makes every constructor check
// that its binary is installed. With an empty PATH that check must fail at Open,
// not at the page that first needs the tool.
func TestSource_OpenWithNoRunnerFailsFastWhenPopplerIsMissing(t *testing.T) {
	t.Setenv("PATH", "")

	_, err := pdf.NewSource(nil).Open(fixturePath(t), t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pdfinfo", "the failure must name the missing binary")
}
