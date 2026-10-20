package cli

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/composition"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	ragapp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
)

// runList lists the stored artifacts. It reads the store and nothing else, so it
// keeps working with no API key, no database and no PDF.
func runList(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("list", "List the artifacts the store holds.", stderr)
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

	store, err := storage.New(cfg.StorageRoot, cfg.MaxUploadBytes)
	if err != nil {
		return fmt.Errorf("open storage at %s: %w", cfg.StorageRoot, err)
	}

	artifacts, err := store.List()
	if err != nil {
		return err
	}

	if len(artifacts) == 0 {
		fmt.Fprintf(stdout, "no artifacts in %s\n", store.Root())
		return nil
	}

	writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
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

// runSearch searches the chunk index. The query is positional, so a phrase does
// not need quoting to survive a flag parser: "search shortest path -limit 5".
func runSearch(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("search", "Search the chunk index. Needs no API key, no PDF and no storage.", stderr)
	limit := fs.Int("limit", ragapp.DefaultSearchLimit, "maximum number of results")
	if err := parseFlags(fs, args, stdout); err != nil {
		return err
	}

	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		fs.Usage()
		return errUsage
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Search reads the index and nothing else: read-only, no migrations, no
	// storage tree. It deliberately does not use the planning path, which falls
	// back to an empty schema and would answer every query with "0 results"
	// instead of reporting that the database it was told to read is not there.
	sqlDB, err := composition.OpenReadOnlyDatabase(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open database for search: %w", err)
	}
	defer sqlDB.Close()

	// The semantic stage is best-effort at the command level. With no key the
	// search stays lexical and reads only the database, which is what keeps it
	// working on a machine with no credential and no network. With a key, the use
	// case still falls back to lexical when the model has no stored vector.
	clients, err := composition.NewEmbeddingClients(cfg)
	if err != nil {
		return err
	}

	// A configured key without an embedding client must not downgrade this to a
	// lexical search: that still answers and looks correct, so the downgrade
	// would be invisible.
	if err := composition.RequireEmbedding(cfg, clients); err != nil {
		return err
	}

	searcher := composition.NewRAG(sqlDB).Search(clients, cfg.EmbeddingModel)
	return printResults(ctx, stdout, searcher, query, *limit)
}

// runReindex rebuilds the chunk index. It reads recorded page text and writes
// chunks, so it needs the database and nothing else.
func runReindex(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("reindex", "Rebuild the chunk index of every document with recorded pages.", stderr)
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

	sqlDB, err := composition.OpenDatabase(ctx, cfg)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	report, err := composition.NewRAG(sqlDB).Index().ReindexAll(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "reindexed %d documents, %d chunks\n", report.Documents, report.Chunks)
	return nil
}

// runEmbed rebuilds the embeddings. It is the only command that costs money and
// reaches the network, so it refuses to start without the key.
func runEmbed(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("embed", "Rebuild the embeddings of every document that has chunks.", stderr)
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

	if cfg.OpenRouterAPIKey == "" {
		return fmt.Errorf("%s is not set: embed calls the embedding model and cannot run without it",
			config.EnvOpenRouterAPIKey)
	}

	// Only the embedding client is built: this command never transcribes a page.
	clients, err := composition.NewEmbeddingClients(cfg)
	if err != nil {
		return err
	}

	// The database is opened read-write because this command writes vectors and
	// applies the migration that adds their table.
	sqlDB, err := composition.OpenDatabase(ctx, cfg)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	embedder, err := composition.NewRAG(sqlDB).Embed(clients, cfg.EmbeddingModel)
	if err != nil {
		return err
	}

	report, err := embedder.EmbedAll(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "embedded %d documents, %d vectors at %d dimensions with %s\n",
		report.Documents, report.Embeddings, report.Dimensions, report.Model)
	return nil
}

// printResults writes one ranked line per result, then the count.
func printResults(
	ctx context.Context,
	stdout io.Writer,
	searcher *ragapp.SearchUseCase,
	query string,
	limit int,
) error {
	results, err := searcher.Search(ctx, query, limit)
	if err != nil {
		return fmt.Errorf("search %q: %w", query, err)
	}

	if len(results) > 0 {
		writer := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
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
		fmt.Fprintln(stdout)
	}

	fmt.Fprintf(stdout, "%d results\n", len(results))
	return nil
}
