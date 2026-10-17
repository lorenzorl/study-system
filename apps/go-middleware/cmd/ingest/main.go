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
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/llm"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/domain"
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
	outPath := flag.String("out", "", "path of the Markdown file to write; required unless -list, -search, -reindex or -embed is used")
	list := flag.Bool("list", false, "list stored artifacts and exit")
	search := flag.String("search", "", "search the chunk index and exit; needs no API key, no PDF and no storage")
	limit := flag.Int("limit", app.DefaultSearchLimit, "maximum number of results for -search")
	reindex := flag.Bool("reindex", false, "rebuild the chunk index of every document with recorded pages, and exit")
	embed := flag.Bool("embed", false, "rebuild the embeddings of every document that has chunks, and exit; needs the API key and the network")
	dryRun := flag.Bool("dry-run", false, "print the plan and stop before any model work or write")
	maxModelPages := flag.Int("max-model-pages", 0, "refuse the run when the plan would send more pages to the model than this (0 = no limit)")
	force := flag.Bool("force", false, "replace the output file even when this pipeline did not generate it")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	// Listing reads the store and nothing else. It must keep working with no API
	// key, no database, and no PDF, so it returns before any of them is opened.
	if *list {
		store, err := storage.New(cfg.StorageRoot, cfg.MaxUploadBytes)
		if err != nil {
			return fmt.Errorf("open storage at %s: %w", cfg.StorageRoot, err)
		}
		return listArtifacts(store)
	}

	// Search reads the index and nothing else. It opens the database read-only,
	// without migrations and without the storage tree, and returns before the API
	// key and the PDF are touched, so it works on a machine that has none of them.
	//
	// It deliberately does not go through openDatabase's planning path: that path
	// falls back to an empty in-memory schema when the file cannot be opened, and
	// for a search that fallback would answer every query with "0 results" instead
	// of reporting that the database it was told to read is not there.
	if *search != "" {
		ctx := context.Background()

		sqlDB, err := db.Open(ctx, readOnlyDSN(cfg.DatabaseDSN))
		if err != nil {
			return fmt.Errorf("open database for search: %w", err)
		}
		defer sqlDB.Close()

		chunks := ragsqlite.NewChunkRepository(sqlDB)

		// The semantic stage is best-effort at the command level. With no key the
		// search stays lexical and reads only the database, which is what keeps it
		// working on a machine with no credential and no network. With a key, the
		// use case still falls back to lexical when the model has no stored vector.
		var searcher *app.SearchUseCase
		if cfg.OpenRouterAPIKey == "" {
			searcher = app.NewSearchUseCase(chunks)
		} else {
			client, err := llm.NewEmbeddingClient(
				cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, cfg.EmbeddingModel, llm.Options{})
			if err != nil {
				return fmt.Errorf("create embedding client: %w", err)
			}

			searcher = app.NewHybridSearchUseCase(
				chunks, ragsqlite.NewEmbeddingRepository(sqlDB), client, cfg.EmbeddingModel)
		}

		return runSearch(ctx, searcher, *search, *limit)
	}

	// A reindex reads recorded page text and writes chunks, so it needs the
	// database and nothing else: no API key, no PDF, no storage.
	if *reindex {
		ctx := context.Background()

		sqlDB, err := openDatabase(ctx, cfg, false)
		if err != nil {
			return err
		}
		defer sqlDB.Close()

		indexer := app.NewIndexDocumentUseCase(
			ragsqlite.NewDocumentRepository(sqlDB),
			ragsqlite.NewJobRepository(sqlDB),
			ragsqlite.NewPageOutcomeRepository(sqlDB),
			ragsqlite.NewChunkRepository(sqlDB),
		)
		return runReindex(ctx, indexer)
	}

	// Embedding is the only step that costs money and reaches the network, so it
	// is separate from the reindex and refuses to start without the key. The
	// database is opened read-write because this command writes vectors and
	// applies the migration that adds their table.
	if *embed {
		if cfg.OpenRouterAPIKey == "" {
			return fmt.Errorf("%s is not set: -embed calls the embedding model and cannot run without it",
				config.EnvOpenRouterAPIKey)
		}

		client, err := llm.NewEmbeddingClient(
			cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, cfg.EmbeddingModel, llm.Options{})
		if err != nil {
			return fmt.Errorf("create embedding client: %w", err)
		}

		ctx := context.Background()

		sqlDB, err := openDatabase(ctx, cfg, false)
		if err != nil {
			return err
		}
		defer sqlDB.Close()

		embedder := app.NewEmbedDocumentUseCase(
			ragsqlite.NewDocumentRepository(sqlDB),
			ragsqlite.NewChunkRepository(sqlDB),
			ragsqlite.NewEmbeddingRepository(sqlDB),
			client,
			cfg.EmbeddingModel,
		)
		return runEmbed(ctx, embedder)
	}

	if *pdfPath == "" {
		flag.Usage()
		return errors.New("-pdf is required unless -list, -search, -reindex or -embed is used")
	}
	if *outPath == "" {
		flag.Usage()
		return errors.New("-out is required: the backend never guesses the vault path")
	}

	// The key is checked before the database is opened and before the first page
	// is read, so a missing key cannot cost hundreds of pages of work. A dry run
	// is different: it plans without ever calling the model, so it needs no key,
	// and a text-only book needs none either.
	var transcriber domain.PageTranscriber
	if !*dryRun {
		if cfg.OpenRouterAPIKey == "" {
			return fmt.Errorf("%s is not set: the ingestion run cannot transcribe pages without it",
				config.EnvOpenRouterAPIKey)
		}

		client, err := llm.NewClient(cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, cfg.LLMModel, llm.Options{})
		if err != nil {
			return fmt.Errorf("create model client: %w", err)
		}
		transcriber = client
	}

	ctx := context.Background()

	sqlDB, err := openDatabase(ctx, cfg, *dryRun)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	documents := ragsqlite.NewDocumentRepository(sqlDB)
	jobs := ragsqlite.NewJobRepository(sqlDB)
	pages := ragsqlite.NewPageOutcomeRepository(sqlDB)
	chunks := ragsqlite.NewChunkRepository(sqlDB)

	// A dry run must not create the storage tree either; it reads the local PDF
	// and the database only.
	var store *storage.Store
	if !*dryRun {
		store, err = storage.New(cfg.StorageRoot, cfg.MaxUploadBytes)
		if err != nil {
			return fmt.Errorf("open storage at %s: %w", cfg.StorageRoot, err)
		}
	}

	useCase := app.NewIngestDocumentUseCase(
		documents,
		jobs,
		pages,
		// A nil runner selects the real poppler binaries, so a missing tool fails
		// at Open rather than at the page that first needs it.
		pdf.NewSource(nil),
		transcriber,
		store,
	)

	report, runErr := useCase.Execute(ctx, app.IngestRequest{
		PDFPath:   *pdfPath,
		Title:     filepath.Base(*pdfPath),
		SourceURI: *pdfPath,
		OutPath:   *outPath,
	}, app.Options{
		DryRun:        *dryRun,
		Force:         *force,
		MaxModelPages: *maxModelPages,
	}, app.Hooks{
		OnPlan: printPlan,
		OnPage: printPageEvent,
	})

	if runErr != nil {
		// A partial report is printed even when the run aborted, so the failure
		// is reported next to how far it got. A refusal at planning reached no
		// job and has nothing to summarize, and a dry run produced no pages.
		if report != nil && !*dryRun && report.JobID != "" {
			printSummary(report)
		}
		return describe(runErr)
	}

	if *dryRun {
		return nil
	}

	printSummary(report)

	if report.Extracted+report.Transcribed == 0 {
		return fmt.Errorf("no page produced text; the run converted nothing")
	}

	// The index is derived data, and the command rebuilds it from the run that
	// just settled. Execute stays unaware of the index: a conversion is worth
	// keeping even when indexing is not, and a failure here must not look like a
	// failure to convert.
	indexed, err := app.NewIndexDocumentUseCase(documents, jobs, pages, chunks).Execute(ctx, report.DocumentID)
	if err != nil {
		return fmt.Errorf("index document %s: %w", report.DocumentID, err)
	}
	fmt.Printf("indexed %d chunks\n", indexed.Chunks)

	return nil
}

