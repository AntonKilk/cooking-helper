# Plan: CH-3 Data Models & SQLite Repository Layer

## Summary

Build the persistence foundation for Cooking Helper: typed domain structs for the PRD §15
data model (`HouseholdProfile`, `Recipe`/`Ingredient`/`Feedback`, `WeeklyPlan`,
`ShoppingListItem`) in `internal/domain` with **zero** infra dependencies; a first
`golang-migrate` migration (embedded in the binary, applied on startup) that creates all
tables; and `internal/repository` providing typed CRUD per model via `database/sql` with a
pure-Go SQLite driver (`modernc.org/sqlite`, keeps `CGO_ENABLED=0`). Multi-table writes
(WeeklyPlan + its ShoppingListItems) go through one transaction. Slice/embedded fields
(ingredients, steps, disliked, pantry, recipe_ids) are stored as JSON TEXT columns;
the optional `Feedback` is stored as nullable columns on `recipe`. Each model gets CRUD
unit tests against a fresh SQLite file in `t.TempDir()`.

## User Story

As a developer
I want a persistence layer with typed models and a single repository interface
So that the rest of the features read/write data through one consistent, SQL-isolated layer.

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY |
| Complexity | MEDIUM |
| Systems Affected | `internal/domain`, `internal/repository`, `migrations/`, `cmd/server`, `internal/handler`, `go.mod`/`go.sum`, `Dockerfile`, `docker-compose.yml` |
| GitHub Issue | #3 (CH-3) |

---

## Locked-in Constraints (from CLAUDE.md + tech-design §3.3)

- **Stack is fixed**: Go + `database/sql` + SQLite. No ORM. No re-litigating.
- **Layer direction**: handlers → services → repositories → domain. **SQL lives ONLY in `internal/repository`.**
- **Domain purity**: no `sql.Row`, no HTTP types, no framework deps in `internal/domain`. IDs are `string` (documented UUID) so domain stays dependency-free; the `repository` generates UUIDs via `github.com/google/uuid`.
- **CGO off**: the Dockerfile builds with `CGO_ENABLED=0` and explicitly mandates the pure-Go `modernc.org/sqlite` driver. Do **not** use `mattn/go-sqlite3` (cgo).
- **Migrations**: `golang-migrate`, never hand-edit schema, **applied on startup**. Files live in `migrations/` (root, for CLI use) AND are embedded via `//go:embed` so the single binary self-applies them (the Docker run stage copies only the binary, not the files).
- **Fault tolerance**: explicit `context` timeout on every query; SQLite is single-writer → keep write transactions short. `PRAGMA foreign_keys=ON`, `busy_timeout`, WAL.
- **Observability**: wrap errors with context `fmt.Errorf("...: %w", err)`; never leak `sql` details out of the repository.
- **Security**: `govulncheck ./...` before adding/bumping deps.
- **Every table carries `household_id` UUID** (future multi-user).
- **`created_at` / `updated_at`** on the relevant tables, set in Go (UTC, deterministic for tests).

---

## Patterns to Follow

### Naming / package docs
```go
// SOURCE: internal/repository/doc.go:1-3
// Package repository is the only place SQL lives. It provides typed CRUD access
// to the SQLite database via database/sql (no ORM). Wired up in CH-3.
package repository
```

### Error Handling
```go
// SOURCE: cmd/server/main.go:63-65
if err != nil {
    return fmt.Errorf("listen and serve: %w", err)
}
```

### Tests (got/want, t.Fatalf)
```go
// SOURCE: internal/handler/health_test.go:16-30
func TestHealth_OK(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
    rec := httptest.NewRecorder()
    Health(rec, req)
    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
    }
}
```

### Logger-in-test helper
```go
// SOURCE: internal/handler/health_test.go:12-14
func slogDiscard() *slog.Logger {
    return slog.New(slog.NewTextHandler(io.Discard, nil))
}
```

---

## Schema (migration `000001_init`)

