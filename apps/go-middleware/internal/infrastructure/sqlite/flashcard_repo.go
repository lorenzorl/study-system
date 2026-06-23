package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// FlashcardRepository implements domain.FlashcardRepository using SQLite.
type FlashcardRepository struct {
	db *sql.DB
}

// NewFlashcardRepository creates a new FlashcardRepository backed by the given database.
func NewFlashcardRepository(db *sql.DB) *FlashcardRepository {
	return &FlashcardRepository{db: db}
}

// FindByObsidianID returns a flashcard by its ObsidianID, or nil,ErrNotFound if missing.
func (r *FlashcardRepository) FindByObsidianID(ctx context.Context, obsidianID string) (*domain.Flashcard, error) {
	var card domain.Flashcard
	var createdAt string

	err := r.db.QueryRowContext(ctx,
		`SELECT id, concept_id, question, answer, obsidian_id, created_at
		 FROM flashcards WHERE obsidian_id = ?`,
		obsidianID,
	).Scan(&card.ID, &card.ConceptID, &card.Question, &card.Answer, &card.ObsidianID, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("query flashcard by obsidian_id: %w", err)
	}

	card.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &card, nil
}

// FindDueWithContext returns all flashcards whose CardState.NextReview <= now,
// joined with concept and topic context for display.
func (r *FlashcardRepository) FindDueWithContext(ctx context.Context, now time.Time) ([]domain.DueCardResult, error) {
	query := `
		SELECT f.id, f.question, f.answer, c.title, t.name, cs.next_review
		FROM flashcards f
		JOIN card_states cs ON cs.flashcard_id = f.id
		JOIN concepts c ON c.id = f.concept_id
		JOIN topics t ON t.id = c.topic_id
		WHERE cs.next_review <= ?
		ORDER BY cs.next_review ASC`

	rows, err := r.db.QueryContext(ctx, query, now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("query due cards: %w", err)
	}
	defer rows.Close()

	var results []domain.DueCardResult
	for rows.Next() {
		var r domain.DueCardResult
		var nextReviewStr string
		if err := rows.Scan(&r.FlashcardID, &r.Front, &r.Back, &r.ConceptTitle, &r.TopicName, &nextReviewStr); err != nil {
			return nil, fmt.Errorf("scan due card: %w", err)
		}
		r.NextReview, _ = time.Parse(time.RFC3339, nextReviewStr)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due cards: %w", err)
	}

	if results == nil {
		results = []domain.DueCardResult{}
	}
	return results, nil
}

// UpsertByObsidianID upserts flashcards by their ObsidianID within a concept.
// Uses INSERT ... ON CONFLICT(obsidian_id) DO UPDATE for idempotent upsert.
func (r *FlashcardRepository) UpsertByObsidianID(ctx context.Context, conceptID string, cards []domain.Flashcard) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `INSERT INTO flashcards (id, concept_id, question, answer, obsidian_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(obsidian_id) DO UPDATE SET
			question = excluded.question,
			answer = excluded.answer`

	processed := 0
	for _, card := range cards {
		cardID := uuid.New().String()
		now := time.Now().UTC().Format(time.RFC3339)

		_, err := tx.ExecContext(ctx, query,
			cardID, conceptID, card.Question, card.Answer, card.ObsidianID, now,
		)
		if err != nil {
			return processed, fmt.Errorf("upsert flashcard %q: %w", card.ObsidianID, err)
		}
		processed++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	return processed, nil
}
