package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/server"
)

// main is the server entrypoint: flags, configuration, and one call.
//
// The serve loop lives in internal/server so the CLI can mount the same one. This
// file is what startup looked like before that package existed, minus the wiring.
func main() {
	portFlag := flag.String("port", "", "HTTP listen port; overrides STUDY_PORT")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}

	port := cfg.Port
	if *portFlag != "" {
		port, err = config.ParsePort(*portFlag)
		if err != nil {
			log.Fatalf("invalid -port: %v", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, cfg, port); err != nil {
		log.Fatal(err)
	}
}
