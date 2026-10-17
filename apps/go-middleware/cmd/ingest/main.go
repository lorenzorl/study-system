// Command ingest converts a PDF into one Markdown document for the retrieval
// index.
//
// It is a command rather than an HTTP endpoint on purpose: ingestion is long
// work, and a request that blocks for minutes is a request that fails.
//
// Resumability is the contract. The document is identified by the hash of its
// bytes and the job's per-page outcomes are persisted, so a run that dies at page
// 179 continues at page 180 instead of restarting, and the previous complete
// output file stays valid until a run settles.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"text/tabwriter"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/llm"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/infra/pdf"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
	studysqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("ingest: %v", err)
	}
}

func run() error {
	pdfPath := flag.String("pdf", "", "file to convert, by path on this machine")
	outPath := flag.String("out", "", "path of the Markdown file to write; required unless -list is used")
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

	// Listing reads the store and nothing else. It must keep working with no API
	// key, no database, and no PDF, so it returns before any of them is opened.
	if *list {
		return listArtifacts(store)
	}

	if *pdfPath == "" {
		flag.Usage()
		return errors.New("-pdf is required unless -list is used")
	}
	if *outPath == "" {
		flag.Usage()
		return errors.New("-out is required: the backend never guesses the vault path")
	}

	// The key is checked before the database is opened and before the first page
	// is read, so a missing key cannot cost hundreds of pages of work. config.Load
	// keeps it optional because the API server never ingests.
	if cfg.OpenRouterAPIKey == "" {
		return fmt.Errorf("%s is not set: the ingestion run cannot transcribe pages without it",
			config.EnvOpenRouterAPIKey)
	}

	client, err := llm.NewClient(cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, cfg.LLMModel, llm.Options{})
	if err != nil {
		return fmt.Errorf("create model client: %w", err)
	}

	ctx := context.Background()

	sqlDB, err := db.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer sqlDB.Close()

	// Each context owns its migration block; the composition root is what puts
	// them in one database, in one run, exactly as the server does.
	migrations := slices.Concat(studysqlite.Migrations(), ragsqlite.Migrations())
	if err := db.Migrate(ctx, sqlDB, migrations); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	useCase := app.NewIngestDocumentUseCase(
		ragsqlite.NewDocumentRepository(sqlDB),
		ragsqlite.NewJobRepository(sqlDB),
		ragsqlite.NewPageOutcomeRepository(sqlDB),
		// A nil runner selects the real poppler binaries, so a missing tool fails
		// at Open rather than at the page that first needs it.
		pdf.NewSource(nil),
		client,
		store,
	)

	report, runErr := useCase.Execute(ctx, app.IngestRequest{
		PDFPath:   *pdfPath,
		Title:     filepath.Base(*pdfPath),
		SourceURI: *pdfPath,
		OutPath:   *outPath,
	}, printPageEvent)

	// A partial report is printed even when the run aborted, so the failure is
	// reported next to how far it got.
	if report != nil {
		printSummary(report)
	}
	if runErr != nil {
		return describe(runErr)
	}

	if report.Extracted+report.Transcribed == 0 {
		return fmt.Errorf("no page produced text; the run converted nothing")
	}

	return nil
}

// printPageEvent writes one line per processed page.
func printPageEvent(event app.PageEvent) {
	line := fmt.Sprintf("page %d/%d  %-7s %s",
		event.Page, event.Total, event.Method, event.Elapsed.Round(100*time.Millisecond))
	if event.Err != "" {
		line += "  failed: " + event.Err
	} else {
		line += fmt.Sprintf("  %d chars", event.Chars)
	}

	fmt.Println(line)
}

// printSummary reports the job's totals, which include pages an earlier run
// recorded.
func printSummary(report *app.Report) {
	fmt.Println()
	fmt.Println("summary")
	fmt.Printf("  document    %s\n", report.DocumentID)
	fmt.Printf("  job         %s\n", report.JobID)
	fmt.Printf("  pages       %d (%d extracted, %d transcribed, %d failed, %d already recorded)\n",
		report.TotalPages, report.Extracted, report.Transcribed, report.Failed, report.Skipped)
	fmt.Printf("  settled     %t\n", report.Settled)
	fmt.Printf("  output      %s\n", report.OutPath)
	fmt.Printf("  duration    %s\n", report.Duration.Round(time.Second))
}

// describe adds the fix to a limit failure. The limit is configuration, so the
// message names the variable rather than only the number.
func describe(err error) error {
	if errors.Is(err, apperror.ErrTooLarge) {
		return fmt.Errorf("%w (raise %s to accept it)", err, config.EnvMaxUploadBytes)
	}
	return err
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