> Reverse-order DROP in the `.down.sql`. SQLite stores TEXT for UUIDs and RFC3339 timestamps;
> JSON-encoded slices live in TEXT columns. Timestamps are written from Go for determinism.

**`household_profile`**: `id` TEXT PK · `language` TEXT CHECK in (ru,fi,en) · `family_adults` INT · `family_kids` INT · `disliked_ingredients` TEXT (JSON `[]string`, default `'[]'`) · `pantry_basics` TEXT (JSON `[]string`, default `'[]'`) · `created_at` · `updated_at`.

**`recipe`**: `id` TEXT PK · `household_id` TEXT NOT NULL REFERENCES household_profile(id) ON DELETE CASCADE · `language` TEXT CHECK · `title` · `description` · `cook_time_minutes` INT · `servings` INT · `ingredients` TEXT (JSON `[]Ingredient`) · `steps` TEXT (JSON `[]string`) · `source` TEXT CHECK in (llm,history) · `feedback_liked` INT NULL · `feedback_disliked` INT NULL · `feedback_cook_again` INT NULL · `feedback_created_at` TIMESTAMP NULL · `created_at` · `updated_at`. Index on `household_id`.

**`weekly_plan`**: `id` TEXT PK · `household_id` TEXT NOT NULL REFERENCES household_profile(id) ON DELETE CASCADE · `week_start` TEXT (date `YYYY-MM-DD`) · `recipe_ids` TEXT (JSON `[]string`) · `created_at`. Index on `household_id`.

**`shopping_list_item`**: `id` TEXT PK · `weekly_plan_id` TEXT NOT NULL REFERENCES weekly_plan(id) ON DELETE CASCADE · `household_id` TEXT NOT NULL · `name` · `amount` REAL · `unit` · `category` TEXT (default `'other'`) · `checked` INT (0/1) · `manually_removed` INT (0/1) · `created_at`. Index on `weekly_plan_id`.

> **Design note**: `recipe_ids` is a JSON array column to match the PRD high-level model and keep
> scope tight; the only AC-required atomic multi-table write is **WeeklyPlan + ShoppingListItems**,
> which the transaction in Task 7 covers. Feedback is 1:1-optional → nullable columns, not a side table.

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `go.mod` / `go.sum` | UPDATE | Add `modernc.org/sqlite`, `github.com/golang-migrate/migrate/v4`, `github.com/google/uuid`; `go mod tidy`. |
| `internal/domain/household.go` | CREATE | `HouseholdProfile`, `FamilySize`, `Language` + consts. |
| `internal/domain/recipe.go` | CREATE | `Recipe`, `Ingredient`, `IngredientCategory` + consts, `RecipeSource` + consts, `Feedback`. |
| `internal/domain/plan.go` | CREATE | `WeeklyPlan`, `ShoppingListItem`. |
| `internal/domain/doc.go` | KEEP | Existing package comment stays. |
| `migrations/000001_init.up.sql` | CREATE | Create all four tables + indexes. |
| `migrations/000001_init.down.sql` | CREATE | Drop tables in reverse order. |
| `migrations/embed.go` | CREATE | `package migrations` + `//go:embed *.sql` → exported `embed.FS` for startup auto-apply. |
| `internal/repository/db.go` | CREATE | `Open(dsn)` → `*sql.DB` with modernc driver + PRAGMAs (foreign_keys, WAL, busy_timeout). |
| `internal/repository/migrate.go` | CREATE | `RunMigrations(db)` via golang-migrate `iofs` source (embedded FS) + sqlite `WithInstance`. |
| `internal/repository/store.go` | CREATE | `Store` struct, `New(db)`, `withTx` helper, query-timeout const, JSON marshal/unmarshal helpers. |
| `internal/repository/household.go` | CREATE | CRUD for `HouseholdProfile`. |
| `internal/repository/recipe.go` | CREATE | CRUD for `Recipe` (incl. feedback columns). |
| `internal/repository/weeklyplan.go` | CREATE | Create (txn: plan + items), Get (loads items), Delete, list items. |
| `internal/repository/errors.go` | CREATE | `ErrNotFound` sentinel returned to callers (no `sql.ErrNoRows` leakage). |
| `internal/repository/doc.go` | KEEP | Existing package comment stays. |
| `internal/repository/household_test.go` | CREATE | CRUD round-trip on temp-dir DB. |
| `internal/repository/recipe_test.go` | CREATE | CRUD + feedback round-trip. |
| `internal/repository/weeklyplan_test.go` | CREATE | Txn create + cascade + item load. |
| `internal/repository/db_test.go` | CREATE | Shared `newTestStore(t)` helper (Open + RunMigrations on `t.TempDir()`). |
| `cmd/server/main.go` | UPDATE | Open DB from `DB_PATH` env, `RunMigrations` on startup, build `Store`, pass DB to router; close on shutdown. |
| `internal/handler/router.go` | UPDATE | `NewRouter(logger, db)` — pass DB to the health handler. |
| `internal/handler/health.go` | UPDATE | Real readiness: `PingContext` → 200 / 503 (fulfills the existing stub TODO). |
| `internal/handler/health_test.go` | UPDATE | Adjust to new `NewRouter`/health signature with a ping stub or temp DB. |
| `Dockerfile` | UPDATE | `COPY go.mod go.sum ./` before `go mod download` (per existing line-12 TODO). |
| `docker-compose.yml` | UPDATE | Add `DB_PATH: "/data/cooking.db"` env. |

