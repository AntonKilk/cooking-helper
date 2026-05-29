# Implementation Report

**Plan**: `.agents/plans/completed/ch-15-disliked-ingredients-management.plan.md`
**Branch**: `claude/intelligent-gates-7EaQq`
**GitHub Issue**: #15 (CH-15)
**Status**: COMPLETE

## Summary

Added the disliked-ingredients management screen (F-7 / CH-15) — a CRUD/UI surface
over data that already existed. `HouseholdProfile.DislikedIngredients` was already
persisted, already injected into every generation/swap prompt as a hard constraint,
and already post-validated (CH-10), so no migration or prompt change was needed. The
work delivers: two household-service methods (`AddDisliked` / `RemoveDisliked`), a
`dislikedHandlers` set (show / add / remove) rendering HTMX fragments, a
`disliked.gohtml` template with native `<datalist>` autosuggest sourced from recent
recipe ingredients, a Settings hub link, and ru/fi/en strings.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Service AddDisliked/RemoveDisliked + ErrEmptyIngredient | `internal/service/household.go` | ✅ |
| 2 | Service unit tests | `internal/service/household_test.go` | ✅ |
| 3 | Disliked handlers + suggestion projection | `internal/handler/disliked.go` | ✅ |
| 4 | Disliked template (page/content/list) | `templates/disliked.gohtml` | ✅ |
| 5 | Handler tests (show/add/blank/remove) | `internal/handler/disliked_test.go` | ✅ |
| 6 | Route wiring | `internal/handler/router.go` | ✅ |
| 7 | Settings hub link | `templates/settings.gohtml` | ✅ |
| 8 | Settings test asserts link | `internal/handler/settings_test.go` | ✅ |
| 9 | i18n strings (en/ru/fi) | `i18n/{en,ru,fi}.json` | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ |
| `golangci-lint run ./...` (v2.12.2, rebuilt with go1.26.3) | ✅ 0 issues |
| `go test ./...` | ✅ all packages pass |
| `CGO_ENABLED=0 go build ./cmd/server` | ✅ |
| E2E (real binary + SQLite over HTTP) | ✅ 7/7 steps |
| `govulncheck ./...` | ⏭ deferred — vuln.go.dev 403 in sandbox; no new deps; gated at CH-21 |

### E2E steps verified (real `/tmp/server` + modernc SQLite)

1. `GET /settings` contains `href="/settings/disliked"` ✅
2. `GET /settings/disliked` renders heading, `<datalist>`, empty state ✅
3. `POST /settings/disliked` (HX-Request) returns `disliked/list` fragment with the new item + remove button ✅
4. `POST /settings/disliked` with blank ingredient → HTTP 400 + localized "Enter an ingredient name." ✅
5. `GET /settings/disliked` shows the added term persisted across requests ✅
6. `POST /settings/disliked/remove` returns the empty-state fragment ✅
7. Empty list is valid after removal ✅

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `internal/service/household.go` | UPDATE | +56 |
| `internal/service/household_test.go` | UPDATE | +84 |
| `internal/handler/disliked.go` | CREATE | +162 |
| `internal/handler/disliked_test.go` | CREATE | +186 |
| `internal/handler/router.go` | UPDATE | +4 |
| `internal/handler/settings_test.go` | UPDATE | +3 |
| `templates/disliked.gohtml` | CREATE | +57 |
| `templates/settings.gohtml` | UPDATE | +5 |
| `i18n/en.json` | UPDATE | +8 |
| `i18n/ru.json` | UPDATE | +8 |
| `i18n/fi.json` | UPDATE | +8 |

## Deviations from Plan

- **golangci-lint toolchain workaround (environment).** The repo pins the Go
  toolchain to go1.26.3 (commit 3165415). A stock `go install ...golangci-lint@latest`
  produced a binary built with an older Go that refused the config
  ("Go language version used to build golangci-lint is lower than the targeted Go
  version 1.26.3"), and the `@latest` tag resolved to the v1 module while the repo's
  `.golangci.yml` is v2 format. Resolved by installing the **v2** module path
  (`github.com/golangci/golangci-lint/v2/cmd/golangci-lint`) under
  `GOTOOLCHAIN=go1.26.3`. Lint then ran clean (0 issues). No code change.
- Everything else matched the plan.

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/service/household_test.go` | `TestAddDisliked` (new term trimmed+persisted; case-insensitive duplicate no-op; blank → ErrEmptyIngredient), `TestRemoveDisliked` (absent no-op; case-insensitive removal leaves empty list, persisted) |
| `internal/handler/disliked_test.go` | `TestDislikedShowRendersListAndSuggestions` (existing term, datalist, history suggestion present, already-disliked excluded), `TestDislikedAddPersistsAndRendersList` (AddDisliked called, list fragment returned), `TestDislikedAddBlankRejected` (400 + localized error), `TestDislikedRemoveRendersList` (RemoveDisliked called, term gone) |
| `internal/handler/settings_test.go` | extended to assert `/settings/disliked` link |
