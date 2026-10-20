package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/composition"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	ragapp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	raghttp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/http"
	studyhttp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/http"
)

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

	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("create data directory: %v", err)
	}

	// The composition root is internal/composition: it applies both contexts'
	// migration blocks in one run and wires each context over this one handle.
	sqlDB, err := composition.OpenDatabase(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()

	study := composition.NewStudy(sqlDB)
	rag := composition.NewRAG(sqlDB)

	// The store is built even without a key: it is where an upload would stream,
	// and building it touches only the filesystem, never the network.
	store, err := storage.New(cfg.StorageRoot, cfg.MaxUploadBytes)
	if err != nil {
		log.Fatalf("open storage at %s: %v", cfg.StorageRoot, err)
	}

	// The two model clients are created once and shared: the embedding client
	// serves both the semantic search and the embedding stage, so one credential
	// builds one client. Without a key neither exists, and the server still boots
	// and serves search and status (S5).
	clients, err := composition.NewClients(cfg)
	if err != nil {
		log.Fatal(err)
	}

	// The processor runs ingestion, indexing, and embedding in sequence for an
	// uploaded document. It exists only with a key, because the upload endpoint
	// answers 503 without one and therefore never queues a document. Its context
	// is derived from the server's, so a cancelled server stops the workers, and
	// Wait drains them on shutdown.
	var processor *ragapp.BackgroundProcessor
	if clients.Embedding != nil {
		processor = ragapp.NewBackgroundProcessor(
			ctx, ragapp.DefaultProcessWorkers, rag.Process(clients, cfg.EmbeddingModel, store))
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

	// --- composition root: mount the contexts, then apply platform policy ---
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
		study.GetDueCards,
		study.SubmitReview,
	)

	ragRoutes := raghttp.NewRouter(
		searchUC,
		raghttp.NewUploadDocumentHandler(store, rag.DocumentRepo(), submitter, clients.Embedding != nil),
		raghttp.NewDocumentStatusHandler(statusUC),
		raghttp.NewDocumentsHandler(rag.ListDocuments()),
	)

	rootMux := http.NewServeMux()
	rootMux.Handle("/api/rag/", ragRoutes)
	rootMux.Handle("/", studyRoutes)

	server := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: httpx.CORS(cfg.CORSAllowedOrigins)(rootMux),
	}

	go func() {
		log.Printf("server listening on :%d (cors origins: %d)", port, len(cfg.CORSAllowedOrigins))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown error: %v", err)
	}

	// Drain the processor only after the server stops accepting requests, so no
	// new document can be queued behind the drain.
	if processor != nil {
		processor.Wait()
	}

	log.Println("server stopped")
}
