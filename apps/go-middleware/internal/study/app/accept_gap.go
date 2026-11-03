package app

// Accepting a gap into a card: the reader writes the question, the book provides the
// evidence (S6), and the server decides the names (S10).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// CardWriter is the existing flashcards path behind a port.
//
// It is an interface and not the concrete use case so the accept route can be tested
// for what it derives, while the invariants a card has keep one owner: the derived
// obsidian_id and the initial card state belong to that path, and this route never
// re-implements them.
type CardWriter interface {
	Execute(ctx context.Context, conceptID string, cards []domain.Flashcard) (int, error)
}

// AcceptGapInput is one request to turn a gap into a card.
type AcceptGapInput struct {
	GapID string

	// Question is the reader's own question, in their words. It becomes the card's
	// front; the gap's quote and page become its back.
	Question string
}

// AcceptGapResult is what one accept wrote.
type AcceptGapResult struct {
	GapID     string
	Question  string
	Answer    string
	ConceptID string
	TopicID   string
	State     domain.RangeGapState
}

// AcceptGapUseCase turns an accepted gap into a flashcard.
//
// The naming rule is the server's, so a client never re-derives it: the topic is the
// document's title, the concept is the active plan item's title when the explained
// range is exactly that item's range, and `pp. <from>-<to>` otherwise. The concept's
// file_path is `ranges/<document_id>/<from>-<to>`, the stable key that makes a second
// accept reuse the concept instead of failing the unique constraint.
type AcceptGapUseCase struct {
	documents    Documents
	gaps         domain.RangeGapRepository
	explanations domain.RangeExplanationRepository
	plans        domain.StudyPlanRepository
	topics       domain.TopicRepository
	concepts     domain.ConceptRepository
	cards        CardWriter
}

// NewAcceptGapUseCase creates an AcceptGapUseCase.
func NewAcceptGapUseCase(
	documents Documents,
	gaps domain.RangeGapRepository,
	explanations domain.RangeExplanationRepository,
	plans domain.StudyPlanRepository,
	topics domain.TopicRepository,
	concepts domain.ConceptRepository,
	cards CardWriter,
) *AcceptGapUseCase {
	return &AcceptGapUseCase{
		documents:    documents,
		gaps:         gaps,
		explanations: explanations,
		plans:        plans,
		topics:       topics,
		concepts:     concepts,
		cards:        cards,
	}
}

