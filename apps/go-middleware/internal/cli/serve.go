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
func runServe(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("serve", "Run the HTTP API until interrupted.", stderr)
	port := fs.String("port", "", "HTTP listen port; overrides STUDY_PORT")
	if err := parseFlags(fs, args, stdout); err != nil {
		return err
	}
	if err := rejectExtraArgs(fs); err != nil {
		return err
	}

	cfg, err := loadConfig()
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

// loadConfig is the one place every command resolves configuration, so a load
// failure reads the same from any of them.
func loadConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	return cfg, nil
}