// runSearch prints one ranked line per result, then the count.
func runSearch(ctx context.Context, searcher *app.SearchUseCase, query string, limit int) error {
	results, err := searcher.Search(ctx, query, limit)
	if err != nil {
		return fmt.Errorf("search %q: %w", query, err)
	}

	if len(results) > 0 {
		writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(writer, "#\tTITLE\tPAGE\tMATCH\tBM25\tCOSINE\tSNIPPET")

		for i, result := range results {
			// bm25 is smaller-is-better, so its magnitude is printed under a name
			// that says what it means: a bigger BM25 is a better lexical match.
			// The cosine is already larger-is-better. A stage that did not rank the
			// chunk prints 0 for its score, and MATCH names the stage that did.
			// math.Abs keeps that 0 positive: negating a zero score would print
			// "-0.000", which reads as a score rather than as "not ranked here".
			fmt.Fprintf(writer, "%d\t%s\t%d\t%s\t%.3f\t%.3f\t%s\n",
				i+1, result.DocumentTitle, result.PageNumber, result.Stage,
				math.Abs(result.Score), result.Similarity, result.Snippet)
		}
		if err := writer.Flush(); err != nil {
			return err
		}
		fmt.Println()
	}

	fmt.Printf("%d results\n", len(results))
	return nil
}

