# Implementation Report

**Plan**: `.agents/plans/completed/ch-16-recipe-feedback.plan.md`
**Branch**: `claude/jolly-hamilton-RiypN`
**Issue**: #16 (CH-16 — Recipe feedback collection, F-5)
**Status**: COMPLETE

## Summary

Wired the like / dislike / cook-again feedback feature on top of the
already-existing `domain.Feedback` model and repository persistence. Added a thin
`RecipeService.SetFeedback` (read-modify-write with timestamp + clear-on-all-false),
a single idempotent `POST /recipe/{id}/feedback` endpoint, and a shared
`recipe/feedback` template fragment rendered in **both** the fullscreen recipe
detail view and the current-week recipe cards. Feedback persists with a timestamp,
is freely changeable, and the endpoint takes **absolute** state (not a toggle) so a
Service-Worker offline replay is a safe no-op. UI strings added in RU/FI/EN.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | `RecipeService.SetFeedback` (load → clear/set+timestamp → update) | `internal/service/recipe.go` | ✅ |
| 2 | Service unit tests | `internal/service/recipe_test.go` | ✅ |
| 3 | `Feedback` handler + `feedbackView`/`feedbackSetter` + detail projection | `internal/handler/recipe.go` | ✅ |
| 4 | Feedback fields on week cards (Generate + Swap) | `internal/handler/generate.go` | ✅ |
| 5 | Router wiring + `POST /recipe/{id}/feedback` (unconditional) | `internal/handler/router.go` | ✅ |
| 6 | Shared `recipe/feedback` fragment; rendered in detail + cards | `templates/recipe.gohtml`, `templates/generate.gohtml` | ✅ |
| 7 | i18n strings (RU/FI/EN) | `i18n/{en,fi,ru}.json` | ✅ |
| 8 | Nordic Kitchen feedback CSS (44px targets, terracotta active) | `static/css/app.css` | ✅ |
| 9 | Handler/helper tests (set, idempotent, benign-404, 500, detail+card render) | `internal/handler/{recipe_feedback_test,language_test,generate_test}.go` | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ clean |
| `CGO_ENABLED=0 go build ./...` | ✅ |
| `go test ./...` | ✅ all packages pass |
| `golangci-lint run ./...` | ⚠️ DEFERRED — prebuilt binary built with Go < pinned `go1.26.3` refuses to load config (env limitation). `go vet` covers the `govet` linter; `gofmt` clean. Gate at CH-21 / networked host. |
| `govulncheck ./...` | ⚠️ DEFERRED — `vuln.go.dev` 403 in sandbox (CLAUDE.md). Gate at CH-21. |

## End-to-End Verification (live server + real SQLite)

Built the static binary, seeded a household + recipe into a temp SQLite DB
(generation needs an LLM key, unavailable this run), ran the real server on
`:8099`, and exercised the full stack (router → handler → `RecipeService` →
repository → SQLite):

| Step | Result |
|------|--------|
| `GET /healthz` | ✅ 200 |
| `GET /recipe/seed-1` renders feedback control (heading, `class="feedback"`, `hx-post`, "Cook again"), nothing active | ✅ |
| `POST /recipe/seed-1/feedback` `liked=true` → fragment with `feedback__btn--active` + all 3 inputs | ✅ |
| `GET` again → exactly 1 active (persisted to DB) | ✅ |
| `POST` same again (replay) → still exactly 1 active (idempotent through a real DB round-trip) | ✅ |
| `POST` empty body → 0 active (feedback cleared and persisted) | ✅ |
| `POST /recipe/ghost/feedback` (missing recipe) → 200 benign no-op | ✅ |
| `GET /recipe/ghost` → 404 | ✅ |

Deferred (require tailnet HTTPS / iPad Safari, gated at CH-21): visual Nordic
Kitchen styling & 44px touch targets; Service-Worker offline feedback-queue
replay (idempotency proven at the HTTP level here).

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `internal/service/recipe.go` | CREATE | +57 |
| `internal/service/recipe_test.go` | CREATE | +170 |
| `internal/handler/recipe_feedback_test.go` | CREATE | +168 |
| `internal/handler/recipe.go` | UPDATE | +66/-2 |
| `internal/handler/generate.go` | UPDATE | +8/-1 |
| `internal/handler/router.go` | UPDATE | +2/-1 |
| `internal/handler/generate_test.go` | UPDATE | +6/-1 |
| `internal/handler/language_test.go` | UPDATE | +12/-1 |
| `templates/recipe.gohtml` | UPDATE | +48 |
| `templates/generate.gohtml` | UPDATE | +1 |
| `static/css/app.css` | UPDATE | +57 |
| `i18n/en.json` / `fi.json` / `ru.json` | UPDATE | +4 each |

## Deviations from Plan

1. **Checkbox idiom instead of `<button>` + `hx-vals`.** The plan sketched buttons
   with `aria-pressed` and per-button `hx-vals`. The codebase has **no** `hx-vals`
   usage anywhere; the established idiom (shopping list) is a native `<input
   type="checkbox" name=… value="true">` with `hx-trigger="change"` +
   `hx-target="closest …"` + `hx-swap="outerHTML"`. I mirrored that exactly,
   adding `hx-include="closest .feedback"` so each toggle posts the **absolute**
   state of all three independent flags. This keeps consistency with the existing
   code, preserves idempotency, and gives better native accessibility (the
   checkbox carries the state; the styled `<label>` is the 44px touch target).
   Tests assert `feedback__btn--active` + the three `name=…` inputs instead of
   `aria-pressed`.

2. **Archive icons (AC bullet 4) intentionally out of scope.** The recipe archive
   UI does not exist yet — it is **CH-18**. The shared `recipe/feedback` fragment
   is built to be reused as-is by the archive when it lands. Current-week cards +
   detail view deliver the visible feedback icons for this story.

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/service/recipe_test.go` | persists with non-zero timestamp; changeable later; all-false clears; idempotent replay; not-found (no update); update-error propagates; compile-time guard `*repository.Store` satisfies repo iface |
| `internal/handler/recipe_feedback_test.go` | sets + renders active fragment; idempotent double-POST; blank id → 400; missing recipe → benign 200; service error → 500 (no leak); detail view renders control + active state; compile-time guards (real service + stub satisfy `feedbackSetter`) |
| `internal/handler/generate_test.go` | extended `TestGenerateRendersCards` to assert per-card feedback `hx-post` wiring + active state |
