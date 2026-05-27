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

// CreateRecipe inserts a recipe, assigning a UUID and timestamps when empty.
func (s *Store) CreateRecipe(ctx context.Context, r *domain.Recipe) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if err := insertRecipe(ctx, s.db, r); err != nil {
		return fmt.Errorf("create recipe: %w", err)
	}
	return nil
}

// insertRecipe writes one recipe through any execer (*sql.DB or *sql.Tx),
// assigning a UUID and timestamps when empty. Sharing it lets a single recipe
// write and the atomic week write reuse one INSERT.
func insertRecipe(ctx context.Context, ex execer, r *domain.Recipe) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now

	ingredients, steps, err := encodeRecipeJSON(r)
	if err != nil {
		return err
	}
	liked, disliked, cookAgain, fbCreated := feedbackColumns(r.Feedback)

	const q = `INSERT INTO recipe
		(id, household_id, language, title, description, cook_time_minutes, servings,
		 ingredients, steps, source, feedback_liked, feedback_disliked, feedback_cook_again,
		 feedback_created_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := ex.ExecContext(ctx, q,
		r.ID, r.HouseholdID, string(r.Language), r.Title, r.Description,
		r.CookTimeMinutes, r.Servings, ingredients, steps, string(r.Source),
		liked, disliked, cookAgain, fbCreated, formatTime(r.CreatedAt), formatTime(r.UpdatedAt)); err != nil {
		return err
	}
	return nil
}

// GetRecipe loads a recipe by ID, returning ErrNotFound if absent.
func (s *Store) GetRecipe(ctx context.Context, id string) (*domain.Recipe, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	const q = `SELECT id, household_id, language, title, description, cook_time_minutes, servings,
		ingredients, steps, source, feedback_liked, feedback_disliked, feedback_cook_again,
		feedback_created_at, created_at, updated_at
		FROM recipe WHERE id = ?`

	r, err := scanRecipe(s.db.QueryRowContext(ctx, q, id))
	if err != nil {
		return nil, err
	}
	return r, nil
}

// recipeColumns is the SELECT list shared by every recipe read so the scan order
// in scanRecipe stays in sync with the queries.
const recipeColumns = `id, household_id, language, title, description, cook_time_minutes, servings,
	ingredients, steps, source, feedback_liked, feedback_disliked, feedback_cook_again,
	feedback_created_at, created_at, updated_at`

// scanRecipe maps a recipe row (in recipeColumns order) into a domain Recipe,
// decoding the JSON, feedback, and timestamp columns. It returns ErrNotFound when
// the underlying query yielded no row.
func scanRecipe(row rowScanner) (*domain.Recipe, error) {
	var (
		r                  domain.Recipe
		language, source   string
		ingredients, steps string
		liked, disliked    sql.NullBool
		cookAgain          sql.NullBool
		fbCreated          sql.NullString
		createdAt, updated string
	)
	err := row.Scan(
		&r.ID, &r.HouseholdID, &language, &r.Title, &r.Description,
		&r.CookTimeMinutes, &r.Servings, &ingredients, &steps, &source,
		&liked, &disliked, &cookAgain, &fbCreated, &createdAt, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan recipe: %w", err)
	}

	r.Language = domain.Language(language)
	r.Source = domain.RecipeSource(source)
	if r.Ingredients, err = decodeIngredients(ingredients); err != nil {
		return nil, err
	}
	if r.Steps, err = decodeStrings(steps); err != nil {
		return nil, err
	}
	if r.Feedback, err = scanFeedback(liked, disliked, cookAgain, fbCreated); err != nil {
		return nil, err
	}
	if r.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if r.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &r, nil
}

// RecentRecipes loads up to limit of the household's most recently created
// recipes, newest first. It feeds the generation prompt with week history and
// recent feedback.
func (s *Store) RecentRecipes(ctx context.Context, householdID string, limit int) ([]domain.Recipe, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	q := `SELECT ` + recipeColumns + `
		FROM recipe WHERE household_id = ? ORDER BY created_at DESC LIMIT ?`

	rows, err := s.db.QueryContext(ctx, q, householdID, limit)
	if err != nil {
		return nil, fmt.Errorf("recent recipes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var recipes []domain.Recipe
	for rows.Next() {
		r, err := scanRecipe(rows)
		if err != nil {
			return nil, err
		}
		recipes = append(recipes, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent recipes: %w", err)
	}
	return recipes, nil
}

// UpdateRecipe overwrites a recipe (including feedback) and bumps updated_at.
func (s *Store) UpdateRecipe(ctx context.Context, r *domain.Recipe) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	r.UpdatedAt = time.Now().UTC()

	ingredients, steps, err := encodeRecipeJSON(r)
	if err != nil {
		return err
	}
	liked, disliked, cookAgain, fbCreated := feedbackColumns(r.Feedback)

	const q = `UPDATE recipe SET
		language = ?, title = ?, description = ?, cook_time_minutes = ?, servings = ?,
		ingredients = ?, steps = ?, source = ?, feedback_liked = ?, feedback_disliked = ?,
		feedback_cook_again = ?, feedback_created_at = ?, updated_at = ?
		WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q,
		string(r.Language), r.Title, r.Description, r.CookTimeMinutes, r.Servings,
		ingredients, steps, string(r.Source), liked, disliked, cookAgain, fbCreated,
		formatTime(r.UpdatedAt), r.ID)
	if err != nil {
		return fmt.Errorf("update recipe: %w", err)
	}
	return requireOneRow(res, "update recipe")
}

// DeleteRecipe removes a recipe by ID, returning ErrNotFound if absent.
func (s *Store) DeleteRecipe(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	res, err := s.db.ExecContext(ctx, `DELETE FROM recipe WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete recipe: %w", err)
	}
	return requireOneRow(res, "delete recipe")
}

func encodeRecipeJSON(r *domain.Recipe) (ingredients, steps string, err error) {
	if ingredients, err = encodeJSON(r.Ingredients); err != nil {
		return "", "", err
	}
	if steps, err = encodeJSON(r.Steps); err != nil {
		return "", "", err
	}
	return ingredients, steps, nil
}

// feedbackColumns maps an optional Feedback to its four nullable columns.
func feedbackColumns(f *domain.Feedback) (liked, disliked, cookAgain sql.NullBool, createdAt sql.NullString) {
	if f == nil {
		return liked, disliked, cookAgain, createdAt
	}
	return sql.NullBool{Bool: f.Liked, Valid: true},
		sql.NullBool{Bool: f.Disliked, Valid: true},
		sql.NullBool{Bool: f.CookAgain, Valid: true},
		sql.NullString{String: formatTime(f.CreatedAt), Valid: true}
}

// scanFeedback reverses feedbackColumns; all-null columns yield a nil Feedback.
func scanFeedback(liked, disliked, cookAgain sql.NullBool, createdAt sql.NullString) (*domain.Feedback, error) {
	if !liked.Valid && !disliked.Valid && !cookAgain.Valid && !createdAt.Valid {
		return nil, nil
	}
	f := &domain.Feedback{
		Liked:     liked.Bool,
		Disliked:  disliked.Bool,
		CookAgain: cookAgain.Bool,
	}
	if createdAt.Valid {
		t, err := parseTime(createdAt.String)
		if err != nil {
			return nil, err
		}
		f.CreatedAt = t
	}
	return f, nil
}