// Execute writes the card and then claims the gap.
//
// The order below is the rule, not an accident of reading it top to bottom. The gap's
// state is a claim that a card exists, so the claim is written **last**: written
// first, it could be true of nothing. A failure anywhere before the last step leaves
// the gap `open` and stores no lie, and a retry converges because the card's identity
// is derived from the concept and the question, so writing it again updates one card
// instead of appending a second.
//
// Nothing is written for a refusal. A blank question, an unknown gap and a gap that
// already has a card all stop before the first write.
func (uc *AcceptGapUseCase) Execute(
	ctx context.Context,
	input AcceptGapInput,
) (AcceptGapResult, error) {
	gapID := strings.TrimSpace(input.GapID)
	if gapID == "" {
		return AcceptGapResult{}, fmt.Errorf("gap_id: %w", apperror.ErrInvalidInput)
	}

	// Step 1: the gap, as stored. A gap that already has a card is a conflict and not
	// a second card: the accept has happened once.
	gap, err := uc.gaps.FindByID(ctx, gapID)
	if err != nil {
		return AcceptGapResult{}, fmt.Errorf("find gap: %w", err)
	}

	if gap.State == domain.RangeGapStateCarded {
		return AcceptGapResult{}, fmt.Errorf("accept gap %q: %w", gapID, apperror.ErrConflict)
	}

	// Step 2: the explanation names the document and the range the card belongs to.
	// The page and the quote come from the gap, and the range from the explanation it
	// was found in; both are read, neither is accepted from the client.
	explanation, err := uc.explanations.FindByID(ctx, gap.ExplanationID)
	if err != nil {
		return AcceptGapResult{}, fmt.Errorf("find explanation %q: %w", gap.ExplanationID, err)
	}

	// Step 3: the reader writes the question. A card with a blank front is a card the
	// review queue can never resolve, so it is refused before anything is named.
	question := strings.TrimSpace(input.Question)
	if question == "" {
		return AcceptGapResult{}, fmt.Errorf("question is required: %w", apperror.ErrInvalidInput)
	}

	// Step 4: the topic is the document's title, found or created by name.
	title, err := uc.documents.Title(ctx, explanation.DocumentID)
	if err != nil {
		return AcceptGapResult{}, fmt.Errorf("look up document %q: %w", explanation.DocumentID, err)
	}

	topic, err := uc.topics.UpsertByName(ctx, title)
	if err != nil {
		return AcceptGapResult{}, fmt.Errorf("upsert topic %q: %w", title, err)
	}

	// Step 5: the concept, named by the plan item the range is, or by its pages.
	conceptTitle, err := uc.conceptTitle(
		ctx, explanation.DocumentID, explanation.FromPage, explanation.ToPage,
	)
	if err != nil {
		return AcceptGapResult{}, err
	}

	concept, err := uc.concepts.UpsertByPath(
		ctx,
		topic.ID,
		conceptTitle,
		conceptPath(explanation.DocumentID, explanation.FromPage, explanation.ToPage),
	)
	if err != nil {
		return AcceptGapResult{}, fmt.Errorf("upsert concept %q: %w", conceptTitle, err)
	}

	// Step 6: the card, through the existing flashcards path, so the derived
	// obsidian_id and the initial card state stay owned by one function.
	answer := fmt.Sprintf("%s (p.%d)", gap.Quote, gap.Page)

	if _, err := uc.cards.Execute(ctx, concept.ID, []domain.Flashcard{{
		Question: question,
		Answer:   answer,
	}}); err != nil {
		return AcceptGapResult{}, fmt.Errorf("write card for gap %q: %w", gapID, err)
	}

	// Step 7, last: the claim. Only now can the gap say it has a card.
	stored, err := uc.gaps.UpdateState(ctx, gapID, domain.RangeGapStateCarded)
	if err != nil {
		return AcceptGapResult{}, fmt.Errorf("mark gap %q carded: %w", gapID, err)
	}

	return AcceptGapResult{
		GapID:     stored.ID,
		Question:  question,
		Answer:    answer,
		ConceptID: concept.ID,
		TopicID:   topic.ID,
		State:     stored.State,
	}, nil
}

// conceptTitle names the concept the card lands in.
//
// The plan item's title when the explained range is exactly that item's range — the
// name the reader already reads in the plan — and `pp. <from>-<to>` otherwise. A
// missing plan is not a failure: a reader who explained a range without planning it
// still gets a card, named by its pages.
func (uc *AcceptGapUseCase) conceptTitle(
	ctx context.Context,
	documentID string,
	from, to int,
) (string, error) {
	plan, err := uc.plans.FindActiveByDocument(ctx, documentID)
	if err != nil && !errors.Is(err, apperror.ErrNotFound) {
		return "", fmt.Errorf("find active plan of %q: %w", documentID, err)
	}

	if err == nil {
		for _, item := range plan.Items {
			// The exact range, and nothing else: an overlapping item is a different
			// chapter, and naming this card after it would file the evidence under a
			// range the reader never explained.
			if item.FromPage == from && item.ToPage == to {
				return item.Title, nil
			}
		}
	}

	return fmt.Sprintf("pp. %d-%d", from, to), nil
}

// conceptPath is the concept's unique key, which is what makes a second accept of the
// same range reuse the concept: `concepts.file_path` is NOT NULL UNIQUE, so without a
// stable synthetic key the second accept would fail on the constraint.
func conceptPath(documentID string, from, to int) string {
	return fmt.Sprintf("ranges/%s/%d-%d", documentID, from, to)
}
