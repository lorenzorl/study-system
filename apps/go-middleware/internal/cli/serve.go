package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/server"
)

// runServe mounts the HTTP server and blocks until the context is cancelled.
//
// The serving itself lives in internal/server, shared with cmd/server, so the two
// entrypoints cannot drift.
func runServe(ctx context.Context, args []string, opts options, stdout, stderr io.Writer) error {
	fs := newFlagSet("serve", "Run the HTTP API until interrupted.", stderr)
	port := fs.String("port", "", "HTTP listen port; overrides STUDY_PORT")
	if err := parseFlags(fs, args, stdout); err != nil {
		return err
	}
	if err := rejectExtraArgs(fs); err != nil {
		return err
	}

	cfg, err := loadConfig(opts, stderr)
	if err != nil {
		return err
	}

	listenPort := cfg.Port
	if *port != "" {
		listenPort, err = config.ParsePort(*port)
		if err != nil {
			return fmt.Errorf("invalid -port: %w", err)
		}
	}

	return server.Run(ctx, cfg, listenPort)
}

// options are the values a run resolved before dispatching: they come from the
// global flags, and they override the environment.
type options struct {
	home    string
	envFile string
}

// loadConfig is the one place every command resolves configuration, so a load
// failure reads the same from any of them, and so the notice below has one owner.
func loadConfig(opts options, stderr io.Writer) (*config.Config, error) {
	cfg, err := config.LoadWith(config.Options{Home: opts.home, EnvFile: opts.envFile})
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}

	// Say which root a run used when it is not the working directory. An installed
	// binary must never read or write a database in whatever directory happens to
	// be current without saying so; the repository layout, which is the unchanged
	// case, stays silent.
	if !cfg.RootIsTheWorkingDirectory() {
		fmt.Fprintf(stderr, "data root: %s\n", cfg.Root)
	}

	return cfg, nil
}
