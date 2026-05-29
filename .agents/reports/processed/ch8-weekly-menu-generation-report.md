# Implementation Report

**Plan**: `.agents/plans/completed/ch8-weekly-menu-generation.plan.md`
**Branch**: `claude/prime-8-NkL4U`
**Status**: COMPLETE

## Summary

Implemented CH-8 (F-1) weekly menu generation end-to-end. A "Generate week" button
on the home screen issues an HTMX `POST /generate`; a new `GenerationService`
builds a provider-agnostic LLM prompt from the household profile (family size,
disliked, pantry) plus recent recipe history/feedback, requests three recipes,
validates the hard constraints (exactly three, disliked ingredients 100% excluded
with one semantic retry, portions cover `7 × family size`, ≥2 distinct protein
categories), persists the `WeeklyPlan` and the three `Recipe`s in **one
transaction**, and renders three protein-emoji recipe cards into `#content`. The
LLM provider is selected once in `main.go` from the environment; when no key is
present the feature self-disables and the rest of the app runs unchanged.

A live end-to-end run against OpenAI (`gpt-5.4-mini`) succeeded in ~9s (under the
30s budget), returning three recipes spanning poultry/pork/fish and persisting
them with assigned UUIDs.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Generation prompt + few-shot examples | `internal/llm/prompts/generate_week.v1.txt`, `recipe_examples.v1.txt` | ✅ |
| 2 | `RecentRecipes` + shared `scanRecipe` | `internal/repository/recipe.go` | ✅ |
| 3 | Atomic `CreateWeekWithRecipes` | `internal/repository/weeklyplan.go`, `store.go` | ✅ |
| 4 | Repository tests | `recipe_test.go`, `weeklyplan_test.go` | ✅ |
| 5 | `GenerationService` + DTO | `internal/service/generation.go`, `generation_dto.go` | ✅ |
| 6 | Generation service tests | `internal/service/generation_test.go` | ✅ |
| 7 | `renderFragment` helper | `internal/handler/render.go` | ✅ |
| 8 | Generate handler + emoji map | `internal/handler/generate.go` | ✅ |
| 9 | Router wiring + home flag | `internal/handler/router.go`, `home.go` | ✅ |
| 10 | Handler test | `internal/handler/generate_test.go` | ✅ |
| 11 | Templates | `templates/home.gohtml`, `generate.gohtml` | ✅ |
| 12 | i18n keys (RU/FI/EN) | `i18n/{en,ru,fi}.json` | ✅ |
| 13 | Provider wiring | `cmd/server/main.go` | ✅ |
| 14 | Full validation pass | — | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ |
| `golangci-lint run ./...` | ✅ 0 issues |
| `go test ./...` | ✅ all packages pass |
| `CGO_ENABLED=0 go build ./cmd/server` | ✅ |
| Live `POST /generate` (OpenAI) | ✅ 3 recipes, ~9s, persisted |
| Generation-disabled path (no key) | ✅ button disabled, route absent (404) |

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/llm/prompts/generate_week.v1.txt` | CREATE | System contract + `---TRIGGER---` template |
| `internal/llm/prompts/recipe_examples.v1.txt` | CREATE (prior) | 9 Finnish few-shot examples |
| `internal/service/generation.go` | CREATE | Orchestration, validation, persistence |
| `internal/service/generation_dto.go` | CREATE | LLM JSON DTO |
| `internal/service/generation_test.go` | CREATE | 5 test funcs |
| `internal/repository/recipe.go` | UPDATE | `RecentRecipes`, shared `scanRecipe`/`insertRecipe` |
| `internal/repository/weeklyplan.go` | UPDATE | `CreateWeekWithRecipes`, shared `insertPlanWithItems` |
| `internal/repository/store.go` | UPDATE | `execer` interface |
| `internal/repository/recipe_test.go` | UPDATE | ordering/limit/scoping tests |
| `internal/repository/weeklyplan_test.go` | UPDATE | atomic write + rollback tests |
| `internal/handler/generate.go` | CREATE | `POST /generate`, emoji map, card VM |
| `internal/handler/generate_test.go` | CREATE | cards + friendly-error tests |
| `internal/handler/render.go` | UPDATE | `renderFragment` |
| `internal/handler/home.go` | UPDATE | `homeHandlers` + `CanGenerate` |
| `internal/handler/router.go` | UPDATE | accept `llm.Client`, conditional route |
| `internal/handler/language_test.go`, `health_test.go` | UPDATE | adapt to new signatures |
| `templates/home.gohtml` | UPDATE | generate button + `#week` target |
| `templates/generate.gohtml` | CREATE | `generate/cards` + `generate/error` |
| `i18n/{en,ru,fi}.json` | UPDATE | generate/error/card keys |
| `cmd/server/main.go` | UPDATE | `newLLMClient` env-selected provider |

## Deviations from Plan

1. **`request_id` not propagated into the generation LLM call.** The request ID is
   stored under the handler's unexported context key; reading it from the service
   would couple service→handler (a reverse dependency). I left `Request.RequestID`
   empty rather than introduce that coupling — provider logs show `request_id=""`
   for generation calls. A neutral request-scoped-value package would restore full
   correlation; deferred as a small observability follow-up.
2. **Live generation actually ran in-sandbox.** The plan assumed the provider host
   was blocked; in this environment `OPENAI_API_KEY` is set and `api.openai.com`
   was reachable, so the live E2E (not just the fake-client path) passed. The
   CH-21 deploy gate still applies for the Mac mini / tailnet specifics
   (Service-Worker over HTTPS, `govulncheck`).
3. Added `generation_dto.go` as a separate file (plan folded the DTO into
   `generation.go`) — cosmetic split for readability.

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/service/generation_test.go` | happy path; dislike-retry-succeeds; dislike-persists→`ErrDislikeViolation`; validation table (not-three→`ErrGenerationInvalid`, short→`ErrPortionsShort`, single-protein→`ErrProteinVariety`); prompt-includes-history/feedback/disliked/target/system+examples |
| `internal/repository/recipe_test.go` | RecentRecipes ordering+limit; scoped-to-household |
| `internal/repository/weeklyplan_test.go` | CreateWeekWithRecipes persists plan+3 recipes; rolls back fully on failure |
| `internal/handler/generate_test.go` | renders 3 cards with emoji+time; friendly localized errors with no internal-detail leak |

## Deferred (CH-21 / networked host)

- `govulncheck ./...` (vuln.go.dev 403 in sandbox; no new deps added)
- Service-Worker behavior of the new fragment over tailnet HTTPS
