// Package server runs the HTTP server: it mounts both bounded contexts, applies
// the platform policy, and shuts down cleanly.
//
// It is an entrypoint package, not part of composition. Composition builds the
// application objects; this package mounts them. That is why the routers, the
// CORS wrapper, and the processor's lifecycle live here and not there.
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/composition"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	ragapp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	raghttp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/http"
	studyhttp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/http"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/server/spa"
)

// Run serves until ctx is cancelled, then drains and returns.
//
// A listen failure is returned rather than fatal from inside a goroutine, so the
// caller decides what to do with it. Every message printed here is the one the
// server printed when this code lived in a main().
func Run(ctx context.Context, cfg *config.Config, port int) error {
	// The default database lives under the root. Creating the directory is the
	// server's job because the store creates its own root and the database file
	// does not create its parent.
	if err := os.MkdirAll(filepath.Join(cfg.Root, "data"), 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	if !cfg.RootIsTheWorkingDirectory() {
		log.Printf("data root: %s", cfg.Root)
	}

	// The composition root applies both contexts' migration blocks in one run and
	// wires each context over this one handle.
	sqlDB, err := composition.OpenDatabase(ctx, cfg)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	study := composition.NewStudy(sqlDB)
	rag := composition.NewRAG(sqlDB)

	// The store is built even without a key: it is where an upload would stream,
	// and building it touches only the filesystem, never the network.
	store, err := storage.New(cfg.StorageRoot, cfg.MaxUploadBytes)
	if err != nil {
		return fmt.Errorf("open storage at %s: %w", cfg.StorageRoot, err)
	}

	// The two model clients are created once and shared: the embedding client
	// serves both the semantic search and the embedding stage, so one credential
	// builds one client. Without a key neither exists, and the server still boots
	// and serves search and status.
	clients, err := composition.NewClients(cfg)
	if err != nil {
		return err
	}

	// A configured key without an embedding client would turn the hybrid search
	// into a lexical one that still answers. Refuse the quiet downgrade instead.
	if err := composition.RequireEmbedding(cfg, clients); err != nil {
		return err
	}

	// The processor runs ingestion, indexing, and embedding in sequence for an
	// uploaded document. It exists only with a key, because the upload endpoint
	// answers 503 without one and therefore never queues a document. Its context
	// is derived from the server's, so a cancelled server stops the workers, and
	// Wait drains them on shutdown.
	var processor *ragapp.BackgroundProcessor
	if clients.Embedding != nil {
		process, err := rag.Process(clients, cfg.EmbeddingModel, store)
		if err != nil {
			return err
		}

		processor = ragapp.NewBackgroundProcessor(ctx, ragapp.DefaultProcessWorkers, process)
	}

	// The HTTP surface adds no retrieval behaviour. Without a key the search is
	// lexical over the index the database already holds; the status read is built
	// either way, with a nil processor meaning "no in-process failure is known".
	searchUC := rag.Search(clients, cfg.EmbeddingModel)
	statusUC := rag.Status(processor, cfg.EmbeddingModel)

	// The upload handler refuses with 503 when the key is absent; the processor is
	// only passed when one exists, so the refusal path never reaches a nil
	// processor.
	var submitter raghttp.DocumentProcessor
	if processor != nil {
		submitter = processor
	}

	// --- mount the contexts, then apply platform policy ---
	//
	// Each context owns its routes and neither knows it shares the process with
	// the other. CORS stays the outermost wrapper, so no context encodes platform
	// policy.
	studyRoutes := studyhttp.NewRouter(
		study.SyncConcept,
		study.SyncFlashcards,
		study.ListConcepts,
		study.CreateTopic,
		study.CreateConcept,
		study.SyncResource,
		study.ListResources,
		study.GetDueCards,
		study.SubmitReview,
	)

	// The pasted-text path needs no key to accept material: it indexes always and
	// embeds only when a client exists (composition.TextIngest says why).
	textIngest, err := rag.TextIngest(clients, cfg.EmbeddingModel, store)
	if err != nil {
		return fmt.Errorf("build the pasted-text stage: %w", err)
	}

	ragRoutes := raghttp.NewRouter(
		searchUC,
		raghttp.NewUploadDocumentHandler(store, rag.DocumentRepo(), submitter, clients.Embedding != nil),
		raghttp.NewDocumentStatusHandler(statusUC),
		raghttp.NewDocumentsHandler(rag.ListDocuments()),
		raghttp.NewIngestTextHandler(textIngest, cfg.MaxUploadBytes),
	)

	rootMux := http.NewServeMux()
	rootMux.Handle("/api/rag/", ragRoutes)
	// The study context keeps its routes under `/api/`; the app itself is mounted
	// at the root and owns every other path, client routes included.
	rootMux.Handle("/api/", studyRoutes)
	rootMux.Handle("/", spa.New(spa.App))

	server := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: httpx.CORS(cfg.CORSAllowedOrigins)(rootMux),
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("server listening on :%d (cors origins: %d, web app embedded)", port, len(cfg.CORSAllowedOrigins))
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	// Drain the processor only after the server stops accepting requests, so no
	// new document can be queued behind the drain.
	if processor != nil {
		processor.Wait()
	}

	log.Println("server stopped")
	return nil
}
