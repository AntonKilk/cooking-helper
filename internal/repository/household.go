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

// CreateHousehold inserts a new household profile. It assigns a UUID and sets
// timestamps when they are empty, mutating the passed profile in place.
func (s *Store) CreateHousehold(ctx context.Context, h *domain.HouseholdProfile) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if h.ID == "" {
		h.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	h.CreatedAt = now
	h.UpdatedAt = now

	disliked, err := encodeJSON(h.DislikedIngredients)
	if err != nil {
		return err
	}
	pantry, err := encodeJSON(h.PantryBasics)
	if err != nil {
		return err
	}

	const q = `INSERT INTO household_profile
		(id, language, family_adults, family_kids, disliked_ingredients, pantry_basics, onboarded, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = s.db.ExecContext(ctx, q,
		h.ID, string(h.Language), h.FamilySize.Adults, h.FamilySize.Kids,
		disliked, pantry, boolToInt(h.Onboarded), formatTime(h.CreatedAt), formatTime(h.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create household: %w", err)
	}
	return nil
}

// householdColumns is the SELECT list shared by every household read so the scan
// order in scanHousehold stays in sync with the queries.
const householdColumns = `id, language, family_adults, family_kids, disliked_ingredients, pantry_basics, onboarded, created_at, updated_at`

// boolToInt maps a Go bool onto SQLite's integer boolean convention (0/1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// rowScanner is satisfied by *sql.Row and *sql.Rows, letting scanHousehold serve
// both single-row and result-set queries.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanHousehold maps a household_profile row (in householdColumns order) into a
// domain profile, decoding the JSON and timestamp columns. It returns ErrNotFound
// when the underlying query yielded no row.
func scanHousehold(row rowScanner) (*domain.HouseholdProfile, error) {
	var (
		h                  domain.HouseholdProfile
		language           string
		disliked, pantry   string
		onboarded          int
		createdAt, updated string
	)
	err := row.Scan(
		&h.ID, &language, &h.FamilySize.Adults, &h.FamilySize.Kids,
		&disliked, &pantry, &onboarded, &createdAt, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan household: %w", err)
	}

	h.Language = domain.Language(language)
	h.Onboarded = onboarded != 0
	if h.DislikedIngredients, err = decodeStrings(disliked); err != nil {
		return nil, err
	}
	if h.PantryBasics, err = decodeStrings(pantry); err != nil {
		return nil, err
	}
	if h.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if h.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &h, nil
}

// GetHousehold loads a household profile by ID, returning ErrNotFound if absent.
func (s *Store) GetHousehold(ctx context.Context, id string) (*domain.HouseholdProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	q := `SELECT ` + householdColumns + ` FROM household_profile WHERE id = ?`
	return scanHousehold(s.db.QueryRowContext(ctx, q, id))
}

// FirstHousehold loads the oldest household profile, the single MVP household.
// It returns ErrNotFound when no profile exists yet.
func (s *Store) FirstHousehold(ctx context.Context) (*domain.HouseholdProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	q := `SELECT ` + householdColumns + ` FROM household_profile ORDER BY created_at ASC LIMIT 1`
	return scanHousehold(s.db.QueryRowContext(ctx, q))
}

// UpdateHousehold overwrites a household profile and bumps updated_at. Returns
// ErrNotFound if the row does not exist.
func (s *Store) UpdateHousehold(ctx context.Context, h *domain.HouseholdProfile) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	h.UpdatedAt = time.Now().UTC()

	disliked, err := encodeJSON(h.DislikedIngredients)
	if err != nil {
		return err
	}
	pantry, err := encodeJSON(h.PantryBasics)
	if err != nil {
		return err
	}

	const q = `UPDATE household_profile
		SET language = ?, family_adults = ?, family_kids = ?, disliked_ingredients = ?, pantry_basics = ?, onboarded = ?, updated_at = ?
		WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q,
		string(h.Language), h.FamilySize.Adults, h.FamilySize.Kids,
		disliked, pantry, boolToInt(h.Onboarded), formatTime(h.UpdatedAt), h.ID)
	if err != nil {
		return fmt.Errorf("update household: %w", err)
	}
	return requireOneRow(res, "update household")
}

// DeleteHousehold removes a household profile (cascading to its recipes and
// plans). Returns ErrNotFound if the row does not exist.
func (s *Store) DeleteHousehold(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	res, err := s.db.ExecContext(ctx, `DELETE FROM household_profile WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete household: %w", err)
	}
	return requireOneRow(res, "delete household")
}

// requireOneRow turns a zero-rows-affected result into ErrNotFound.
func requireOneRow(res sql.Result, op string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: rows affected: %w", op, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
