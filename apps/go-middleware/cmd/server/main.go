package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/config"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/db"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/httpx"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/llm"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/storage"
	ragapp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	raghttp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/http"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/infra/pdf"
	ragsqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/sqlite"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	studyhttp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/http"
	studysqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
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

	sqlDB, err := db.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer sqlDB.Close()

	// Each context owns its own migration list and its own version block; see the
	// "Schema migrations" section of the README. The composition root is what puts
	// them in one database, in one run.
	migrations := slices.Concat(studysqlite.Migrations(), ragsqlite.Migrations())
	if err := db.Migrate(ctx, sqlDB, migrations); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	// --- study context: repositories ---
	topicRepo := studysqlite.NewTopicRepository(sqlDB)
	conceptRepo := studysqlite.NewConceptRepository(sqlDB)
	flashcardRepo := studysqlite.NewFlashcardRepository(sqlDB)
	resourceRepo := studysqlite.NewResourceRepository(sqlDB)
	cardStateRepo := studysqlite.NewCardStateRepository(sqlDB)
	reviewLogRepo := studysqlite.NewReviewLogRepository(sqlDB)

	// --- study context: use cases ---
	syncConceptUC := app.NewSyncConceptUseCase(topicRepo, conceptRepo)
	syncFlashcardsUC := app.NewSyncFlashcardsUseCase(conceptRepo, flashcardRepo, cardStateRepo)
	listConceptsUC := app.NewListConceptsUseCase(topicRepo, conceptRepo)
	createTopicUC := app.NewCreateTopicUseCase(topicRepo)
	createConceptUC := app.NewCreateConceptUseCase(topicRepo, conceptRepo)
	syncResourceUC := app.NewSyncResourceUseCase(topicRepo, resourceRepo)
	getDueCardsUC := app.NewGetDueCardsUseCase(flashcardRepo)
	submitReviewUC := app.NewSubmitReviewUseCase(cardStateRepo, reviewLogRepo, domain.DummyFSRS{})

	// --- composition root: mount the context, then apply platform policy ---
	studyRoutes := studyhttp.NewRouter(
		syncConceptUC,
		syncFlashcardsUC,
		listConceptsUC,
		createTopicUC,
		createConceptUC,
		syncResourceUC,
		getDueCardsUC,
		submitReviewUC,
	)

	// --- rag context: repositories, storage, and the two model clients ---
	//
	// The repositories are shared by retrieval, the upload/status endpoints, and
	// the pipeline. The store is built even without a key: it is where an upload
	// would stream, and building it touches only the filesystem, never the
	// network.
	store, err := storage.New(cfg.StorageRoot, cfg.MaxUploadBytes)
	if err != nil {
		log.Fatalf("open storage at %s: %v", cfg.StorageRoot, err)
	}

	documents := ragsqlite.NewDocumentRepository(sqlDB)
	jobs := ragsqlite.NewJobRepository(sqlDB)
	pages := ragsqlite.NewPageOutcomeRepository(sqlDB)
	chunks := ragsqlite.NewChunkRepository(sqlDB)
	embeddings := ragsqlite.NewEmbeddingRepository(sqlDB)

	// The two model clients are created once and shared: the embedding client
	// serves both the semantic search and the embedding stage, so one credential
	// builds one client. Without a key neither exists, and the server still boots
	// and serves search and status (S5).
	var (
		embeddingClient *llm.EmbeddingClient
		modelClient     *llm.Client
	)
	if cfg.OpenRouterAPIKey != "" {
		embeddingClient, err = llm.NewEmbeddingClient(
			cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, cfg.EmbeddingModel, llm.Options{})
		if err != nil {
			log.Fatalf("create embedding client: %v", err)
		}
		modelClient, err = llm.NewClient(
			cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, cfg.LLMModel, llm.Options{})
		if err != nil {
			log.Fatalf("create model client: %v", err)
		}
	}

	// --- rag context: retrieval over the chunk index ---
	//
	// The HTTP surface wraps the search use case and adds no retrieval behaviour.
	// The lexical stage needs only the database. The semantic stage is upgraded in
	// when the key is set, and the use case still falls back to lexical-only when
	// the configured model has no stored vectors, so the key stays optional.
	var searchUC *ragapp.SearchUseCase
	if embeddingClient == nil {
		searchUC = ragapp.NewSearchUseCase(chunks)
	} else {
		searchUC = ragapp.NewHybridSearchUseCase(chunks, embeddings, embeddingClient, cfg.EmbeddingModel)
	}

	// --- rag context: the document pipeline and its status ---
	//
	// The processor runs ingestion, indexing, and embedding in sequence for an
	// uploaded document. It is built only with a key, because the upload endpoint
	// answers 503 without one and therefore never queues a document. Its context
	// is derived from the server's, so a cancelled server stops the workers, and
	// Wait drains them on shutdown.
	var processor *ragapp.BackgroundProcessor
	var statusUC *ragapp.DocumentStatusUseCase
	if embeddingClient == nil {
		statusUC = ragapp.NewDocumentStatusUseCase(
			documents, jobs, pages, chunks, embeddings, nil, cfg.EmbeddingModel)
	} else {
		ingestUC := ragapp.NewIngestDocumentUseCase(
			documents, jobs, pages, pdf.NewSource(nil), modelClient, store)
		indexUC := ragapp.NewIndexDocumentUseCase(documents, jobs, pages, chunks)
		embedUC := ragapp.NewEmbedDocumentUseCase(documents, chunks, embeddings, embeddingClient, cfg.EmbeddingModel)
		processUC := ragapp.NewProcessDocumentUseCase(documents, store, ingestUC, indexUC, embedUC)

		processor = ragapp.NewBackgroundProcessor(ctx, ragapp.DefaultProcessWorkers, processUC)
		statusUC = ragapp.NewDocumentStatusUseCase(
			documents, jobs, pages, chunks, embeddings, processor, cfg.EmbeddingModel)
	}

	// The upload handler refuses with 503 when the key is absent; the processor is
	// only passed when one exists, so the refusal path never reaches a nil
	// processor.
	var submitter raghttp.DocumentProcessor
	if processor != nil {
		submitter = processor
	}
	ragRoutes := raghttp.NewRouter(
		searchUC,
		raghttp.NewUploadDocumentHandler(store, documents, submitter, cfg.OpenRouterAPIKey != ""),
		raghttp.NewDocumentStatusHandler(statusUC),
		raghttp.NewDocumentsHandler(ragapp.NewListDocumentsUseCase(documents, pages, chunks)),
	)

	// The RAG router is mounted under its prefix on a root mux, and the study
	// router catches the rest. Each context is unchanged: neither knows it shares
	// the process with the other, and CORS stays the outermost wrapper.
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
