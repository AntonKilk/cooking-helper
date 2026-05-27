# Implementation Report

**Plan**: `.agents/plans/completed/ch-3-data-models-repository.plan.md`
**Branch**: `claude/prime-3-8ts86`
**Status**: COMPLETE

## Summary

Built the CH-3 persistence layer: PRD §15 domain structs in `internal/domain` (no infra
deps), a first `golang-migrate` migration embedded in the binary and applied on startup, and
`internal/repository` providing typed CRUD per model via `database/sql` with the pure-Go
`modernc.org/sqlite` driver (keeps `CGO_ENABLED=0`). WeeklyPlan + its ShoppingListItems are
written atomically in one transaction. Foreign keys, WAL, and a busy timeout are enabled per
connection; SQL never leaves the repository package, and `sql.ErrNoRows` is mapped to a
package `ErrNotFound`. The server now opens the DB, runs migrations on boot, and `/healthz`
performs a real readiness ping (200/503).

## Tasks Completed

| # | Task | File(s) | Status |
|---|------|---------|--------|
| 1 | Add deps (sqlite, migrate, uuid) | `go.mod`, `go.sum` | ✅ |
| 2 | Domain models | `internal/domain/{household,recipe,plan}.go` | ✅ |
| 3 | Migration files + embed | `migrations/000001_init.{up,down}.sql`, `migrations/embed.go` | ✅ |
| 4 | DB open + pragmas | `internal/repository/db.go` | ✅ |
| 5 | Migration runner | `internal/repository/migrate.go` | ✅ |
| 6 | Store, errors, helpers | `internal/repository/{store,errors}.go` | ✅ |
| 7 | CRUD per model + txn | `internal/repository/{household,recipe,weeklyplan}.go` | ✅ |
| 8 | Repository unit tests | `internal/repository/*_test.go` | ✅ |
| 9 | Wire DB + migrations into server; real healthz | `cmd/server/main.go`, `internal/handler/{router,health,health_test}.go`, `docker-compose.yml` | ✅ |
| 10 | Dockerfile go.sum copy + go 1.25 | `Dockerfile` | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ clean |
| `golangci-lint run ./...` | ✅ 0 issues |
| `go test ./...` | ✅ pass (handler + repository) |
| `CGO_ENABLED=0` static build | ✅ builds |
| `go mod verify` | ✅ all modules verified |
| E2E: server boot → migrate → `/healthz` | ✅ 200, all tables created on fresh DB |
| `govulncheck ./...` | ⚠️ blocked — `vuln.go.dev` returns 403 under the sandbox network policy |

## Files Changed

| File | Action |
|------|--------|
| `internal/domain/household.go` | CREATE |
| `internal/domain/recipe.go` | CREATE |
| `internal/domain/plan.go` | CREATE |
| `migrations/000001_init.up.sql` | CREATE |
| `migrations/000001_init.down.sql` | CREATE |
| `migrations/embed.go` | CREATE |
| `internal/repository/db.go` | CREATE |
| `internal/repository/migrate.go` | CREATE |
| `internal/repository/store.go` | CREATE |
| `internal/repository/errors.go` | CREATE |
| `internal/repository/household.go` | CREATE |
| `internal/repository/recipe.go` | CREATE |
| `internal/repository/weeklyplan.go` | CREATE |
| `internal/repository/db_test.go` | CREATE |
| `internal/repository/household_test.go` | CREATE |
| `internal/repository/recipe_test.go` | CREATE |
| `internal/repository/weeklyplan_test.go` | CREATE |
| `cmd/server/main.go` | UPDATE |
| `internal/handler/router.go` | UPDATE |
| `internal/handler/health.go` | UPDATE |
| `internal/handler/health_test.go` | UPDATE |
| `docker-compose.yml` | UPDATE |
| `Dockerfile` | UPDATE |
| `go.mod` / `go.sum` | UPDATE |

## Deviations from Plan

- **`go` directive bumped to 1.25.0**: `modernc.org/sqlite` v1.50.1 / its deps require Go ≥ 1.25, so `go get` raised the module's `go` directive. Consequently the Dockerfile build image was bumped `golang:1.24-alpine` → `golang:1.25-alpine` (the plan only called for the `go.sum` copy). The toolchain was auto-fetched locally; build/test/lint all pass.
- **`govulncheck` could not run**: the sandbox network policy blocks `vuln.go.dev` (HTTP 403). Deps are mainstream, maintained libraries (`modernc.org/sqlite`, `golang-migrate`, `google/uuid`); recommend re-running `govulncheck ./...` in an environment with network access.
- **Docker image build not run**: the Docker daemon is not running in this sandbox. Validated the equivalent `CGO_ENABLED=0 GOOS=linux go build` instead (static binary builds clean), which is what the Dockerfile runs.
- **Health handler shape**: implemented as `Health(db pinger) http.HandlerFunc` (a small `pinger` interface) rather than a bare handler func, so the readiness probe is unit-testable with a fake; `*sql.DB` satisfies it. Router test uses an in-memory DB via `repository.Open(":memory:")`.

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/repository/db_test.go` | `newTestStore` helper (temp-dir DB + migrations) |
| `internal/repository/household_test.go` | `TestHouseholdCRUD`, `TestHouseholdNotFound` |
| `internal/repository/recipe_test.go` | `TestRecipeCRUD` (incl. feedback round-trip), `TestRecipeCascadeOnHouseholdDelete` |
| `internal/repository/weeklyplan_test.go` | `TestWeeklyPlanCRUDWithItems` (txn + item load + cascade), `TestWeeklyPlanCascadeOnHouseholdDelete` |
| `internal/handler/health_test.go` | `TestHealth_OK`, `TestHealth_Unavailable` (503), `TestRouter_HealthzRoute` |