// runEmbed rebuilds every document's vectors and reports the total, so a run that
// costs money is observable rather than silent.
func runEmbed(ctx context.Context, embedder *app.EmbedDocumentUseCase) error {
	report, err := embedder.EmbedAll(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("embedded %d documents, %d vectors at %d dimensions with %s\n",
		report.Documents, report.Embeddings, report.Dimensions, report.Model)
	return nil
}

// runReindex rebuilds every document that has recorded pages and reports the
// total, so a rebuild is observable rather than silent.
func runReindex(ctx context.Context, indexer *app.IndexDocumentUseCase) error {
	report, err := indexer.ReindexAll(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("reindexed %d documents, %d chunks\n", report.Documents, report.Chunks)
	return nil
}

// openDatabase opens the planning or the working database.
//
// A real run opens read-write and applies migrations. A dry run must not write
// to the database: it opens read-only and skips migrations, and when there is no
// file to open read-only it uses an empty in-memory schema, which answers every
// lookup with "not found" exactly as a database that has never seen the bytes
// does. That fallback writes nothing to disk.
func openDatabase(ctx context.Context, cfg *config.Config, dryRun bool) (*sql.DB, error) {
	migrations := slices.Concat(studysqlite.Migrations(), ragsqlite.Migrations())

	if dryRun {
		if sqlDB, err := db.Open(ctx, readOnlyDSN(cfg.DatabaseDSN)); err == nil {
			return sqlDB, nil
		}

		sqlDB, err := db.Open(ctx, ":memory:")
		if err != nil {
			return nil, fmt.Errorf("open planning database: %w", err)
		}
		if err := db.Migrate(ctx, sqlDB, migrations); err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("prepare planning database: %w", err)
		}
		return sqlDB, nil
	}

	sqlDB, err := db.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.Migrate(ctx, sqlDB, migrations); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return sqlDB, nil
}

// readOnlyDSN appends SQLite's read-only mode to a file DSN, preserving any
// query string it already carries. An in-memory DSN is returned unchanged.
func readOnlyDSN(dsn string) string {
	if strings.Contains(dsn, ":memory:") {
		return dsn
	}
	if strings.Contains(dsn, "?") {
		return dsn + "&mode=ro"
	}
	return dsn + "?mode=ro"
}

// printPlan writes the plan line before any model work starts.
func printPlan(plan app.Plan) {
	fmt.Printf("plan  %d pages: %d already recorded, %d from the text layer, %d to the model (~%s)\n",
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
