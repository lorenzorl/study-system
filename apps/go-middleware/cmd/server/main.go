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
	ragapp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
	raghttp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/http"
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

	// --- rag context: retrieval over the chunk index ---
	//
	// The HTTP surface wraps the search use case and adds no retrieval behaviour.
	// The lexical stage needs only the database. The semantic stage is upgraded in
	// when the key is set, and the use case still falls back to lexical-only when
	// the configured model has no stored vectors, so the key stays optional: the
	// server boots and answers without it.
	chunks := ragsqlite.NewChunkRepository(sqlDB)

	var searchUC *ragapp.SearchUseCase
	if cfg.OpenRouterAPIKey == "" {
		searchUC = ragapp.NewSearchUseCase(chunks)
	} else {
		client, err := llm.NewEmbeddingClient(
			cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, cfg.EmbeddingModel, llm.Options{})
		if err != nil {
			log.Fatalf("create embedding client: %v", err)
		}
		searchUC = ragapp.NewHybridSearchUseCase(
			chunks, ragsqlite.NewEmbeddingRepository(sqlDB), client, cfg.EmbeddingModel)
	}

	// The RAG router is mounted under its prefix on a root mux, and the study
	// router catches the rest. Each context is unchanged: neither knows it shares
	// the process with the other, and CORS stays the outermost wrapper.
	rootMux := http.NewServeMux()
	rootMux.Handle("/api/rag/", raghttp.NewRouter(searchUC))
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

	log.Println("server stopped")
}
