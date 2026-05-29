# Implementation Report

**Plan**: `.agents/plans/completed/ch-13-shopping-list-ui.plan.md`
**Branch**: `claude/charming-thompson-kc5qu`
**Status**: COMPLETE

## Summary

Implemented the CH-13 shopping-list screen: a server-rendered page (reachable from
the home screen and the header nav) that groups the active weekly plan's
consolidated items under the 6 store categories in canonical order, with localized
headings. Each item has an HTMX checkbox that persists its `checked` state to the
DB and a remove button that soft-removes the line (`manually_removed`) with an
inline undo. A client-side "show purchased" toggle hides/reveals checked items.

All write endpoints set an **absolute** state (not a toggle), so a Service-Worker
replay of an offline write is idempotent and safe. No migration was needed — the
`checked` and `manually_removed` columns already existed in `000001_init.up.sql`.
The screen reads the active plan directly from the repository and is wired
unconditionally, so it works even when no LLM client is configured.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Repository: item read + idempotent state writes | `internal/repository/shopping_item.go` | ✅ |
| 2 | Repository tests (idempotency, ErrNotFound) | `internal/repository/shopping_item_test.go` | ✅ |
| 3 | Handler: view models, grouping, list page | `internal/handler/shopping.go` | ✅ |
| 4 | Handler: check / remove / restore endpoints | `internal/handler/shopping.go` | ✅ |
| 5 | Templates (page/content/item/removed partials) | `templates/shopping.gohtml` | ✅ |
| 6 | CSS + "show purchased" JS toggle | `static/css/app.css`, `static/js/shopping-filter.js` | ✅ |
| 7 | Wire routes + nav entry points | `router.go`, `base.gohtml`, `home.gohtml`, `home.go` | ✅ |
| 8 | i18n keys (en/fi/ru) | `i18n/{en,fi,ru}.json` | ✅ |
| 9 | Handler tests (stub) | `internal/handler/shopping_test.go` | ✅ |
| 10 | Full validation + HTTP-level E2E | `internal/handler/shopping_integration_test.go` | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ |
| `golangci-lint run ./...` | ✅ 0 issues |
| `go test ./...` | ✅ all packages pass (verified on go1.26.3) |
| Static `CGO_ENABLED=0` build | ✅ |
| Live binary HTTP smoke (`/shopping`, `/`, `/healthz`) | ✅ |
| `govulncheck ./...` | ✅ clean — `No vulnerabilities found` (locally, on go1.26.3 after the toolchain bump) |

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/repository/shopping_item.go` | CREATE | `GetShoppingItem`, `SetShoppingItemChecked`, `SetShoppingItemRemoved` |
| `internal/repository/shopping_item_test.go` | CREATE | idempotent double-apply + ErrNotFound |
| `internal/handler/shopping.go` | CREATE | handlers, view models, category grouping |
| `internal/handler/shopping_test.go` | CREATE | stub-based handler tests |
| `internal/handler/shopping_integration_test.go` | CREATE | full-router + real-SQLite E2E |
| `templates/shopping.gohtml` | CREATE | page/content/item/removed |
| `static/js/shopping-filter.js` | CREATE | show-purchased toggle |
| `static/css/app.css` | UPDATE | `.shopping-*` + header nav styles |
| `internal/handler/router.go` | UPDATE | `/shopping` + item routes (unconditional) |
| `internal/handler/home.go` | UPDATE | dropped unused `CategoryKeys` field |
| `templates/base.gohtml` | UPDATE | header nav shopping link |
| `templates/home.gohtml` | UPDATE | replaced static category list with shopping link |
| `internal/handler/language_test.go` | UPDATE | repointed home assertions to the shopping link |
| `i18n/en.json`, `i18n/fi.json`, `i18n/ru.json` | UPDATE | `nav.shopping` + `shopping.*` keys |
| `go.mod` | UPDATE | pin `toolchain go1.26.3` (govulncheck remediation) |
| `Dockerfile` | UPDATE | build stage → `golang:1.26.3-alpine` |

## Deviations from Plan

1. **Removed the static category list from the home page** (the plan said "add a link").
   That placeholder list (CH-12 era) became redundant once a real shopping screen
   exists, so it was replaced with a link to `/shopping`. This required repointing
   three existing home language-resolution tests (`language_test.go`) from category
   strings to the localized shopping-list link — the language-resolution intent of
   those tests is preserved.
2. **"Show purchased" default = hide purchased.** The toggle defaults to OFF, hiding
   checked items so the household sees only what is left to buy; turning it on reveals
   purchased lines. This is the common grocery-app pattern and matches the
   `shopping.show_purchased` label. (Either default was defensible; this one chosen.)
3. **No new service layer** — handler depends on narrow repository interfaces
   directly, mirroring `recipeHandlers` (as the plan anticipated).

## Deferred / Out of Scope (recorded for CH-21 gate)

| Item | Why | Where verified |
|------|-----|----------------|
| Service-Worker offline replay of checkbox writes | `static/sw.js` is GET-only; no POST queue/Background-Sync built | Future SW work; tailnet HTTPS / CH-21. Server contract (idempotent writes) is delivered and tested. |
| Real-browser HTMX swap / touch-target smoke | needs running server + Safari | tailnet HTTPS / Mac mini |

### govulncheck — resolved (toolchain bump)

A local `govulncheck ./...` (toolchain go1.26.1) surfaced **9 vulnerabilities, all
in the Go standard library** (`html/template`, `crypto/tls`, `crypto/x509`, `net`,
`net/http`) — advisories GO-2026-4865/4866/4870/4918/4946/4947/4971/4980/4982. None
are in CH-13 code or in any third-party dependency our code calls (govulncheck:
*"your code doesn't appear to call these"* for the imported/required-module findings).
Not introduced by CH-13 — they affect the whole project equally and are a function of
the build toolchain version.

**Remediation (applied):** all nine are fixed in **go1.26.3**, so the build toolchain
is now pinned:
- `go.mod` — added `toolchain go1.26.3`
- `Dockerfile` — build stage bumped to `golang:1.26.3-alpine`

Verified in-sandbox: `go build ./...`, `go vet ./...`, `gofmt -s -l .`, and
`go test ./...` all pass on go1.26.3. **Confirmed locally: `govulncheck ./...` reports
`No vulnerabilities found`** on go1.26.3.

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/repository/shopping_item_test.go` | `TestGetShoppingItem`, `TestSetShoppingItemCheckedIdempotent`, `TestSetShoppingItemRemovedIdempotent` |
| `internal/handler/shopping_test.go` | grouping/order, HTMX fragment, empty state, 500-no-leak, check persist + idempotent + uncheck, benign missing-item check, remove→restore, all-three-languages |
| `internal/handler/shopping_integration_test.go` | `TestShoppingEndToEnd` (list → check → persisted → remove → gone → restore → back through the real router + SQLite) |