---

## Tasks

Execute in order. Each is atomic and verifiable.

### Task 1: Add dependencies

- **Files**: `go.mod`, `go.sum`
- **Action**: UPDATE
- **Implement**: `go get modernc.org/sqlite github.com/golang-migrate/migrate/v4 github.com/google/uuid`, then `go mod tidy`. Run `govulncheck ./...` (CLAUDE.md mandates it before adding deps).
- **Validate**: `go build ./... && go mod verify`

### Task 2: Domain models

- **Files**: `internal/domain/household.go`, `internal/domain/recipe.go`, `internal/domain/plan.go`
- **Action**: CREATE
- **Implement**: PRD §15 structs. `Language`, `IngredientCategory`, `RecipeSource` as named string types with exported consts (`LanguageRU/FI/EN`; categories `produce/meat_fish/dairy/pantry/frozen/other`; sources `llm/history`). IDs are `string`. `Recipe.Feedback` is `*Feedback`. `WeeklyPlan.WeekStart time.Time`, `WeeklyPlan.RecipeIDs []string`, `WeeklyPlan.ShoppingList []ShoppingListItem`. NO imports beyond `time`.
- **Mirror**: `internal/domain/doc.go` (package comment style), CLAUDE.md "Typing" section.
- **Validate**: `go build ./internal/domain/... && go vet ./internal/domain/...`

### Task 3: Migration files + embed

- **Files**: `migrations/000001_init.up.sql`, `migrations/000001_init.down.sql`, `migrations/embed.go`
- **Action**: CREATE
- **Implement**: `.up.sql` creates the four tables + indexes per the Schema section (FKs `ON DELETE CASCADE`, CHECK constraints, JSON-default `'[]'`). `.down.sql` drops in reverse FK order (shopping_list_item → weekly_plan → recipe → household_profile). `embed.go`: `package migrations` with `//go:embed *.sql` exporting `var FS embed.FS`.
- **Validate**: `go build ./migrations/...`

### Task 4: DB open + PRAGMAs

- **File**: `internal/repository/db.go`
- **Action**: CREATE
- **Implement**: `import _ "modernc.org/sqlite"`. `Open(dsn string) (*sql.DB, error)` → `sql.Open("sqlite", dsn)`, then exec `PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`. Set `db.SetMaxOpenConns(1)` (SQLite single-writer). Wrap errors `fmt.Errorf("open db: %w", err)`.
- **Validate**: `go build ./internal/repository/...`

