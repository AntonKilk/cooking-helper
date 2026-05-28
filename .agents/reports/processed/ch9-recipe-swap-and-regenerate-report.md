# Implementation Report: CH-9 Recipe Swap & Full Regenerate

**Plan**: `.agents/plans/ch9-recipe-swap-and-regenerate.plan.md`
**Branch**: `claude/beautiful-shannon-CJNN1`
**GitHub Issue**: #9
**Status**: COMPLETE

## Summary

Added per-card recipe swap and full-plan regenerate on top of CH-8's one-tap generation:

- **Schema**: new nullable `archived_at` column on `weekly_plan` (migration 000002) plus a partial index `WHERE archived_at IS NULL` for fast active-plan lookup.
- **Repository**: `CurrentWeeklyPlan`, `ArchiveAndCreateWeek` (one tx — archive prev + create new), `SwapRecipeInPlan` (one tx — insert recipe, rotate `recipe_ids`, clear shopping items), `RecipesByIDs`.
- **Service**: `GenerateWeek` now archives the previous active plan before creating the new one; new `SwapRecipe` orchestrates kept-recipe loading, the swap LLM prompt, dislike-retry, portion/variety validation, and the atomic write.
- **LLM**: new `swap_recipe.v1.txt` prompt with system/trigger split mirroring `generate_week.v1.txt`. Few-shot Finnish examples are NOT appended (the swap contract is tight enough; reassess at CH-21 on live runs).
- **Handler**: `POST /generate/swap/{recipeID}` re-renders the 3-card fragment after swap; kept cards' protein emoji is inferred from ingredient names (CH-8 didn't store protein).
- **Templates**: per-card "Replace" button + "Regenerate all" button below the cards, both via HTMX targeting `#week`.
- **i18n**: `recipe.replace` and `home.regenerate_all` added in EN/RU/FI; reused existing `generate.error*` keys for the error fragment.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Migration: `archived_at` column + partial index | `migrations/000002_weekly_plan_archive.{up,down}.sql` | ✅ |
| 2 | Domain: `ArchivedAt *time.Time` | `internal/domain/plan.go` | ✅ |
| 3 | Repository: persist/scan `archived_at` (shared `scanPlanRow` + `planColumns`) | `internal/repository/weeklyplan.go` | ✅ |
| 4 | Repository: `CurrentWeeklyPlan` | `internal/repository/weeklyplan.go` | ✅ |
| 5 | Repository: `ArchiveAndCreateWeek` (one tx) | `internal/repository/weeklyplan.go` | ✅ |
| 6 | Repository: `SwapRecipeInPlan` (one tx) | `internal/repository/weeklyplan.go` | ✅ |
| 7 | Repository: `RecipesByIDs` (order-preserving) | `internal/repository/recipe.go` | ✅ |
| 8 | LLM prompt: `swap_recipe.v1.txt` | `internal/llm/prompts/swap_recipe.v1.txt` | ✅ |
| 9 | DTO: `generatedSwap` | `internal/service/generation_dto.go` | ✅ |
| 10 | Service: archive-previous in `GenerateWeek` | `internal/service/generation.go` | ✅ |
| 11 | Service: `SwapRecipe` + helpers (`mapRecipe`, `inferProtein`, `formatKept`, `swapHasProteinVariety`) | `internal/service/generation.go` | ✅ |
| 12 | Handler: `Swap` + `recipeLoader` interface + kept-emoji inference | `internal/handler/generate.go` | ✅ |
| 13 | Router: register `POST /generate/swap/{recipeID}` | `internal/handler/router.go` | ✅ |
| 14 | Template: per-card "Replace" + "Regenerate all" buttons | `templates/generate.gohtml` | ✅ |
| 15 | i18n: new keys in en/ru/fi | `i18n/en.json`, `i18n/ru.json`, `i18n/fi.json` | ✅ |
| 16 | Final validation pass | — | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ |
| `golangci-lint run ./...` | ✅ 0 issues |
| `go test ./...` | ✅ all packages pass |
| `CGO_ENABLED=0 go build ./cmd/server` | ✅ |
| Boot smoke test (server starts, `/healthz` 200, `/generate/swap/{id}` route registered and friendly-errors on no active plan) | ✅ |

| Check | Result |
|-------|--------|
| Live LLM swap / regenerate calls | ⏸ Deferred to CH-21 (no API key, OpenAI host blocked in sandbox per CLAUDE.md) |
| `govulncheck ./...` | ⏸ Deferred (vuln.go.dev 403 in sandbox; no new deps were added) |
| Service-Worker / PWA over tailnet HTTPS | ⏸ Deferred to CH-21 |

## Files Changed

