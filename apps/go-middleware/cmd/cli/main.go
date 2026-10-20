// Command study-system is the single entrypoint: one binary, one subcommand per
// job.
//
// It replaces cmd/ingest (whose modes are subcommands now) and shares the serve
// loop with cmd/server, so the process has one command surface and one server.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/cli"
)

func main() {
	// One context for the whole process: Ctrl-C or SIGTERM cancels whichever
	// command is running, and serve stops accepting requests before it drains.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
