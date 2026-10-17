package pdf_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/infra/pdf"
)

// pngSignature is the eight-byte magic every PNG starts with.
var pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

// stubRunner is an injectable pdf.Runner that records every invocation and
// returns canned output. It keeps the parser tests independent of poppler: the
// parsers are exercised through the public adapters, and no binary is required.
type stubRunner struct {
	calls []string
	fn    func(name string, args []string) ([]byte, error)
}

// runner adapts the stub to the pdf.Runner signature.
func (s *stubRunner) runner() pdf.Runner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		s.calls = append(s.calls, strings.Join(append([]string{name}, args...), " "))
		if s.fn == nil {
			return nil, fmt.Errorf("unexpected call %s %v", name, args)
		}
		return s.fn(name, args)
	}
}

// fixturePath is the committed three-page fixture.
//
// It contains page 1 with text only, page 2 with text and a 2x2 grayscale image,
// and page 3 with the image only. testdata/make_fixture.go reproduces it.
func fixturePath(t *testing.T) string {
	t.Helper()
	return filepath.Join("testdata", "three-pages.pdf")
}

// requirePoppler skips a real-binary test when a needed tool is not installed,
// rather than failing on a machine that has the parsers but not poppler.
func requirePoppler(t *testing.T, binaries ...string) {
	t.Helper()
	for _, binary := range binaries {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("%s not installed; skipping real-binary test", binary)
		}
	}
}

// entries lists a directory's immediate names, for asserting that a render left
// nothing behind.
func entries(t *testing.T, dir string) []string {
	t.Helper()

	found, err := os.ReadDir(dir)
	require.NoError(t, err)

	names := make([]string, 0, len(found))
	for _, entry := range found {
		names = append(names, entry.Name())
	}
	return names
}

// png returns a byte slice starting with the PNG signature, for stubbing a
// pdftoppm output file.
func png() []byte {
	return append(bytes.Clone(pngSignature), bytes.Repeat([]byte{0x00}, 32)...)
}
