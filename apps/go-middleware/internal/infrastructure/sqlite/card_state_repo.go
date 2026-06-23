package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/domain"
)

// CardStateRepository implements domain.CardStateRepository using SQLite.
type CardStateRepository struct {
	db *sql.DB
}

// NewCardStateRepository creates a new CardStateRepository backed by the given database.
func NewCardStateRepository(db *sql.DB) *CardStateRepository {
	return &CardStateRepository{db: db}
}

// Create inserts a new CardState. Returns domain.ErrConflict if a CardState
// already exists for the given FlashcardID (unique constraint).
func (r *CardStateRepository) Create(ctx context.Context, state *domain.CardState) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO card_states (id, flashcard_id, stability, difficulty, next_review, last_review)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		state.ID, state.FlashcardID, state.Stability, state.Difficulty,
		state.NextReview.UTC().Format(time.RFC3339),
		state.LastReview.UTC().Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return domain.ErrConflict
		}
		return fmt.Errorf("create card_state: %w", err)
	}
	return nil
}

// FindByFlashcardID returns the CardState for a given flashcard.
// Returns nil, domain.ErrNotFound when no CardState exists.
func (r *CardStateRepository) FindByFlashcardID(ctx context.Context, flashcardID string) (*domain.CardState, error) {
	var cs domain.CardState
	var nextReviewStr, lastReviewStr string

	err := r.db.QueryRowContext(ctx,
		`SELECT id, flashcard_id, stability, difficulty, next_review, last_review
		 FROM card_states WHERE flashcard_id = ?`,
		flashcardID,
	).Scan(&cs.ID, &cs.FlashcardID, &cs.Stability, &cs.Difficulty, &nextReviewStr, &lastReviewStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("query card_state by flashcard_id: %w", err)
	}

	cs.NextReview, _ = time.Parse(time.RFC3339, nextReviewStr)
	cs.LastReview, _ = time.Parse(time.RFC3339, lastReviewStr)
	return &cs, nil
}

// Update modifies the stability, difficulty, next_review, and last_review
// of an existing CardState identified by its ID.
func (r *CardStateRepository) Update(ctx context.Context, state *domain.CardState) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE card_states
		 SET stability = ?, difficulty = ?, next_review = ?, last_review = ?
		 WHERE id = ?`,
		state.Stability, state.Difficulty,
		state.NextReview.UTC().Format(time.RFC3339),
		state.LastReview.UTC().Format(time.RFC3339),
		state.ID,
	)
	if err != nil {
		return fmt.Errorf("update card_state: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
