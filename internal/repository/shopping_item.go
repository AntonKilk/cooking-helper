package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

// GetShoppingItem loads a single shopping-list item by ID, returning ErrNotFound
// when no such row exists.
func (s *Store) GetShoppingItem(ctx context.Context, id string) (*domain.ShoppingListItem, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	const q = `SELECT id, name, amount, unit, category, checked, manually_removed
		FROM shopping_list_item WHERE id = ?`

	var (
		item     domain.ShoppingListItem
		category string
	)
	err := s.db.QueryRowContext(ctx, q, id).Scan(
		&item.ID, &item.Name, &item.Amount, &item.Unit,
		&category, &item.Checked, &item.ManuallyRemoved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get shopping item: %w", err)
	}
	item.Category = domain.IngredientCategory(category)
	return &item, nil
}

// SetShoppingItemChecked sets an item's checked flag to an absolute value. The
// write is idempotent — applying the same state twice is a no-op — so a queued
// offline write replayed by the Service Worker is safe. Returns ErrNotFound when
// the item does not exist.
func (s *Store) SetShoppingItemChecked(ctx context.Context, id string, checked bool) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	res, err := s.db.ExecContext(ctx,
		`UPDATE shopping_list_item SET checked = ? WHERE id = ?`, checked, id)
	if err != nil {
		return fmt.Errorf("set shopping item checked: %w", err)
	}
	return requireOneRow(res, "set shopping item checked")
}

// SetShoppingItemRemoved sets an item's manually-removed flag to an absolute
// value (true to remove, false to restore). Like the checked write it is
// idempotent and safe to replay. Returns ErrNotFound when the item does not exist.
func (s *Store) SetShoppingItemRemoved(ctx context.Context, id string, removed bool) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	res, err := s.db.ExecContext(ctx,
		`UPDATE shopping_list_item SET manually_removed = ? WHERE id = ?`, removed, id)
	if err != nil {
		return fmt.Errorf("set shopping item removed: %w", err)
	}
	return requireOneRow(res, "set shopping item removed")
}
