package pdf_test

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/infra/pdf"
)

func TestRenderer_ReturnsTheRenderedPNGAndLeavesNothingBehind(t *testing.T) {
	workDir := t.TempDir()
	rendered := png()

	stub := &stubRunner{fn: func(_ string, args []string) ([]byte, error) {
		prefix := args[len(args)-1]
		require.NoError(t, os.WriteFile(prefix+".png", rendered, 0o644))
		return nil, nil
	}}

	renderer, err := pdf.NewRenderer("book.pdf", workDir, stub.runner())
	require.NoError(t, err)

	data, err := renderer.RenderPage(t.Context(), 3)
	require.NoError(t, err)
	assert.Equal(t, rendered, data)

	require.Len(t, stub.calls, 1)
	call := stub.calls[0]
	assert.Contains(t, call, "-f 3 -l 3")
	assert.Contains(t, call, "-r 150")
	assert.Contains(t, call, "-singlefile", "-singlefile is what makes the output name predictable")

	assert.Empty(t, entries(t, workDir), "a render must remove its scratch directory")
}

func TestRenderer_CleansUpEvenWhenRenderingFails(t *testing.T) {
	workDir := t.TempDir()

	stub := &stubRunner{fn: func(_ string, args []string) ([]byte, error) {
		prefix := args[len(args)-1]
		require.NoError(t, os.WriteFile(prefix+".png", png(), 0o644))
		return nil, errors.New("pdftoppm: exit status 1: boom")
	}}

	renderer, err := pdf.NewRenderer("book.pdf", workDir, stub.runner())
	require.NoError(t, err)

	_, err = renderer.RenderPage(t.Context(), 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	assert.Empty(t, entries(t, workDir), "a failed render must not leak its scratch directory")
}

func TestRenderer_RejectsPageBelowOneWithoutSpawning(t *testing.T) {
	workDir := t.TempDir()

	stub := &stubRunner{fn: func(string, []string) ([]byte, error) {
		return nil, errors.New("must not run")
	}}

	renderer, err := pdf.NewRenderer("book.pdf", workDir, stub.runner())
	require.NoError(t, err)

	_, err = renderer.RenderPage(t.Context(), 0)
	assert.ErrorIs(t, err, apperror.ErrInvalidInput)

	assert.Empty(t, stub.calls, "an impossible page number must abort before any process is spawned")
	assert.Empty(t, entries(t, workDir))
}
