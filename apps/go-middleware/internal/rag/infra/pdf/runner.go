// Package pdf extracts pages from PDFs by driving the poppler command-line
// tools: pdfinfo for the page count, pdftotext for the text layer, pdfimages for
// the image inventory, and pdftoppm for page renders.
//
// The tools are the same family, so they share one runner and one error style
// rather than one package each. The runner is injectable, which is what makes
// every parser testable without a binary, and the constructors resolve the
// binaries up front so a missing installation aborts before page 1.
//
// Pages are 1-based everywhere, matching domain.PageOutcome.PageNumber.
package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner runs one external command and returns its standard output.
//
// It is the seam that keeps the parsers testable: a test supplies a fake that
// returns canned poppler output, and no binary is required. Standard error is
// captured by the implementation and carried on the returned error.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// ExecRunner is the production Runner. It runs the named binary with
// exec.CommandContext, captures stdout separately from stderr, and returns
// stdout only on success.
//
// On a non-zero exit the error names the command, includes the exit status, and
// carries the command's trimmed stderr, so a caller sees what poppler said rather
// than a bare "exit status 1". A binary that cannot be found yields a message
// naming the binary and pointing at poppler, because the failure a user is most
// likely to hit is simply not having installed the tools.
func ExecRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, missingBinary(name)
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdout.Bytes(), fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
		}

		return stdout.Bytes(), fmt.Errorf("%s: %w", name, err)
	}

	return stdout.Bytes(), nil
}

// resolveRunner returns the runner a constructor should use. A caller-supplied
// runner is used as-is, because it owns the process decision. A nil runner
// selects ExecRunner and verifies every named binary is on PATH first.
//
// The up-front check is deliberate: without it a missing binary is discovered
// when the first command runs, which on a long book can be hundreds of pages into
// a run. Looking it up in the constructor turns that into an immediate failure.
func resolveRunner(runner Runner, binaries ...string) (Runner, error) {
	if runner != nil {
		return runner, nil
	}

	for _, binary := range binaries {
		if _, err := exec.LookPath(binary); err != nil {
			return nil, missingBinary(binary)
		}
	}
	return ExecRunner, nil
}

// missingBinary describes a binary that is not on PATH. It names the binary and
// the package that provides it, since the fix is an install, not a code change.
func missingBinary(name string) error {
	return fmt.Errorf("%s: executable file not found in $PATH (poppler-utils is required; on macOS: brew install poppler)", name)
}
