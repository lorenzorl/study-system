// Package cli is the command surface: one binary, one subcommand per job.
//
// Run takes the arguments, the two output streams, and a context, and returns an
// exit code. Nothing here calls os.Exit or reads os.Args, which is what makes the
// whole surface testable without a process.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// programName is what the usage text calls the binary.
const programName = "study-system"

// errHelp and errUsage are control values, not failures: the flag package has
// already written the message and the usage text, so Run must not write again.
var (
	errHelp  = errors.New("help requested")
	errUsage = errors.New("usage")
)

// command is one subcommand.
type command struct {
	name    string
	summary string
	run     func(ctx context.Context, args []string, opts options, stdout, stderr io.Writer) error
}

// commands is the whole surface, in the order the usage text lists it.
func commands() []command {
	return []command{
		{"serve", "run the HTTP server", runServe},
		{"search", "search the chunk index", runSearch},
		{"reindex", "rebuild the chunk index of every document", runReindex},
		{"embed", "rebuild the embeddings of every document", runEmbed},
		{"convert", "convert a PDF to Markdown and index it", runConvert},
		{"list", "list the stored artifacts", runList},
		{"due", "list the flashcards that are due now", runDue},
		{"review", "record a review of one flashcard", runReview},
		{"topics", "list the study topics", runTopics},
		{"concepts", "list the concepts of a topic", runConcepts},
		{"backup", "replicate the database and the artifacts to R2", runBackup},
	}
}

// Run dispatches one subcommand and returns the process exit code.
//
// Exit codes are the ones the previous binaries used: 0 for success, 1 for any
// failure, including a usage error. A distinct code for usage would be nicer and
// would also be a change no caller asked for.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, rest, err := parseGlobals(args, stderr)
	if err != nil {
		if errors.Is(err, errHelp) {
			printUsage(stdout)
			return 0
		}
		return 1
	}

	if len(rest) == 0 {
		printUsage(stderr)
		return 1
	}

	switch rest[0] {
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	}

	for _, cmd := range commands() {
		if cmd.name != rest[0] {
			continue
		}

		err := cmd.run(ctx, rest[1:], opts, stdout, stderr)
		switch {
		case err == nil:
			return 0
		case errors.Is(err, errHelp):
			return 0
		case errors.Is(err, errUsage):
			return 1
		default:
			fmt.Fprintf(stderr, "%s: %v\n", cmd.name, err)
			return 1
		}
	}

	fmt.Fprintf(stderr, "unknown command %q\n\n", rest[0])
	printUsage(stderr)
	return 1
}

// parseGlobals reads the flags that apply to every command and returns the
// options with the remaining arguments.
//
// They must come before the command name, the way git takes -C: flag.Parse stops
// at the first non-flag argument, so the remainder is the command and its own
// arguments. Accepting them after the command would make -home ambiguous with a
// command's own flags, which is a conflict worth refusing rather than guessing at.
func parseGlobals(args []string, stderr io.Writer) (options, []string, error) {
	fs := flag.NewFlagSet(programName, flag.ContinueOnError)
	fs.SetOutput(stderr)
	// -h here asks for the program's usage, not this flag set's, which lists two
	// flags and is not what someone typing "study-system -h" wants.
	fs.Usage = func() {}

	home := fs.String("home", "", "directory holding .env and data/; overrides STUDY_HOME")
	envFile := fs.String("env-file", "", "configuration file to read; overrides STUDY_ENV_FILE")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, nil, errHelp
		}
		return options{}, nil, errUsage
	}

	return options{home: *home, envFile: *envFile}, fs.Args(), nil
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s [-home DIR] [-env-file FILE] <command> [flags]\n\ncommands:\n", programName)
	for _, cmd := range commands() {
		fmt.Fprintf(w, "  %-8s %s\n", cmd.name, cmd.summary)
	}
	fmt.Fprintf(w, "\nGlobal flags, before the command:\n")
	fmt.Fprintf(w, "  -home DIR       directory holding .env and data/; overrides STUDY_HOME\n")
	fmt.Fprintf(w, "  -env-file FILE  configuration file to read; overrides STUDY_ENV_FILE\n")
	fmt.Fprintf(w, "\nRun \"%s <command> -h\" for the flags of one command.\n", programName)
}

// newFlagSet builds the flag set for one subcommand: named, quieter than the
// default, and with a usage line that names the program and the command.
func newFlagSet(name, synopsis string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: %s %s [flags]\n\n%s\n\n", programName, name, synopsis)
		fs.PrintDefaults()
	}
	return fs
}

// parseFlags parses args and routes the flag package's outcomes onto the control
// values Run understands. Help goes to stdout and succeeds, the way a command line
// is expected to answer -h; a parse failure is a usage error the flag package has
// already written to stderr.
//
// The arguments are reordered first, so "search figure -limit 3" and
// "search -limit 3 figure" mean the same thing. Without that, the flag package
// stops at the first positional and would hand the trailing flag to the command as
// part of its query — a wrong answer that looks like a normal one.
func parseFlags(fs *flag.FlagSet, args []string, stdout io.Writer) error {
	if asksForHelp(args) {
		// Help goes to stdout; the writer is restored so the flag set keeps writing
		// errors to stderr if the command continues past this point.
		stderr := fs.Output()
		fs.SetOutput(stdout)
		fs.Usage()
		fs.SetOutput(stderr)
		return errHelp
	}

	if err := fs.Parse(intersperse(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return errHelp
		}
		return errUsage
	}
	return nil
}

// asksForHelp reports whether args ask for the command's usage before any "--"
// separator hands the rest to the command as positional arguments.
func asksForHelp(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--":
			return false
		case "-h", "-help", "--help":
			return true
		}
	}
	return false
}

// intersperse returns the flags first and the positional arguments after them,
// preserving the order within each group and separating the two with "--" so a
// positional that begins with a dash is not parsed as a flag. A flag that takes
// its value as the next argument keeps them together; a boolean flag consumes
// nothing.
func intersperse(fs *flag.FlagSet, args []string) []string {
	takesValue := make(map[string]bool)
	fs.VisitAll(func(f *flag.Flag) {
		if boolean, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && boolean.IsBoolFlag() {
			return
		}
		takesValue["-"+f.Name] = true
		takesValue["--"+f.Name] = true
	})

	flags := make([]string, 0, len(args))
	positional := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}

		if len(arg) > 1 && arg[0] == '-' {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && takesValue[arg] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}

		positional = append(positional, arg)
	}

	ordered := make([]string, 0, len(args)+1)
	ordered = append(ordered, flags...)
	if len(positional) > 0 {
		ordered = append(ordered, "--")
		ordered = append(ordered, positional...)
	}
	return ordered
}

// rejectExtraArgs fails when a command takes no positional arguments.
func rejectExtraArgs(fs *flag.FlagSet) error {
	if fs.NArg() > 0 {
		fs.Usage()
		return errUsage
	}
	return nil
}
