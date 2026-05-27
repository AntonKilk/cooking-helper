package repository

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// Open returns a SQLite-backed *sql.DB for the given filesystem path. It enables
// foreign keys, WAL, and a busy timeout on every connection via DSN pragmas, and
// caps the pool at one connection since SQLite is single-writer.
func Open(path string) (*sql.DB, error) {
	dsn := dsnFromPath(path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// SQLite serializes writes; a single connection keeps pragmas and write
	// ordering consistent and avoids "database is locked" under concurrency.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return db, nil
}

// dsnFromPath builds a modernc.org/sqlite DSN with the pragmas we always want.
func dsnFromPath(path string) string {
	pragmas := []string{
		"_pragma=foreign_keys(1)",
		"_pragma=journal_mode(WAL)",
		"_pragma=busy_timeout(5000)",
	}
	return "file:" + path + "?" + strings.Join(pragmas, "&")
}
