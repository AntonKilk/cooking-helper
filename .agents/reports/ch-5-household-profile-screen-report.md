# Implementation Report

**Plan**: `.agents/plans/completed/ch-5-household-profile-screen.plan.md`
**Branch**: `claude/prime-5-ZV2eL`
**Status**: COMPLETE

## Summary

Implemented the CH-5 household profile screen. Users reach a profile form from settings,
set family composition (adults 1–6, kids 0–6) and UI language, and the values persist to
SQLite via the CH-3 repository. On first access the singleton household is auto-created
with defaults (2 adults, 0 kids, language from `Accept-Language`). This introduced the
project's first service layer (`HouseholdService`) between handlers and the repository,
plus a `FirstHousehold` repository query.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Add `FirstHousehold` + shared `scanHousehold` helper | `internal/repository/household.go` | ✅ |
| 2 | Test `FirstHousehold` | `internal/repository/household_test.go` | ✅ |
| 3 | Create `HouseholdService` (`Current`, `UpdateProfile`) | `internal/service/household.go` | ✅ |
| 4 | Service tests (defaults, idempotency, range validation) | `internal/service/household_test.go` | ✅ |
| 5 | Profile i18n strings (en/fi/ru) | `i18n/{en,fi,ru}.json` | ✅ |
| 6 | Profile template | `templates/profile.gohtml` | ✅ |
| 7 | Profile handlers (`Show`, `Save`) | `internal/handler/profile.go` | ✅ |
| 8 | Wire `GET`/`POST /settings/profile` | `internal/handler/router.go` | ✅ |
| 9 | Link profile from settings | `templates/layout.gohtml` | ✅ |
| 10 | Handler tests (stub-based) | `internal/handler/profile_test.go` | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ |
| `golangci-lint run ./...` | ✅ 0 issues |
| `go test ./...` | ✅ all packages pass |

## End-to-End Verification

Ran the built server against a temp SQLite DB:

1. First access (`Accept-Language: ru`) → form shows defaults adults=2, kids=0, `ru` radio checked, title "Профиль семьи". ✅
2. `POST adults=4&kids=3&lang=fi` → 303 redirect to `/settings/profile`, `lang=fi` cookie set. ✅
3. Reload with cookie → persisted adults=4, kids=3, `fi` radio checked, heading "Talouden profiili" (proves SQLite persistence). ✅
4. `POST adults=9` → HTTP 400 with localized error "Aikuisia on oltava 1–6 ja lapsia 0–6.", no persistence. ✅
5. Home page renders the `/settings/profile` settings link. ✅

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `internal/repository/household.go` | UPDATE | +43/-11 |
| `internal/repository/household_test.go` | UPDATE | +28 |
| `internal/service/household.go` | CREATE | +96 |
| `internal/service/household_test.go` | CREATE | +160 |
| `internal/handler/profile.go` | CREATE | +118 |
| `internal/handler/profile_test.go` | CREATE | +150 |
| `internal/handler/render.go` | UPDATE | +11/-2 |
| `internal/handler/router.go` | UPDATE | +6 |
| `templates/profile.gohtml` | CREATE | +41 |
| `templates/layout.gohtml` | UPDATE | +1 |
| `i18n/en.json` | UPDATE | +7 |
| `i18n/fi.json` | UPDATE | +7 |
| `i18n/ru.json` | UPDATE | +7 |

## Deviations from Plan

- **`renderer.renderStatus` added** (not in the plan). `render` previously set the
  Content-Type after the response was committed, so a 400 re-render would have lost the
  header. Factored render into a status-aware `renderStatus`; `render` delegates with 200,
  and `renderInvalid` uses 400. Keeps a single buffer-first render path.
- **`minAdults`/`maxAdults`/`minKids`/`maxKids` duplicated in the handler package** as
  planned (handler boundary validation), matching the service constants. Deliberate
  defense-in-depth, not shared to avoid a handler→service constant dependency.

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/repository/household_test.go` | `TestFirstHousehold` (empty → ErrNotFound; after create → oldest row) |
| `internal/service/household_test.go` | `TestCurrentCreatesDefaults`, `TestCurrentIsIdempotent`, `TestUpdateProfilePersists`, `TestUpdateProfileRejectsOutOfRange` (3 sub-cases) |
| `internal/handler/profile_test.go` | `TestProfileShowRendersCurrentValues`, `TestProfileSaveValidPersistsAndRedirects`, `TestProfileSaveOutOfRangeRejected` (3 sub-cases) |
