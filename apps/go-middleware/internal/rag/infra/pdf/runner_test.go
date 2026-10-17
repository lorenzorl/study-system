package pdf_test

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/infra/pdf"
)

func TestExecRunner_SurfacesStderrAndExitCode(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available; skipping process-failure test")
	}

	_, err := pdf.ExecRunner(t.Context(), "sh", "-c", "echo boom >&2; exit 7")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sh", "the command name must be named")
	assert.Contains(t, err.Error(), "boom", "the command's stderr must be included")
	assert.Contains(t, err.Error(), "exit status 7", "the exit code must be included")
}

func TestExecRunner_MissingBinaryNamesTheBinaryAndPoppler(t *testing.T) {
	const binary = "study-system-missing-binary-for-tests"

	_, err := pdf.ExecRunner(t.Context(), binary)
	require.Error(t, err)
	assert.Contains(t, err.Error(), binary, "a missing binary must name itself")
	assert.Contains(t, err.Error(), "poppler-utils is required")
}

// The fail-fast guarantee: with the default runner, a missing binary is reported
// by the constructor rather than when the page that first needs it is processed.
//
// PATH points at an empty directory, so the check cannot pass by accident on a
// machine that happens to have poppler installed. That is what makes this test
// mean the same thing everywhere, instead of being a tautology here and a
// failure on a fresh machine.
func TestConstructors_FailFastWhenPopplerIsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	for name, build := range map[string]func() error{
		"NewExtractor": func() error {
			_, err := pdf.NewExtractor("book.pdf", nil)
			return err
		},
		"NewFigureIndex": func() error {
			_, err := pdf.NewFigureIndex("book.pdf", nil)
			return err
		},
		"NewRenderer": func() error {
			_, err := pdf.NewRenderer("book.pdf", t.TempDir(), nil)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := build()
			require.Error(t, err, "a missing poppler must abort the constructor")
			assert.Contains(t, err.Error(), "poppler-utils is required")
			assert.Contains(t, err.Error(), "$PATH", "the message must say where it looked")
		})
	}
}

// The counterpart of the fail-fast check: an injected runner owns the process
// decision, so no binary is looked up. This is what keeps the parser tests
// hermetic on a machine without poppler, and it is asserted rather than assumed
// because losing it would silently start requiring an installation to run tests.
func TestConstructors_DoNotLookForBinariesWhenARunnerIsInjected(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	stub := &stubRunner{fn: func(string, []string) ([]byte, error) { return nil, nil }}

	_, err := pdf.NewExtractor("book.pdf", stub.runner())
	require.NoError(t, err, "an injected runner means no binary is needed")

	_, err = pdf.NewFigureIndex("book.pdf", stub.runner())
	require.NoError(t, err)

	_, err = pdf.NewRenderer("book.pdf", t.TempDir(), stub.runner())
	require.NoError(t, err)
}
