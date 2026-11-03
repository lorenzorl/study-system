package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/platform/apperror"
	"github.com/lorenzorangel/study-system/apps/go-middleware/internal/study/domain"
)

// StudyPlanRepository implements domain.StudyPlanRepository over SQLite.
type StudyPlanRepository struct {
	db *sql.DB
}

// NewStudyPlanRepository creates a StudyPlanRepository backed by the given
// database.
func NewStudyPlanRepository(db *sql.DB) *StudyPlanRepository {
	return &StudyPlanRepository{db: db}
}

// CreateActive stores one plan with its items and supersedes the document's
// previous active plan, in one transaction.
//
// One transaction because the two facts are one decision: a plan inserted without
// its items would claim a schedule with nothing in it, and superseding without
// inserting would leave the book unplanned. The previous plan's rows are kept and
// only stamped, so throwing a plan away never loses what it said.
func (r *StudyPlanRepository) CreateActive(
	ctx context.Context,
	plan *domain.StudyPlan,
) (*domain.StudyPlan, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	supersededAt := domain.Now().UTC().Format(time.RFC3339)
	_, err = tx.ExecContext(ctx,
		`UPDATE study_plans SET superseded_at = ?
		 WHERE document_id = ? AND superseded_at IS NULL`,
		supersededAt, plan.DocumentID,
	)
	if err != nil {
		return nil, fmt.Errorf("supersede the active plan: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO study_plans (id, document_id, reading_mode, objective, source, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		plan.ID,
		plan.DocumentID,
		string(plan.ReadingMode),
		plan.Objective,
		plan.Source,
		plan.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("create study_plan: %w", err)
	}

	for _, item := range plan.Items {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO study_plan_items (id, plan_id, position, title, from_page, to_page, note, state)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			item.ID,
			plan.ID,
			item.Position,
			item.Title,
			item.FromPage,
			item.ToPage,
			item.Note,
			string(item.State),
		)
		if err != nil {
			return nil, fmt.Errorf("create study_plan_item at position %d: %w", item.Position, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return plan, nil
}

// FindActiveByDocument returns the active plan with its items in position order,
// or apperror.ErrNotFound when the document has none.
func (r *StudyPlanRepository) FindActiveByDocument(
	ctx context.Context,
	documentID string,
) (*domain.StudyPlan, error) {
	var plan domain.StudyPlan
	var readingMode, createdAt string
	var supersededAt sql.NullString

	err := r.db.QueryRowContext(ctx,
		`SELECT id, document_id, reading_mode, objective, source, created_at, superseded_at
		 FROM study_plans
		 WHERE document_id = ? AND superseded_at IS NULL
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		documentID,
	).Scan(
		&plan.ID,
		&plan.DocumentID,
		&readingMode,
		&plan.Objective,
		&plan.Source,
		&createdAt,
		&supersededAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("query active study_plan: %w", err)
	}

	plan.ReadingMode = domain.ReadingMode(readingMode)
	if plan.CreatedAt, err = time.Parse(time.RFC3339, createdAt); err != nil {
		return nil, fmt.Errorf("parse study_plan.created_at %q: %w", createdAt, err)
	}
	if supersededAt.Valid {
		parsed, err := time.Parse(time.RFC3339, supersededAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse study_plan.superseded_at %q: %w", supersededAt.String, err)
		}
		plan.SupersededAt = &parsed
	}

	items, err := listPlanItems(ctx, r.db, plan.ID)
	if err != nil {
		return nil, err
	}
	plan.Items = items

	return &plan, nil
}

func listPlanItems(ctx context.Context, db *sql.DB, planID string) ([]domain.StudyPlanItem, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, plan_id, position, title, from_page, to_page, note, state
		 FROM study_plan_items
		 WHERE plan_id = ?
		 ORDER BY position`,
		planID,
	)
	if err != nil {
		return nil, fmt.Errorf("query study_plan_items: %w", err)
	}
	defer rows.Close()

	items := make([]domain.StudyPlanItem, 0)
	for rows.Next() {
		var item domain.StudyPlanItem
		var state string
		if err := rows.Scan(
			&item.ID,
			&item.PlanID,
			&item.Position,
			&item.Title,
			&item.FromPage,
			&item.ToPage,
			&item.Note,
			&state,
		); err != nil {
			return nil, fmt.Errorf("scan study_plan_item: %w", err)
		}
		item.State = domain.PlanItemState(state)
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate study_plan_items: %w", err)
	}

	return items, nil
}