### Task 5: Migration runner

- **File**: `internal/repository/migrate.go`
- **Action**: CREATE
- **Implement**: `RunMigrations(db *sql.DB) error` using `iofs.New(migrations.FS, ".")` as source and the pure-Go `database/sqlite` driver's `WithInstance(db, &sqlite.Config{})`; `m.Up()` and treat `migrate.ErrNoChange` as success. Wrap errors with context.
- **Mirror**: error-wrapping pattern `cmd/server/main.go:63-65`.
- **Validate**: `go build ./internal/repository/...`

### Task 6: Store scaffolding + errors + JSON helpers

- **Files**: `internal/repository/store.go`, `internal/repository/errors.go`
- **Action**: CREATE
- **Implement**: `Store{ db *sql.DB }`, `New(db) *Store`. `withTx(ctx, fn func(*sql.Tx) error) error` (begin, rollback on error/panic, commit). `const queryTimeout = 5 * time.Second` and a `ctx, cancel := context.WithTimeout(...)` helper. Unexported JSON marshal/unmarshal helpers for `[]string` / `[]domain.Ingredient`. `errors.go`: `var ErrNotFound = errors.New("not found")`; map `sql.ErrNoRows` → `ErrNotFound` inside repo methods so callers never see `sql` types.
- **Validate**: `go build ./internal/repository/...`

### Task 7: CRUD — Household, Recipe, WeeklyPlan(+items in txn)

- **Files**: `internal/repository/household.go`, `internal/repository/recipe.go`, `internal/repository/weeklyplan.go`
- **Action**: CREATE
- **Implement**:
  - All write methods accept `context.Context`, use parameterized queries, set `created_at`/`updated_at` via `time.Now().UTC()`, generate IDs with `uuid.NewString()` when empty.
  - **Household**: `CreateHousehold`, `GetHousehold`, `UpdateHousehold` (bumps `updated_at`), `DeleteHousehold`. JSON-encode disliked/pantry.
  - **Recipe**: `CreateRecipe`, `GetRecipe`, `UpdateRecipe`, `DeleteRecipe`. Marshal ingredients/steps; map `*Feedback` ↔ four nullable columns (`sql.NullBool`/`sql.NullTime`).
  - **WeeklyPlan**: `CreateWeeklyPlan` inserts the plan **and all `ShoppingList` items inside one `withTx`** (the AC-required atomic write); `GetWeeklyPlan` loads the plan then its items; `DeleteWeeklyPlan`. Encode `RecipeIDs` as JSON; `week_start` as `YYYY-MM-DD`.
- **Mirror**: error wrapping `cmd/server/main.go:63-65`; domain types from Task 2.
- **Validate**: `go build ./internal/repository/... && go vet ./internal/repository/...`

### Task 8: Repository unit tests

- **Files**: `internal/repository/db_test.go`, `internal/repository/household_test.go`, `internal/repository/recipe_test.go`, `internal/repository/weeklyplan_test.go`
- **Action**: CREATE
- **Implement**: `newTestStore(t)` in `db_test.go` → `Open(filepath.Join(t.TempDir(), "test.db"))`, `RunMigrations`, register `t.Cleanup` to close. Per-model: create → get (assert round-trip incl. JSON slices & feedback) → update → get → delete → get returns `ErrNotFound`. WeeklyPlan test: create plan with ≥2 shopping items, assert items load back; assert deleting the household/plan cascades (items gone). Use `context.Background()`.
- **Mirror**: `internal/handler/health_test.go:12-30` (got/want, `t.Fatalf`).
- **Validate**: `go test ./internal/repository/...`

### Task 9: Wire startup migration + DB into server (fulfills healthz stub)

