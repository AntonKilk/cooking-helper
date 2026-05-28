package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

// CategoriesByNames returns the cached store category for each given normalized
// ingredient name that has one. Names without a cached category are simply absent
// from the returned map; an empty input yields an empty map. The cache is global
// (keyed by name only) — an ingredient's store section does not vary by household.
func (s *Store) CategoriesByNames(ctx context.Context, names []string) (map[string]domain.IngredientCategory, error) {
	out := make(map[string]domain.IngredientCategory, len(names))
	if len(names) == 0 {
		return out, nil
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	placeholders := strings.Repeat("?,", len(names))
	placeholders = placeholders[:len(placeholders)-1]
	q := `SELECT name_normalized, category FROM ingredient_category WHERE name_normalized IN (` + placeholders + `)`

	args := make([]any, len(names))
	for i, n := range names {
		args[i] = n
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("categories by names: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var name, category string
		if err := rows.Scan(&name, &category); err != nil {
			return nil, fmt.Errorf("scan ingredient category: %w", err)
		}
		out[name] = domain.IngredientCategory(category)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ingredient categories: %w", err)
	}
	return out, nil
}

// SaveCategory caches the store category for a normalized ingredient name. It is
// idempotent — re-caching an already-cached name is a no-op — so it is safe to
// retry and races resolve harmlessly to first-writer-wins.
func (s *Store) SaveCategory(ctx context.Context, nameNormalized string, c domain.IngredientCategory) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	const q = `INSERT INTO ingredient_category (name_normalized, category, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(name_normalized) DO NOTHING`
	if _, err := s.db.ExecContext(ctx, q, nameNormalized, string(c), formatTime(time.Now().UTC())); err != nil {
		return fmt.Errorf("save ingredient category: %w", err)
	}
	return nil
}
