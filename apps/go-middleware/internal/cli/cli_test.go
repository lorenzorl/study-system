package cli

import (
	"bytes"
	"context"
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The dispatch is tested without a process: Run takes its streams and its
// arguments, so an exit code and the exact text on each stream are observable.

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	var out, errOut bytes.Buffer
	code = Run(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRun_NoArgumentsPrintsUsageOnStderrAndFails(t *testing.T) {
	code, stdout, stderr := runCLI(t)

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "usage: study-system <command>")
	assert.Contains(t, stderr, "convert")
}

func TestRun_HelpPrintsUsageOnStdoutAndSucceeds(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		code, stdout, stderr := runCLI(t, arg)

		assert.Equal(t, 0, code, "arg %q", arg)
		assert.Contains(t, stdout, "usage: study-system <command>", "arg %q", arg)
		assert.Empty(t, stderr, "arg %q", arg)
	}
}

func TestRun_UnknownCommandFails(t *testing.T) {
	code, stdout, stderr := runCLI(t, "nope")

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, `unknown command "nope"`)
	assert.Contains(t, stderr, "commands:")
}

func TestRun_CommandHelpSucceeds(t *testing.T) {
	code, stdout, stderr := runCLI(t, "search", "-h")

	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "usage: study-system search")
	assert.Empty(t, stderr)
}

func TestRun_SearchWithoutAQueryIsAUsageError(t *testing.T) {
	code, stdout, stderr := runCLI(t, "search")

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "usage: study-system search")
}

func TestRun_EmbedWithoutAKeyNamesTheVariable(t *testing.T) {
	t.Setenv("STUDY_OPENROUTER_API_KEY", "")
	t.Chdir(t.TempDir())

	code, stdout, stderr := runCLI(t, "embed")

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "embed: ")
	assert.Contains(t, stderr, "STUDY_OPENROUTER_API_KEY")
}

func TestIntersperse_MovesFlagsBeforePositionals(t *testing.T) {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	limit := fs.Int("limit", 10, "")
	verbose := fs.Bool("verbose", false, "")

	cases := []struct {
		name    string
		args    []string
		limit   int
		verbose bool
	}{
		{"flag first", []string{"-limit", "3", "figure"}, 3, false},
		{"flag last", []string{"figure", "-limit", "3"}, 3, false},
		{"joined value", []string{"figure", "-limit=3"}, 3, false},
		{"bool after positional", []string{"figure", "-verbose"}, 10, true},
		{"two positionals", []string{"shortest", "path", "-limit", "3"}, 3, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			*limit = 10
			*verbose = false

			require.NoError(t, fs.Parse(intersperse(fs, tc.args)))

			assert.Equal(t, tc.limit, *limit, "the limit flag must survive wherever it appears")
			assert.Equal(t, tc.verbose, *verbose)
			assert.NotEmpty(t, fs.Args(), "the positional arguments must remain")
		})
	}
}

func TestIntersperse_KeepsEverythingAfterADoubleDashPositional(t *testing.T) {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	limit := fs.Int("limit", 10, "")

	require.NoError(t, fs.Parse(intersperse(fs, []string{"-limit", "3", "--", "-not-a-flag"})))

	assert.Equal(t, 3, *limit)
	assert.Equal(t, []string{"-not-a-flag"}, fs.Args())
}
