# Implementation Report

**Plan**: `.agents/plans/completed/ch-14-pantry-basics.plan.md`
**Branch**: `claude/awesome-maxwell-QI8s8`
**Status**: COMPLETE

## Summary

Implemented CH-14 (Pantry Basics Management, F-6): a settings-reachable screen to edit
the household's "always at home" staples so they stay off the shopping list. The
persistence layer (`HouseholdProfile.PantryBasics`) and the shopping-list exclusion
(`ShoppingBuilder.Build` → `shopping.Consolidate`) already existed, so this work added a
localized default list, add/remove service operations, an HTMX handler path, the screen
template, the settings link, and i18n keys. **No DB migration** — the `pantry_basics`
column already exists.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Localized default pantry list | `internal/domain/household.go` | ✅ |
| 2 | Seed defaults + add/remove service ops + `ErrEmptyIngredient` | `internal/service/household.go` | ✅ |
| 3 | Service tests (seed, add/dedupe/empty, remove) | `internal/service/household_test.go` | ✅ |
| 4 | Extend shared `householdProfiles` interface | `internal/handler/profile.go` | ✅ |
| 5 | Pantry handlers (Show/Add/Remove) | `internal/handler/pantry.go` | ✅ |
| 6 | Pantry templates (page/content/list fragment) | `templates/pantry.gohtml` | ✅ |
| 7 | Settings link | `templates/settings.gohtml` | ✅ |
| 8 | i18n keys (en/ru/fi) | `i18n/{en,ru,fi}.json` | ✅ |
| 9 | Wire router + extend stubs + handler tests + E2E | `router.go`, `*_test.go`, `pantry_integration_test.go` | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ clean |
| `golangci-lint run ./...` | ⚠️ Could not run in sandbox (installed binary built with go1.25 < targeted go1.26.3). Deferred to a networked host / CH-21. `gofmt` + `go vet` pass. |
| `go test ./...` | ✅ all packages pass |
| Static build (`CGO_ENABLED=0`) | ✅ OK |
| HTTP-level E2E (`TestPantryEndToEnd`) | ✅ pass (real router + migrated SQLite + service + repo + templates + i18n) |
| `govulncheck ./...` | n/a — no new dependencies added |

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `internal/domain/household.go` | UPDATE | +16 |
| `internal/service/household.go` | UPDATE | +64/-2 |
| `internal/service/household_test.go` | UPDATE | +97 |
| `internal/handler/profile.go` | UPDATE | +2 |
| `internal/handler/profile_test.go` | UPDATE | +16 |
| `internal/handler/generate_test.go` | UPDATE | +8 |
| `internal/handler/router.go` | UPDATE | +4 |
| `internal/handler/pantry.go` | CREATE | new |
| `internal/handler/pantry_test.go` | CREATE | new |
| `internal/handler/pantry_integration_test.go` | CREATE | new |
| `templates/pantry.gohtml` | CREATE | new |
| `templates/settings.gohtml` | UPDATE | +5 |
| `i18n/en.json` | UPDATE | +9 |
| `i18n/ru.json` | UPDATE | +9 |
| `i18n/fi.json` | UPDATE | +9 |

## Deviations from Plan

- **Extra file updated**: `internal/handler/generate_test.go` — it contains a second
  interface stub (`stubHouseholds`) that also implements `householdProfiles`; it needed
  the two new methods to keep the package compiling. Not anticipated in the plan's file
  list but a direct consequence of the (planned) interface widening.
- **Added an integration E2E test** (`pantry_integration_test.go`) beyond the unit tests
  the plan listed, mirroring the existing `TestShoppingEndToEnd` pattern, to satisfy the
  E2E gate through the real router + migrated SQLite store.

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/service/household_test.go` | `TestCurrentCreatesDefaults` (extended: seeded pantry defaults), `TestAddPantryBasicAppendsAndDedupes`, `TestAddPantryBasicRejectsEmpty`, `TestRemovePantryBasicCaseInsensitive` |
| `internal/handler/pantry_test.go` | `TestPantryShowRendersItems`, `TestPantryAddAppendsItem`, `TestPantryAddEmptyRejected`, `TestPantryRemoveDropsItem` |
| `internal/handler/pantry_integration_test.go` | `TestPantryEndToEnd` (show seeds defaults → add → persist → dedupe → remove → persist) |
