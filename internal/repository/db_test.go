package repository

import (
	"path/filepath"
	"testing"
)

// newTestStore opens a fresh migrated SQLite database in a temp directory and
// returns a Store bound to it. The DB is closed automatically when the test ends.
func newTestStore(t *testing.T) *Store {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return New(db)
}
