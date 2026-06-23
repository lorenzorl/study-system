package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/application"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/infrastructure/sqlite"
	httppkg "github.com/lorenzorangel/study-system/apps/go-middleware/internal/interfaces/http"
)

func main() {
	port := flag.String("port", "8080", "server port")
	flag.Parse()

	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("create data directory: %v", err)
	}

	db, err := sqlite.Open("file:data/study.db")
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	topicRepo := sqlite.NewTopicRepository(db)
	conceptRepo := sqlite.NewConceptRepository(db)
	flashcardRepo := sqlite.NewFlashcardRepository(db)
	resourceRepo := sqlite.NewResourceRepository(db)
	cardStateRepo := sqlite.NewCardStateRepository(db)
	reviewLogRepo := sqlite.NewReviewLogRepository(db)

	syncConceptUC := application.NewSyncConceptUseCase(topicRepo, conceptRepo)
	syncFlashcardsUC := application.NewSyncFlashcardsUseCase(conceptRepo, flashcardRepo, cardStateRepo)
	listConceptsUC := application.NewListConceptsUseCase(topicRepo, conceptRepo)
	createTopicUC := application.NewCreateTopicUseCase(topicRepo)
	createConceptUC := application.NewCreateConceptUseCase(topicRepo, conceptRepo)
	syncResourceUC := application.NewSyncResourceUseCase(topicRepo, resourceRepo)
	getDueCardsUC := application.NewGetDueCardsUseCase(flashcardRepo)
	dummyFSRS := domain.DummyFSRS{}
	submitReviewUC := application.NewSubmitReviewUseCase(cardStateRepo, reviewLogRepo, dummyFSRS)

	router := httppkg.NewRouter(syncConceptUC, syncFlashcardsUC, listConceptsUC, createTopicUC, createConceptUC, syncResourceUC, getDueCardsUC, submitReviewUC)

	server := &http.Server{
		Addr:    ":" + *port,
		Handler: router,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("server listening on :%s", *port)
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
