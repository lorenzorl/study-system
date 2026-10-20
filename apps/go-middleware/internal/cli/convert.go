package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/composition"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	ragapp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
)

// runConvert converts one PDF into one Markdown document and indexes it.
//
// Resumability is the contract: the document is identified by the hash of its
// bytes and the job's per-page outcomes are persisted, so a run that dies at page
// 179 continues at page 180, and the previous complete output file stays valid.
func runConvert(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("convert",
		"Convert a PDF into Markdown, then rebuild that document's index.", stderr)
	pdfPath := fs.String("pdf", "", "file to convert, by path on this machine; required")
	outPath := fs.String("out", "", "path of the Markdown file to write; required")
	dryRun := fs.Bool("dry-run", false, "print the plan and stop before any model work or write")
	maxModelPages := fs.Int("max-model-pages", 0,
		"refuse the run when the plan would send more pages to the model than this (0 = no limit)")
	force := fs.Bool("force", false, "replace the output file even when this pipeline did not generate it")
	if err := parseFlags(fs, args, stdout); err != nil {
		return err
	}
	if err := rejectExtraArgs(fs); err != nil {
		return err
	}

	if *pdfPath == "" || *outPath == "" {
		fs.Usage()
		return errUsage
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// The key is checked before the database is opened and before the first page
	// is read, so a missing key cannot cost hundreds of pages of work. A dry run
	// is different: it plans without ever calling the model, so it needs no key,
	// and a text-only book needs none either. No client is built on a dry run, so
	// the use case receives no transcriber at all.
	var clients composition.Clients
	if !*dryRun {
		if cfg.OpenRouterAPIKey == "" {
			return fmt.Errorf("%s is not set: the conversion cannot transcribe pages without it",
				config.EnvOpenRouterAPIKey)
		}

		// The conversion only transcribes pages: the index and the vectors are
		// separate commands, so no embedding client is built here.
		clients, err = composition.NewTranscriptionClients(cfg)
		if err != nil {
			return err
		}
	}

	// A real run opens read-write and applies migrations. A dry run opens the
	// planning database: the real file read-only when it exists, and an empty
	// migrated in-memory schema when it does not, which answers every lookup with
	// "not found" and writes nothing to disk.
	var sqlDB *sql.DB
	if *dryRun {
		sqlDB, err = composition.OpenPlanningDatabase(ctx, cfg)
	} else {
		sqlDB, err = composition.OpenDatabase(ctx, cfg)
	}
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	rag := composition.NewRAG(sqlDB)

	// A dry run must not create the storage tree either; it reads the local PDF
	// and the database only.
	var store *storage.Store
	if !*dryRun {
		store, err = storage.New(cfg.StorageRoot, cfg.MaxUploadBytes)
		if err != nil {
			return fmt.Errorf("open storage at %s: %w", cfg.StorageRoot, err)
		}
	}

	useCase := rag.Ingest(clients, store)

	report, runErr := useCase.Execute(ctx, ragapp.IngestRequest{
		PDFPath:   *pdfPath,
		Title:     filepath.Base(*pdfPath),
		SourceURI: *pdfPath,
		OutPath:   *outPath,
	}, ragapp.Options{
		DryRun:        *dryRun,
		Force:         *force,
		MaxModelPages: *maxModelPages,
	}, ragapp.Hooks{
		OnPlan: func(plan ragapp.Plan) { printPlan(stdout, plan) },
		OnPage: func(event ragapp.PageEvent) { printPageEvent(stdout, event) },
	})

	if runErr != nil {
		// A partial report is printed even when the run aborted, so the failure is
		// reported next to how far it got. A refusal at planning reached no job and
		// has nothing to summarize, and a dry run produced no pages.
		if report != nil && !*dryRun && report.JobID != "" {
			printSummary(stdout, report)
		}
		return describe(runErr)
	}

	if *dryRun {
		return nil
	}

	printSummary(stdout, report)

	if report.Extracted+report.Transcribed == 0 {
		return fmt.Errorf("no page produced text; the run converted nothing")
	}

	// The index is derived data, and the command rebuilds it from the run that just
	// settled. Execute stays unaware of the index: a conversion is worth keeping
	// even when indexing is not, and a failure here must not look like a failure to
	// convert.
	indexed, err := rag.Index().Execute(ctx, report.DocumentID)
	if err != nil {
		return fmt.Errorf("index document %s: %w", report.DocumentID, err)
	}
	fmt.Fprintf(stdout, "indexed %d chunks\n", indexed.Chunks)

	return nil
}

// printPlan writes the plan line before any model work starts.
func printPlan(stdout io.Writer, plan ragapp.Plan) {
	fmt.Fprintf(stdout, "plan  %d pages: %d already recorded, %d from the text layer, %d to the model (~%s)\n",
		plan.TotalPages, plan.AlreadyRecorded, plan.ToExtract, plan.ToModel, formatEstimate(plan.EstimatedVision))
}

// formatEstimate renders a vision-time estimate compactly: 1h22m, 4m, or 0s.
func formatEstimate(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}

	d = d.Round(time.Minute)
	if d < time.Minute {
		d = time.Minute
	}

	hours := d / time.Hour
	minutes := (d % time.Hour) / time.Minute
	if hours == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh%02dm", hours, minutes)
}

// printPageEvent writes one line per processed page.
func printPageEvent(stdout io.Writer, event ragapp.PageEvent) {
	line := fmt.Sprintf("page %d/%d  %-7s %s",
		event.Page, event.Total, event.Method, event.Elapsed.Round(100*time.Millisecond))
	if event.Err != "" {
		line += "  failed: " + event.Err
	} else {
		line += fmt.Sprintf("  %d chars", event.Chars)
	}

	fmt.Fprintln(stdout, line)
}

// printSummary reports the job's totals, which include pages an earlier run
// recorded.
func printSummary(stdout io.Writer, report *ragapp.Report) {
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "summary")
	fmt.Fprintf(stdout, "  document    %s\n", report.DocumentID)
	fmt.Fprintf(stdout, "  job         %s\n", report.JobID)
	fmt.Fprintf(stdout, "  pages       %d (%d extracted, %d transcribed, %d failed, %d already recorded)\n",
		report.TotalPages, report.Extracted, report.Transcribed, report.Failed, report.Skipped)
	fmt.Fprintf(stdout, "  settled     %t\n", report.Settled)
	fmt.Fprintf(stdout, "  output      %s\n", report.OutPath)
	fmt.Fprintf(stdout, "  duration    %s\n", report.Duration.Round(time.Second))
}

// describe adds the fix to a limit failure. The limit is configuration, so the
// message names the variable rather than only the number.
func describe(err error) error {
	if errors.Is(err, apperror.ErrTooLarge) {
		return fmt.Errorf("%w (raise %s to accept it)", err, config.EnvMaxUploadBytes)
	}
	return err
}
