package composition

import (
	"database/sql"

	studyapp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/app"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
	studysqlite "github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/sqlite"
)

// Study is the study context, wired.
//
// The use cases are exported as fields rather than behind methods because they
// are the context's application surface: a transport mounts them, and a command
// calls them. There is nothing to decide at construction time, so there is no
// reason to defer building them.
type Study struct {
	SyncConcept    *studyapp.SyncConceptUseCase
	SyncFlashcards *studyapp.SyncFlashcardsUseCase
	ListConcepts   *studyapp.ListConceptsUseCase
	CreateTopic    *studyapp.CreateTopicUseCase
	CreateConcept  *studyapp.CreateConceptUseCase
	SyncResource   *studyapp.SyncResourceUseCase
	GetDueCards    *studyapp.GetDueCardsUseCase
	SubmitReview   *studyapp.SubmitReviewUseCase
}

// NewStudy wires the study context over an open database.
func NewStudy(sqlDB *sql.DB) *Study {
	topics := studysqlite.NewTopicRepository(sqlDB)
	concepts := studysqlite.NewConceptRepository(sqlDB)
	flashcards := studysqlite.NewFlashcardRepository(sqlDB)
	resources := studysqlite.NewResourceRepository(sqlDB)
	cardStates := studysqlite.NewCardStateRepository(sqlDB)
	reviewLogs := studysqlite.NewReviewLogRepository(sqlDB)

	return &Study{
		SyncConcept:    studyapp.NewSyncConceptUseCase(topics, concepts),
		SyncFlashcards: studyapp.NewSyncFlashcardsUseCase(concepts, flashcards, cardStates),
		ListConcepts:   studyapp.NewListConceptsUseCase(topics, concepts),
		CreateTopic:    studyapp.NewCreateTopicUseCase(topics),
		CreateConcept:  studyapp.NewCreateConceptUseCase(topics, concepts),
		SyncResource:   studyapp.NewSyncResourceUseCase(topics, resources),
		GetDueCards:    studyapp.NewGetDueCardsUseCase(flashcards),
		SubmitReview:   studyapp.NewSubmitReviewUseCase(cardStates, reviewLogs, domain.DummyFSRS{}),
	}
}
