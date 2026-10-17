package pdf_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/infra/pdf"
)

func TestExtractor_PageCountReadsThePagesLine(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte("Title:          three-pages\nAuthor:         \nPages:           3\nEncrypted:      no\n"), nil
	}}

	extractor, err := pdf.NewExtractor("book.pdf", stub.runner())
	require.NoError(t, err)

	pages, err := extractor.PageCount(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, pages)
}

func TestExtractor_PageCountRejectsMissingPagesLine(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte("Title:          no page count here\n"), nil
	}}

	extractor, err := pdf.NewExtractor("book.pdf", stub.runner())
	require.NoError(t, err)

	_, err = extractor.PageCount(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Pages line")
}

func TestExtractor_PageCountRejectsUnparseablePagesLine(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte("Pages:           many\n"), nil
	}}

	extractor, err := pdf.NewExtractor("book.pdf", stub.runner())
	require.NoError(t, err)

	_, err = extractor.PageCount(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "many")
}

func TestExtractor_PageTextStripsTheFormFeed(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte("Page one text\n\n\f"), nil
	}}

	extractor, err := pdf.NewExtractor("book.pdf", stub.runner())
	require.NoError(t, err)

	text, err := extractor.PageText(t.Context(), 2)
	require.NoError(t, err)
	assert.Equal(t, "Page one text", text, "the trailing newlines and form feed are separators, not content")
	assert.Equal(t, []string{"pdftotext -f 2 -l 2 book.pdf -"}, stub.calls)
}

func TestExtractor_PageTextOfAnImageOnlyPageIsEmptyNotAnError(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return []byte("\f"), nil
	}}

	extractor, err := pdf.NewExtractor("book.pdf", stub.runner())
	require.NoError(t, err)

	text, err := extractor.PageText(t.Context(), 3)
	require.NoError(t, err, "a page with no text layer is a normal scanned page, not a failure")
	assert.Equal(t, "", text)
}

func TestExtractor_RejectsPageBelowOneWithoutSpawning(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return nil, errors.New("must not run")
	}}

	extractor, err := pdf.NewExtractor("book.pdf", stub.runner())
	require.NoError(t, err)

	_, err = extractor.PageText(t.Context(), 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperror.ErrInvalidInput)

	_, err = extractor.PageText(t.Context(), -1)
	assert.ErrorIs(t, err, apperror.ErrInvalidInput)

	assert.Empty(t, stub.calls, "an impossible page number must abort before any process is spawned")
}

func TestExtractor_SurfacesProcessFailure(t *testing.T) {
	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return nil, errors.New("pdftotext: exit status 99: Wrong page range given")
	}}

	extractor, err := pdf.NewExtractor("book.pdf", stub.runner())
	require.NoError(t, err)

	_, err = extractor.PageText(t.Context(), 4)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Wrong page range", "the process message must survive the wrap")
}
