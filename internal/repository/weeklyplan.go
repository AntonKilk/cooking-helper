package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

// CreateWeeklyPlan inserts a plan and all of its shopping-list items atomically
// in a single transaction. IDs and CreatedAt are assigned when empty.
func (s *Store) CreateWeeklyPlan(ctx context.Context, p *domain.WeeklyPlan) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		return insertPlanWithItems(ctx, tx, p)
	})
}

// CreateWeekWithRecipes persists a generated week — the 3 recipes and the plan
// that references them — in a single transaction, so a partial week never lands.
// Recipe IDs are assigned and copied into p.RecipeIDs; the plan's (possibly empty)
// shopping list is written alongside.
func (s *Store) CreateWeekWithRecipes(ctx context.Context, p *domain.WeeklyPlan, recipes []domain.Recipe) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		return insertWeekWithRecipes(ctx, tx, p, recipes)
	})
}

// ArchiveAndCreateWeek archives the previous active plan (when previousPlanID is
// non-empty) AND inserts the new plan + its recipes in one transaction, so a
// regenerate that fails halfway never leaves the household with no active plan.
// A previousPlanID that does not match a live (non-archived) row is treated as
// "already archived" and does not error — the create still proceeds.
func (s *Store) ArchiveAndCreateWeek(ctx context.Context, previousPlanID string, p *domain.WeeklyPlan, recipes []domain.Recipe) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		if previousPlanID != "" {
			if _, err := tx.ExecContext(ctx,
				`UPDATE weekly_plan SET archived_at = ? WHERE id = ? AND archived_at IS NULL`,
				formatTime(time.Now().UTC()), previousPlanID); err != nil {
				return fmt.Errorf("archive previous plan: %w", err)
			}
		}
		return insertWeekWithRecipes(ctx, tx, p, recipes)
	})
}

// insertWeekWithRecipes is the shared body of CreateWeekWithRecipes and
// ArchiveAndCreateWeek: inserts each recipe through tx, sets p.RecipeIDs to the
// assigned IDs, and inserts the plan plus its shopping list.
func insertWeekWithRecipes(ctx context.Context, tx *sql.Tx, p *domain.WeeklyPlan, recipes []domain.Recipe) error {
	ids := make([]string, len(recipes))
	for i := range recipes {
		if err := insertRecipe(ctx, tx, &recipes[i]); err != nil {
			return fmt.Errorf("create week recipe: %w", err)
		}
		ids[i] = recipes[i].ID
	}
	p.RecipeIDs = ids
	return insertPlanWithItems(ctx, tx, p)
}

// CurrentWeeklyPlan returns the household's currently-active plan (most recent
// row with archived_at IS NULL) and its shopping-list items, or ErrNotFound if
// the household has no active plan. ORDER BY created_at DESC is a safety net in
// case a race ever produced two actives.
func (s *Store) CurrentWeeklyPlan(ctx context.Context, householdID string) (*domain.WeeklyPlan, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	const q = `SELECT ` + planColumns + `
		FROM weekly_plan
		WHERE household_id = ? AND archived_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1`

	p, err := scanPlanRow(s.db.QueryRowContext(ctx, q, householdID))
	if err != nil {
		return nil, err
	}
	if p.ShoppingList, err = s.listShoppingItems(ctx, p.ID); err != nil {
		return nil, err
	}
	return p, nil
}

