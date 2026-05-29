# Implementation Report

**Plan**: `.agents/plans/completed/ch-12-shopping-list-builder.plan.md`
**Branch**: `claude/prime-12-6y9Jh`
**Status**: COMPLETE

## Summary

CH-12 builds the consolidated, store-categorized shopping list automatically when
a `WeeklyPlan` is created, and rebuilds it after a recipe swap. Pure logic lives
in `internal/shopping`: ingredient consolidation (sum compatible units, split
incompatible ones), unit normalization (mass/volume/count families incl. FI/RU
units), pantry-basics exclusion, and a multilingual ingredient→category
dictionary. The LLM (Haiku/`gpt-5.4-nano` via `RoleCategorize`) resolves
dictionary misses, orchestrated by a new `service.ShoppingBuilder` with a global,
name-keyed DB cache so the model is never asked twice. Categorization never fails
the build — any cache/LLM error degrades the line to `other`.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Unit normalization (families, conversion, display) | `internal/shopping/units.go` | ✅ |
| 2 | Unit tests | `internal/shopping/units_test.go` | ✅ |
| 3 | Ingredient→category dictionary | `internal/shopping/categories.go` | ✅ |
| 4 | Dictionary coverage test (5-week corpus) | `internal/shopping/categories_test.go` | ✅ |
| 5 | Consolidation (sum/split/pantry/dictionary) | `internal/shopping/consolidate.go` | ✅ |
| 6 | Consolidation tests | `internal/shopping/consolidate_test.go` | ✅ |
| 7 | Category cache migration + repository | `migrations/000003_*`, `internal/repository/ingredient_category.go` | ✅ |
| 8 | ShoppingBuilder (consolidate→cache→LLM) | `internal/service/shopping.go` | ✅ |
| 9 | Builder tests (incl. ≥95% accuracy gate) | `internal/service/shopping_test.go` | ✅ |
| 10 | Wire builder into GenerateWeek | `internal/service/generation.go` | ✅ |
| 11 | Rebuild list on swap | `internal/service/generation.go`, `internal/repository/weeklyplan.go` | ✅ |
| 12 | Update tests + wiring | `*_test.go`, `internal/handler/router.go` | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ clean |
| `golangci-lint run ./...` | ✅ 0 issues |
| `go test ./...` | ✅ all packages pass |
| `CGO_ENABLED=0 go build` | ✅ ok |
| Live LLM E2E (OpenAI) | ✅ ran in-session (OPENAI_API_KEY present) |

Metrics: dictionary coverage 29/30 on the corpus; full pipeline categorization
accuracy 32/32 (100% ≥ 95% AC). Live builder E2E confirmed consolidation
(carrot 250 g + 100 g → 350 g), dictionary placement (chicken→meat_fish),
LLM fallback (halloumi→dairy, soy sauce→pantry), and pantry exclusion (salt).

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/shopping/units.go` | CREATE | unit families + conversion + display |
| `internal/shopping/units_test.go` | CREATE | classify + display tables |
| `internal/shopping/categories.go` | CREATE | EN/FI/RU dictionary |
| `internal/shopping/categories_test.go` | CREATE | no-false-hits + specific cases |
| `internal/shopping/consolidate.go` | CREATE | `Consolidate(recipes, pantry)` |
| `internal/shopping/consolidate_test.go` | CREATE | sum/split/pantry/order |
| `migrations/000003_ingredient_category.up.sql` / `.down.sql` | CREATE | global cache table |
| `internal/repository/ingredient_category.go` | CREATE | `CategoriesByNames` / `SaveCategory` |
| `internal/repository/ingredient_category_test.go` | CREATE | round-trip + idempotency |
| `internal/service/shopping.go` | CREATE | `ShoppingBuilder` orchestration |
| `internal/service/shopping_test.go` | CREATE | dict/cache/LLM/error + accuracy |
| `internal/service/shopping_integration_test.go` | CREATE | skip-guarded live E2E |
| `internal/service/generation.go` | UPDATE | builder dep; build on generate + swap |
| `internal/service/generation_test.go` | UPDATE | fakeBuilder, helper, new signatures |
| `internal/repository/weeklyplan.go` | UPDATE | `SwapRecipeInPlan` replaces items; shared insert helper |
| `internal/repository/weeklyplan_test.go` | UPDATE | new signature, rebuilt-list assertions |
| `internal/handler/router.go` | UPDATE | wire `NewShoppingBuilder` |

## Deviations from Plan

- **Wiring site**: the plan said `cmd/server/main.go`; the `GenerationService` is
  actually constructed in `internal/handler/router.go`, so the builder was wired
  there instead.
- **Categorize prompt rendering**: used a literal `strings.ReplaceAll` for the
  `{{ingredient}}` slot rather than `text/template` — the prompt's bare
  `{{ingredient}}` marker is not valid Go template syntax, and a string replace
  avoids changing the versioned prompt file.
- **5-week fixture not shared across packages**: Go test files aren't importable,
  so the corpus is defined locally in both the shopping dictionary test and the
  service accuracy test rather than exported once.
- **Global cache table** keyed by name only (no `household_id`) — confirmed with
  the owner; rationale documented in the migration.

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/shopping/units_test.go` | `TestClassifyUnit`, `TestDisplayAmount` |
| `internal/shopping/categories_test.go` | `TestLookupCategoryNoFalseHits`, `TestLookupCategorySpecificCases` |
| `internal/shopping/consolidate_test.go` | sum, convert-within-family, incompatible-split, opaque-merge, pantry-exclude, unknown-empty, deterministic-order |
| `internal/repository/ingredient_category_test.go` | round-trip, empty, idempotent |
| `internal/service/shopping_test.go` | dictionary-only (no LLM), cache-hit, LLM-fallback+cache, LLM-error→other, cache-read-error degrade, ≥95% accuracy |
| `internal/service/shopping_integration_test.go` | live OpenAI builder E2E (skip-guarded) |
| `internal/service/generation_test.go` | `TestGenerateWeekAttachesShoppingList`, `TestGenerateWeekBuilderErrorFails`, `TestSwapRecipeRebuildsShoppingList` |
| `internal/repository/weeklyplan_test.go` | swap now asserts the rebuilt list replaces old items |
