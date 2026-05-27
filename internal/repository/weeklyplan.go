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
		ids := make([]string, len(recipes))
		for i := range recipes {
			if err := insertRecipe(ctx, tx, &recipes[i]); err != nil {
				return fmt.Errorf("create week recipe: %w", err)
			}
			ids[i] = recipes[i].ID
		}
		p.RecipeIDs = ids
		return insertPlanWithItems(ctx, tx, p)
	})
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

	const planQ = `INSERT INTO weekly_plan (id, household_id, week_start, recipe_ids, created_at)
		VALUES (?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, planQ,
		p.ID, p.HouseholdID, p.WeekStart.UTC().Format(dateLayout), recipeIDs, formatTime(p.CreatedAt)); err != nil {
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

	const planQ = `SELECT id, household_id, week_start, recipe_ids, created_at
		FROM weekly_plan WHERE id = ?`

	var (
		p                  domain.WeeklyPlan
		weekStart, recipes string
		createdAt          string
	)
	err := s.db.QueryRowContext(ctx, planQ, id).Scan(&p.ID, &p.HouseholdID, &weekStart, &recipes, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get weekly plan: %w", err)
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
	if p.ShoppingList, err = s.listShoppingItems(ctx, id); err != nil {
		return nil, err
	}
	return &p, nil
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
