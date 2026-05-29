# Implementation Report

**Plan**: `.agents/plans/ch-18-recipe-archive.plan.md`
**Branch**: `claude/magical-heisenberg-mQhdD`
**Status**: COMPLETE

## Summary

Implemented the recipe archive (CH-18 / F-8): a main-menu-reachable screen listing
every household recipe newest-first with read-only feedback icons, a debounced
(~200 ms) HTMX substring title search, and a "Cook again" action that replays an
archived recipe into the current weekly plan — the user picks which of the three to
replace via a dialog — and rebuilds the shopping list. Listing and search are pure
repository reads available unconditionally; "cook again" reuses the existing atomic
`SwapRecipeInPlan` transaction plus `ShoppingBuilder`, so it is wired only when the LLM
is configured (the same `canGenerate` gate the per-card swap uses). Archive read
failures degrade to a friendly in-page banner instead of a whole-page 500.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | `SearchRecipes` + `escapeLike` | `internal/repository/recipe.go` | ✅ |
| 2 | Search repository tests | `internal/repository/recipe_test.go` | ✅ |
| 3 | `CookAgain` service method + `GetRecipe` on `generationRepo` | `internal/service/generation.go` | ✅ |
| 4 | `CookAgain` tests + fake `GetRecipe` | `internal/service/generation_test.go` | ✅ |
| 5 | Archive handlers (Show/Search/CookAgainDialog/CookAgain) | `internal/handler/archive.go` | ✅ |
| 6 | Archive handler tests | `internal/handler/archive_test.go` | ✅ |
| 7 | Archive templates (page/content/list/dialog/done/error) | `templates/archive.gohtml` | ✅ |
| 8 | Main-menu nav link | `templates/base.gohtml` | ✅ |
| 9 | Router wiring (always-on list/search; gated cook-again) | `internal/handler/router.go` | ✅ |
| 10 | i18n keys (en/fi/ru) | `i18n/{en,fi,ru}.json` | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ clean |
| `golangci-lint run ./...` (v2 under go1.26.3) | ✅ 0 issues |
| `go test ./...` | ✅ all packages pass |
| `CGO_ENABLED=0 go build ./cmd/server` | ✅ |
| `govulncheck ./...` | ⏸ deferred (vuln.go.dev 403 in sandbox) → CH-21 |

## End-to-End Verification (live)

Ran the real binary against a temp SQLite DB with the OpenAI provider wired:

- `GET /healthz` → 200; `GET /archive` empty state renders ✅
- `app-header__archive` nav link present on the home page ✅
- `GET /archive/search?q=…` returns the `#archive-list` fragment ✅
- `POST /generate` (live OpenAI) created a week; `GET /archive` then listed all 3 recipes ✅
- `GET /archive/cook-again/{id}` dialog listed the active week's 3 recipes ✅
- `POST /archive/cook-again/{id}` with `old=…` → "Added to this week: …" confirmation ✅
- Shopping list rebuilt with items after cook-again ✅

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `internal/repository/recipe.go` | UPDATE | +45 |
| `internal/repository/recipe_test.go` | UPDATE | +92 |
| `internal/service/generation.go` | UPDATE | +63 |
| `internal/service/generation_test.go` | UPDATE | +89 |
| `internal/handler/archive.go` | CREATE | +~250 |
| `internal/handler/archive_test.go` | CREATE | +~250 |
| `templates/archive.gohtml` | CREATE | +~95 |
| `templates/base.gohtml` | UPDATE | +1 |
| `internal/handler/router.go` | UPDATE | +11/-1 |
| `i18n/en.json` / `fi.json` / `ru.json` | UPDATE | +11 each |

## Deviations from Plan

- **Dialog choices use a hidden-input form, not `hx-vals`** — the plan's snippet showed
  `hx-vals`, but CLAUDE.md mandates the hidden-input + form-post idiom (no `hx-vals`).
  Implemented per the rule; documented in the template comment.
- **`CookAgain` is wired (and exercised live) in this session** because an `OPENAI_API_KEY`
  is present, so `canGenerate=true`. The gating logic is unchanged; the route is simply
  active here, which let the live E2E cover the full path.

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/repository/recipe_test.go` | substring+order+limit; LIKE-wildcard escaping; household scoping |
| `internal/service/generation_test.go` | CookAgain happy path (history copy, builder+swap, in-place rotation); unknown old ID; source not found |
| `internal/handler/archive_test.go` | list newest-first + icons + cook control; cook hidden when disabled; degradation banner on read error; search fragment; dialog no-plan; dialog lists current recipes; cook-again success; service error friendly; POST no-plan |
