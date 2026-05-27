package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

// queryTimeout bounds every repository query so nothing blocks indefinitely.
const queryTimeout = 5 * time.Second

// Store is the single entry point for SQL access. All persistence goes through
// its methods; no SQL lives outside this package.
type Store struct {
	db *sql.DB
}

// New returns a Store backed by the given database.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// withTx runs fn inside a transaction, committing on success and rolling back on
// error or panic. Used for atomic multi-table writes.
func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// dateLayout is how week_start (a calendar date, no time) is stored.
const dateLayout = "2006-01-02"

// formatTime serializes a timestamp for storage. Times are normalized to UTC so
// round-trips are deterministic regardless of the server's local zone.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// parseTime reverses formatTime.
func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time: %w", err)
	}
	return t, nil
}

// encodeJSON marshals a value to a JSON string for storage in a TEXT column.
func encodeJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode json: %w", err)
	}
	return string(b), nil
}

// decodeStrings unmarshals a JSON TEXT column into a string slice.
func decodeStrings(s string) ([]string, error) {
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("decode strings: %w", err)
	}
	return out, nil
}

// decodeIngredients unmarshals a JSON TEXT column into an ingredient slice.
func decodeIngredients(s string) ([]domain.Ingredient, error) {
	var out []domain.Ingredient
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("decode ingredients: %w", err)
	}
	return out, nil
}
