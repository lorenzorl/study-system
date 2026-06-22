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
