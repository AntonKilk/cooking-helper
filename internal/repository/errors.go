package repository

import "errors"

// ErrNotFound is returned when a requested record does not exist. It shields
// callers from database/sql sentinels (sql.ErrNoRows never escapes this package).
var ErrNotFound = errors.New("repository: not found")