// SwapRecipeInPlan replaces oldRecipeID with newRecipe inside an existing plan
// atomically. The old recipe row is kept (archive history); only the plan's
// recipe_ids array rotates. The plan's shopping_list_item rows are cleared as a
// forward-compatible invalidation hook for CH-12. Returns ErrNotFound when the
// plan does not exist or oldRecipeID is not in its recipe_ids.
func (s *Store) SwapRecipeInPlan(ctx context.Context, planID, oldRecipeID string, newRecipe *domain.Recipe) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		var recipesJSON string
		err := tx.QueryRowContext(ctx,
			`SELECT recipe_ids FROM weekly_plan WHERE id = ?`, planID).Scan(&recipesJSON)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("load plan recipe_ids: %w", err)
		}
		ids, err := decodeStrings(recipesJSON)
		if err != nil {
			return err
		}
		idx := indexOf(ids, oldRecipeID)
		if idx < 0 {
			return ErrNotFound
		}

		if err := insertRecipe(ctx, tx, newRecipe); err != nil {
			return fmt.Errorf("insert swap recipe: %w", err)
		}

		ids[idx] = newRecipe.ID
		updated, err := encodeJSON(ids)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE weekly_plan SET recipe_ids = ? WHERE id = ?`, updated, planID); err != nil {
			return fmt.Errorf("update plan recipe_ids: %w", err)
		}

		if _, err := tx.ExecContext(ctx,
			`DELETE FROM shopping_list_item WHERE weekly_plan_id = ?`, planID); err != nil {
			return fmt.Errorf("clear shopping items: %w", err)
		}
		return nil
	})
}

// indexOf returns the index of v in xs or -1 when absent.
func indexOf(xs []string, v string) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return -1
}

// insertPlanWithItems writes a weekly plan and its shopping-list items through tx,
// assigning IDs and CreatedAt when empty. The caller owns the transaction.
func insertPlanWithItems(ctx context.Context, tx *sql.Tx, p *domain.WeeklyPlan) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	p.CreatedAt = time.Now().UTC()

	recipeIDs, err := encodeJSON(p.RecipeIDs)
	if err != nil {
		return err
	}

	const planQ = `INSERT INTO weekly_plan (id, household_id, week_start, recipe_ids, created_at, archived_at)
		VALUES (?, ?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, planQ,
		p.ID, p.HouseholdID, p.WeekStart.UTC().Format(dateLayout), recipeIDs,
		formatTime(p.CreatedAt), archivedColumn(p.ArchivedAt)); err != nil {
		return fmt.Errorf("create weekly plan: %w", err)
	}

	const itemQ = `INSERT INTO shopping_list_item
		(id, weekly_plan_id, household_id, name, amount, unit, category, checked, manually_removed, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	for i := range p.ShoppingList {
		item := &p.ShoppingList[i]
		if item.ID == "" {
			item.ID = uuid.NewString()
		}
		if _, err := tx.ExecContext(ctx, itemQ,
			item.ID, p.ID, p.HouseholdID, item.Name, item.Amount, item.Unit,
			string(item.Category), item.Checked, item.ManuallyRemoved, formatTime(p.CreatedAt)); err != nil {
			return fmt.Errorf("create shopping item: %w", err)
		}
	}
	return nil
}

// GetWeeklyPlan loads a plan and its shopping-list items, returning ErrNotFound
// if the plan does not exist.
func (s *Store) GetWeeklyPlan(ctx context.Context, id string) (*domain.WeeklyPlan, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	const planQ = `SELECT ` + planColumns + ` FROM weekly_plan WHERE id = ?`

	p, err := scanPlanRow(s.db.QueryRowContext(ctx, planQ, id))
	if err != nil {
		return nil, err
	}
	if p.ShoppingList, err = s.listShoppingItems(ctx, id); err != nil {
		return nil, err
	}
	return p, nil
}

// planColumns is the SELECT list shared by every plan read so the scan order in
// scanPlanRow stays in sync with the queries.
const planColumns = `id, household_id, week_start, recipe_ids, created_at, archived_at`

// scanPlanRow maps a weekly_plan row (in planColumns order) into a domain
// WeeklyPlan. ShoppingList is left to the caller — most reads load it via
// listShoppingItems; some (e.g. CurrentWeeklyPlan in a tight loop) do not need
// it. Returns ErrNotFound on no rows.
func scanPlanRow(row rowScanner) (*domain.WeeklyPlan, error) {
	var (
		p                  domain.WeeklyPlan
		weekStart, recipes string
		createdAt          string
		archivedAt         sql.NullString
	)
	err := row.Scan(&p.ID, &p.HouseholdID, &weekStart, &recipes, &createdAt, &archivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan weekly plan: %w", err)
	}

	if p.WeekStart, err = time.Parse(dateLayout, weekStart); err != nil {
		return nil, fmt.Errorf("parse week_start: %w", err)
	}
	if p.RecipeIDs, err = decodeStrings(recipes); err != nil {
		return nil, err
	}
	if p.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if archivedAt.Valid {
		t, perr := parseTime(archivedAt.String)
		if perr != nil {
			return nil, perr
		}
		p.ArchivedAt = &t
	}
	return &p, nil
}

// archivedColumn maps a nullable ArchivedAt to its storage form.
func archivedColumn(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*t), Valid: true}
}

// DeleteWeeklyPlan removes a plan (cascading to its shopping-list items).
func (s *Store) DeleteWeeklyPlan(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	res, err := s.db.ExecContext(ctx, `DELETE FROM weekly_plan WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete weekly plan: %w", err)
	}
	return requireOneRow(res, "delete weekly plan")
}

func (s *Store) listShoppingItems(ctx context.Context, planID string) ([]domain.ShoppingListItem, error) {
	const q = `SELECT id, name, amount, unit, category, checked, manually_removed
		FROM shopping_list_item WHERE weekly_plan_id = ? ORDER BY rowid`

	rows, err := s.db.QueryContext(ctx, q, planID)
	if err != nil {
		return nil, fmt.Errorf("list shopping items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []domain.ShoppingListItem
	for rows.Next() {
		var (
			item     domain.ShoppingListItem
			category string
		)
		if err := rows.Scan(&item.ID, &item.Name, &item.Amount, &item.Unit,
			&category, &item.Checked, &item.ManuallyRemoved); err != nil {
			return nil, fmt.Errorf("scan shopping item: %w", err)
		}
		item.Category = domain.IngredientCategory(category)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shopping items: %w", err)
	}
	return items, nil
}
