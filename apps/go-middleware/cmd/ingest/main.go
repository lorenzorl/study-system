// Command ingest acquires a document and persists it as a stored artifact.
//
// This is the first slice of the ingestion pipeline: acquire and persist.
// Conversion, chunking, and embedding come later and will extend this command.
//
// It is a command rather than an HTTP endpoint on purpose: ingestion is long
// work, and a request that blocks for minutes is a request that fails.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"text/tabwriter"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("ingest: %v", err)
	}
}

func run() error {
	path := flag.String("pdf", "", "file to ingest, by path on this machine")
	list := flag.Bool("list", false, "list stored artifacts and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	store, err := storage.New(cfg.StorageRoot, cfg.MaxUploadBytes)
	if err != nil {
		return fmt.Errorf("open storage at %s: %w", cfg.StorageRoot, err)
	}

	if *list {
		return listArtifacts(store)
	}

	if *path == "" {
		flag.Usage()
		return errors.New("-pdf is required unless -list is used")
	}

	artifact, err := ingest(context.Background(), store, *path)
	if err != nil {
		return err
	}

	payloadPath, err := store.Path(artifact.ID)
	if err != nil {
		return fmt.Errorf("resolve payload path: %w", err)
	}

	workspace, err := store.Workspace(artifact.ID)
	if err != nil {
		return fmt.Errorf("prepare workspace: %w", err)
	}

	fmt.Printf("stored artifact\n")
	fmt.Printf("  id            %s\n", artifact.ID)
	fmt.Printf("  original name %s\n", artifact.OriginalName)
	fmt.Printf("  size          %d bytes\n", artifact.Size)
	fmt.Printf("  payload       %s\n", payloadPath)
	fmt.Printf("  workspace     %s\n", workspace)
	fmt.Printf("\nnext: convert. the payload path above is what a converter consumes.\n")

	return nil
}

// ingest opens path and streams it into the store.
func ingest(ctx context.Context, store *storage.Store, path string) (storage.Artifact, error) {
	info, err := os.Stat(path)
	if err != nil {
		return storage.Artifact{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	if info.IsDir() {
		return storage.Artifact{}, fmt.Errorf("%s is a directory: %w", path, apperror.ErrInvalidInput)
	}

	file, err := os.Open(path)
	if err != nil {
		return storage.Artifact{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	artifact, err := store.Put(ctx, info.Name(), file)
	if err != nil {
		if errors.Is(err, apperror.ErrTooLarge) {
			return storage.Artifact{}, fmt.Errorf(
				"%s is %d bytes, over the %d byte limit; raise %s to accept it",
				path, info.Size(), store.MaxBytes(), config.EnvMaxUploadBytes,
			)
		}
		return storage.Artifact{}, fmt.Errorf("store %s: %w", path, err)
	}

	return artifact, nil
}

func listArtifacts(store *storage.Store) error {
	artifacts, err := store.List()
	if err != nil {
		return err
	}

	if len(artifacts) == 0 {
		fmt.Printf("no artifacts in %s\n", store.Root())
		return nil
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "ID\tORIGINAL NAME\tSIZE\tSTORED AT")

	for _, artifact := range artifacts {
		fmt.Fprintf(writer, "%s\t%s\t%d\t%s\n",
			artifact.ID,
			artifact.OriginalName,
			artifact.Size,
			artifact.StoredAt.Format(time.RFC3339),
		)
	}

	return writer.Flush()
}