- **Files**: `cmd/server/main.go`, `internal/handler/router.go`, `internal/handler/health.go`, `internal/handler/health_test.go`, `docker-compose.yml`
- **Action**: UPDATE
- **Implement**:
  - `main.go`: read `DB_PATH` env (default `data/cooking.db`), `repository.Open`, `repository.RunMigrations` (fail fast on error — this satisfies "applied on startup"), `defer db.Close()`, pass `db` into `handler.NewRouter`.
  - `router.go`: `NewRouter(logger *slog.Logger, db *sql.DB) http.Handler`; mount health handler built from `db`.
  - `health.go`: define a minimal `pinger interface { PingContext(context.Context) error }`; handler pings with a short context → `200 {"status":"ok"}` / `503 {"status":"unavailable"}`. Update the stub comment.
  - `health_test.go`: pass a fake pinger (or temp DB) into the handler/router; add a 503 case for a failing ping.
  - `docker-compose.yml`: add `DB_PATH: "/data/cooking.db"` under `environment`.
- **Mirror**: existing middleware/router style `internal/handler/router.go:18-23`.
- **Validate**: `go build ./... && go test ./internal/handler/...`

### Task 10: Dockerfile go.sum copy

- **File**: `Dockerfile`
- **Action**: UPDATE
- **Implement**: Change line 7 to `COPY go.mod go.sum ./` (per the existing line-12 TODO comment) so the build stage restores deps reproducibly. Verify the build still produces a static binary with `CGO_ENABLED=0` (modernc is pure-Go, so no cgo needed).
- **Validate**: `docker build -t cooking-helper .` (if Docker available; otherwise `go build` already covers compilation).

---

## Risks

| Risk | Mitigation |
|------|------------|
| Picking a cgo SQLite driver breaks the `CGO_ENABLED=0` static build | Use `modernc.org/sqlite` (pure Go) — explicitly mandated by `Dockerfile:11-12`. |
| Migration files not present in the Docker run stage (only the binary is copied) | Embed migrations via `//go:embed` + `iofs` source; never read from disk at runtime. |
| golang-migrate sqlite driver mismatch with modernc | Use golang-migrate's pure-Go `database/sqlite` driver (also modernc-based) with `WithInstance` reusing the same `*sql.DB`. |
| FK cascade not enforced | `PRAGMA foreign_keys=ON` on every connection in `Open`; `SetMaxOpenConns(1)` so the pragma applies to the one connection. |
| WAL files in temp test dirs | `t.TempDir()` is auto-cleaned; close DB in `t.Cleanup` before the dir is removed. |
| SQL leaking out of repository | Map `sql.ErrNoRows` → `repository.ErrNotFound`; keep all SQL in `internal/repository`. |
| Domain coupling to infra | IDs are `string`; `google/uuid` used only in repository. |
| `recipe.feedback` round-trip loses partial nulls | Use `sql.NullBool`/`sql.NullTime`; treat all-null as `Feedback == nil`. |

---

## Validation

```bash
gofmt -s -l .            # formatting (no output = clean)
go vet ./...             # vet
golangci-lint run ./...  # lint (errcheck, govet, ineffassign, staticcheck, unused)
go test ./...            # tests
govulncheck ./...        # dependency vulnerabilities (after Task 1 dep changes)
```

---

## Acceptance Criteria

- [ ] PRD §15 schemas implemented as Go structs in `internal/domain`, no `sql.Row`/HTTP types inside
- [ ] First `golang-migrate` migration creates all tables; applied on startup (embedded, fail-fast)
- [ ] `internal/repository` does CRUD per model via `database/sql`, no ORM
- [ ] SQL exists ONLY in `internal/repository` (not in service/handler)
- [ ] `household_id` UUID on every table
- [ ] WeeklyPlan + ShoppingList written in a single transaction
- [ ] Unit tests cover CRUD per model against a temp-dir SQLite file
- [ ] `created_at` / `updated_at` populated; query timeouts via `context`
- [ ] All validation commands pass; `CGO_ENABLED=0` build still static