| File | Action | Lines (insertions / deletions) |
|------|--------|--------------------------------|
| `migrations/000002_weekly_plan_archive.up.sql` | CREATE | +12 |
| `migrations/000002_weekly_plan_archive.down.sql` | CREATE | +5 |
| `internal/domain/plan.go` | UPDATE | +4 / -1 |
| `internal/handler/generate.go` | UPDATE | +127 |
| `internal/handler/generate_test.go` | UPDATE | +181 / -10 |
| `internal/handler/router.go` | UPDATE | +7 / -1 |
| `internal/repository/recipe.go` | UPDATE | +51 |
| `internal/repository/recipe_test.go` | UPDATE | +54 |
| `internal/repository/weeklyplan.go` | UPDATE | +146 / -30 |
| `internal/repository/weeklyplan_test.go` | UPDATE | +179 |
| `internal/service/generation.go` | UPDATE | +349 / -55 |
| `internal/service/generation_dto.go` | UPDATE | +5 |
| `internal/service/generation_test.go` | UPDATE | +253 / -10 |
| `templates/generate.gohtml` | UPDATE | +11 |
| `internal/llm/prompts/swap_recipe.v1.txt` | CREATE | +56 |
| `i18n/en.json`, `i18n/ru.json`, `i18n/fi.json` | UPDATE | +2 each |

Plan archived to `.agents/plans/completed/`.

## Deviations from Plan

| Plan said | Actually did | Why |
|-----------|--------------|-----|
| `weeklyGenerator` interface gains a `CurrentPlan` method | Same; named `CurrentPlan` on the service (returns `nil, nil` when no active plan, instead of forcing the handler to import `repository.ErrNotFound`) | Keeps the handler layer free of repository-error imports — the boundary contract is "nil = no plan". |
| Plan suggested the down migration use rename-and-copy for safety | Used `ALTER TABLE … DROP COLUMN` + `DROP INDEX IF EXISTS` | `modernc.org/sqlite v1.50.1` bundles a SQLite ≥ 3.45, which supports `DROP COLUMN` natively. Documented in the SQL comment. |
| Plan defined `containsAny` once (service) | Defined in both service AND handler (`internal/handler/generate.go`) | Cross-package dependency would have moved `inferProtein` into a shared helper package; for two ~10-line helpers, duplication is the smaller cost. Both copies cover the same EN/FI/RU keyword set. |
| Plan mentioned an optional `home.working` i18n key for a shared indicator label | Skipped — kept the existing `home.generating` indicator | Per the plan's own "no-op alternative". The current "Cooking up your week…" label reads naturally for both generate and swap; introducing a new key would have churned three locales for no functional gain. |
| Plan suggested also testing a "rollback leaves previous plan unarchived" path | Implemented and passing as `TestArchiveAndCreateWeekAtomicity` | — |

## Tests Written

| Test File | New Test Cases |
|-----------|----------------|
| `internal/repository/recipe_test.go` | `TestRecipesByIDsPreservesOrder`, `TestRecipesByIDsMissingReturnsNotFound`, `TestRecipesByIDsEmpty` |
| `internal/repository/weeklyplan_test.go` | `TestCurrentWeeklyPlanIgnoresArchived`, `TestCurrentWeeklyPlanNotFound`, `TestArchiveAndCreateWeekAtomicity`, `TestArchiveAndCreateWeekWithoutPrevious`, `TestSwapRecipeInPlanRotatesIDs`, `TestSwapRecipeInPlanNotFound` |
| `internal/service/generation_test.go` | `TestGenerateWeekArchivesPreviousPlan`, `TestGenerateWeekNoPreviousPlan`, `TestCurrentPlanNoActiveReturnsNil`, `TestSwapRecipeHappyPath`, `TestSwapRecipeUnknownOldID`, `TestSwapRecipeDislikeRetrySucceeds`, `TestSwapRecipeDislikePersistsFails`, `TestSwapRecipePortionsShort`, `TestSwapRecipeProteinVariety`, `TestSwapRecipeKeptContextInPrompt` |
| `internal/handler/generate_test.go` | `TestSwapRendersCards` (incl. new buttons + emoji inference for kept cards), `TestSwapRendersFriendlyErrors` (table of 4 sentinels), `TestSwapNoActivePlanRendersError`, `TestSwapMissingRecipeIDRendersError`. Existing `TestGenerateRendersCards` extended to assert "Replace" + "Regenerate all" + per-card swap `hx-post` URLs. |

## Acceptance Criteria

All boxes from the plan check off via the test suite + smoke test. Live LLM swap / regenerate against a real provider remains the only verification gated to CH-21, as scoped in the plan.
