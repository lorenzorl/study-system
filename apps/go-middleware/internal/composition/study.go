package composition

import (
	"database/sql"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/llm"
	ragapp "github.com/lorenzorangel/study-system/apps/go-middleware/internal/rag/app"
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
	ListResources  *studyapp.ListResourcesUseCase
	GetDueCards    *studyapp.GetDueCardsUseCase
	SubmitReview   *studyapp.SubmitReviewUseCase
	ExplainRange   *studyapp.ExplainRangeUseCase
}

// NewStudy wires the study context over an open database.
//
// It builds the whole context, range explanations included. That use case is the
// one study feature whose ports live outside this context — the recorded page text
// belongs to the ingestion context and the model is a platform client — so this
// constructor builds the page text over the handle it was given and the gap finder
// as a named refusal, because a caller that names no model has none. The server,
// which has one, calls NewStudyWithRangeExplanations.
//
// Neither constructor leaves a field nil: a nil port stored behind a non-nil
// interface is the trap composition/clients.go documents.
func NewStudy(sqlDB *sql.DB) *Study {
	return NewStudyWithRangeExplanations(sqlDB, NewRAG(sqlDB).Pages(), nil)
}

// NewStudyWithRangeExplanations wires the study context with the two ports the
// range-explanation use case needs, so a process that holds them can answer
// POST /api/study/explanation without this context importing either one.
func NewStudyWithRangeExplanations(
	sqlDB *sql.DB,
	pages *ragapp.DocumentPagesUseCase,
	model *llm.Client,
) *Study {
	topics := studysqlite.NewTopicRepository(sqlDB)
	concepts := studysqlite.NewConceptRepository(sqlDB)
	flashcards := studysqlite.NewFlashcardRepository(sqlDB)
	resources := studysqlite.NewResourceRepository(sqlDB)
	cardStates := studysqlite.NewCardStateRepository(sqlDB)
	reviewLogs := studysqlite.NewReviewLogRepository(sqlDB)
	explanations := studysqlite.NewRangeExplanationRepository(sqlDB)

	return &Study{
		SyncConcept:    studyapp.NewSyncConceptUseCase(topics, concepts),
		SyncFlashcards: studyapp.NewSyncFlashcardsUseCase(concepts, flashcards, cardStates),
		ListConcepts:   studyapp.NewListConceptsUseCase(topics, concepts),
		CreateTopic:    studyapp.NewCreateTopicUseCase(topics),
		CreateConcept:  studyapp.NewCreateConceptUseCase(topics, concepts),
		SyncResource:   studyapp.NewSyncResourceUseCase(topics, resources),
		ListResources:  studyapp.NewListResourcesUseCase(resources),
		GetDueCards:    studyapp.NewGetDueCardsUseCase(flashcards),
		SubmitReview:   studyapp.NewSubmitReviewUseCase(cardStates, reviewLogs, domain.DummyFSRS{}),
		ExplainRange: studyapp.NewExplainRangeUseCase(
			newPageRangeText(pages),
			newModelGapFinder(model),
			explanations,
		),
	}
}
