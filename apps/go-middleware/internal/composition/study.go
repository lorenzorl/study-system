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

	// SetReadingProgress and ListReadingProgress are the coverage axis: where the
	// reader is inside a material, which the plan turns into the next action.
	SetReadingProgress  *studyapp.SetReadingProgressUseCase
	ListReadingProgress *studyapp.ListReadingProgressUseCase

	// SavePlan and GetPlan are the plan the agent produces: the app stores it and
	// projects it, and never classifies a book itself.
	SavePlan *studyapp.SavePlanUseCase
	GetPlan  *studyapp.GetPlanUseCase

	// Today is the projection over the three axes above plus the due count: what to
	// do today, read from rows the other routes already stored.
	Today *studyapp.TodayUseCase

	// GetExplanation and SetGapState make a stored explanation usable twice:
	// re-reading it costs nothing, and a gap can be dismissed on its own.
	GetExplanation *studyapp.GetExplanationUseCase
	SetGapState    *studyapp.SetGapStateUseCase

	// AcceptGap turns an accepted gap into a card. It is the only writer of `carded`,
	// because it is the only route that writes the card that state claims.
	AcceptGap *studyapp.AcceptGapUseCase
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
	gaps := studysqlite.NewRangeGapRepository(sqlDB)
	readingProgress := studysqlite.NewReadingProgressRepository(sqlDB)
	plans := studysqlite.NewStudyPlanRepository(sqlDB)

	// The plan names a material, so it needs the ingestion context's catalogue: the
	// adapter keeps study from importing rag, the same shape the page-text adapter
	// uses. Building the repository twice over one *sql.DB is free and stateless.
	documents := newDocumentTitles(NewRAG(sqlDB).DocumentRepo())

	// One flashcards use case, held here and passed to two callers: the sync route and
	// the accept route. A second instance would be a second owner of the derived
	// obsidian_id and of the initial card state, which is exactly what the accept route
	// must not have.
	syncFlashcards := studyapp.NewSyncFlashcardsUseCase(concepts, flashcards, cardStates)

	return &Study{
		SyncConcept:    studyapp.NewSyncConceptUseCase(topics, concepts),
		SyncFlashcards: syncFlashcards,
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
		GetExplanation: studyapp.NewGetExplanationUseCase(explanations),
		SetGapState:    studyapp.NewSetGapStateUseCase(gaps),
		AcceptGap: studyapp.NewAcceptGapUseCase(
			documents, gaps, explanations, plans, topics, concepts, syncFlashcards,
		),
		SetReadingProgress:  studyapp.NewSetReadingProgressUseCase(readingProgress),
		ListReadingProgress: studyapp.NewListReadingProgressUseCase(readingProgress),
		SavePlan:            studyapp.NewSavePlanUseCase(documents, plans, readingProgress),
		GetPlan:             studyapp.NewGetPlanUseCase(plans, readingProgress),
		Today: studyapp.NewTodayUseCase(
			flashcards, plans, readingProgress, explanations, documents,
		),
	}
}
