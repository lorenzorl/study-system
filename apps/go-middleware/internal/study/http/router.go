package http

import (
	"net/http"
	"reflect"
)

// UseCases is the study context's application surface, as the transport needs it.
//
// One struct and not a parameter list: the context has fifteen use cases, and a
// positional call with fifteen arguments is a place where two of them silently
// swap and the compiler does not notice. Named fields make the wiring at the
// composition root read as what it is.
type UseCases struct {
	SyncConcept         SyncConceptUseCase
	SyncFlashcards      SyncFlashcardsUseCase
	ListConcepts        ListConceptsUseCase
	CreateTopic         CreateTopicUseCase
	CreateConcept       CreateConceptUseCase
	SyncResource        SyncResourceUseCase
	ListResources       ListResourcesUseCase
	GetDueCards         GetDueCardsUseCase
	SubmitReview        SubmitReviewUseCase
	ExplainRange        ExplainRangeUseCase
	SetReadingProgress  SetReadingProgressUseCase
	ListReadingProgress ListReadingProgressUseCase
	SavePlan            SavePlanUseCase
	GetPlan             GetPlanUseCase
	Today               TodayUseCase
	GetExplanation      GetExplanationUseCase
	SetGapState         SetGapStateUseCase
	AcceptGap           AcceptGapUseCase
}

// NewRouter builds the study context's HTTP handler.
//
// It accepts the use case implementations, which cmd/server wires together. The
// context owns its own routes; a future top-level composition point can mount
// this alongside other contexts.
//
// Cross-cutting transport concerns such as CORS are deliberately NOT applied
// here. The composition root wraps this handler, so a context never encodes
// platform policy.
func NewRouter(useCases UseCases) http.Handler {
	// Before a single route is registered: a use case the composition root forgot is a
	// boot failure that names the field, not a handler holding a nil interface that
	// panics on the first reader who reaches that route.
	requireWired(useCases)

	mux := http.NewServeMux()

	mux.Handle("POST /api/sync/concept", NewSyncConceptHandler(useCases.SyncConcept))
	mux.Handle("POST /api/sync/flashcards", NewSyncFlashcardsHandler(useCases.SyncFlashcards))
	mux.Handle("GET /api/concepts", NewListConceptsHandler(useCases.ListConcepts))
	mux.Handle("POST /api/topics", NewCreateTopicHandler(useCases.CreateTopic))
	mux.Handle("POST /api/concepts", NewCreateConceptHandler(useCases.CreateConcept))
	mux.Handle("POST /api/sync/resource", NewSyncResourceHandler(useCases.SyncResource))
	mux.Handle("GET /api/resources", NewListResourcesHandler(useCases.ListResources))
	mux.Handle("GET /api/study/due", NewDueCardsHandler(useCases.GetDueCards))
	mux.Handle("POST /api/study/review", NewReviewHandler(useCases.SubmitReview))
	mux.Handle("POST /api/study/explanation", NewExplanationHandler(useCases.ExplainRange))

	// The explanation is stored, so it is also readable, and its gaps are actionable
	// one by one. Without these two, opening a page again would pay for the model and
	// a reader could not dismiss a gap they disagreed with.
	mux.Handle("GET /api/study/explanations/{explanationID}", NewExplanationReadHandler(useCases.GetExplanation))
	mux.Handle("PUT /api/study/gaps/{gapID}/state", NewGapStateHandler(useCases.SetGapState))

	// Accepting a gap is its own route and not a third state: it writes the card, so it
	// is the only writer of `carded`, and a state that says a card exists can only come
	// from the request that stored one.
	mux.Handle("POST /api/study/gaps/{gapID}/card", NewAcceptGapHandler(useCases.AcceptGap))

	// The coverage axis. It sits under /api/study/ beside the retrieval routes and
	// shares their mux: marking a range read is a study action, not a document
	// edit, which is why it is not under /api/rag/.
	mux.Handle("PUT /api/study/reading", NewSetReadingProgressHandler(useCases.SetReadingProgress))
	mux.Handle("GET /api/study/reading", NewListReadingProgressHandler(useCases.ListReadingProgress))

	// The plan: the agent produces it, the app stores and projects it. One route to
	// write it and one to read the active plan of a document; there is no route to
	// edit an item, because a plan is partial and disposable and a new POST replaces
	// it instead.
	mux.Handle("POST /api/study/plan", NewSavePlanHandler(useCases.SavePlan))
	mux.Handle("GET /api/study/plan", NewGetPlanHandler(useCases.GetPlan))

	// The projection that answers what to do today. It is one read over the three
	// routes above and the due query: it stores nothing, and adding it changed no
	// shape any of them answers with.
	mux.Handle("GET /api/study/today", NewTodayHandler(useCases.Today))

	return mux
}

// requireWired panics when any use case of the context is missing.
//
// The check walks the struct instead of listing the fields again: a hand-written list
// is one more place to forget, and forgetting is exactly the defect this prevents —
// two routes shipped holding nil use cases because nothing compared UseCases with the
// struct literal that filled it.
//
// A nil here is a composition error, not a request error. A panic at construction is a
// bug report with the field's name in it; the same nil discovered by a request is an
// outage.
func requireWired(useCases UseCases) {
	value := reflect.ValueOf(useCases)
	typeOf := value.Type()

	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		// Only interfaces can hold nil; a value field is always wired.
		if field.Kind() != reflect.Interface || !field.IsNil() {
			continue
		}

		panic("study http: use case " + typeOf.Field(index).Name + " is not wired")
	}
}
