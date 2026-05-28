# Implementation Report

**Plan**: `.agents/plans/completed/ch-11-fullscreen-recipe-view.plan.md`
**Branch**: `claude/prime-11-fHT4f`
**GitHub Issue**: #11
**Status**: COMPLETE

## Summary

Replaced the stub recipe page with the real F-4 fullscreen view. The new
`recipeHandlers` loads a `domain.Recipe` through a narrow `recipeReader`
interface (satisfied by `*repository.Store`), projects it into a template-safe
view model, and renders a Nordic Kitchen layout: large title + metadata,
ingredients in a left column, numbered steps in a right column, collapsing to
one column on portrait via `@media (orientation: landscape) and (min-width:
900px)`. Active step + step-done state is tracked entirely in the browser
(vanilla JS + `sessionStorage` keyed by recipe id) — no schema change, no
server round-trip per tap. All UI strings are localized in en/fi/ru.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Add i18n keys for the recipe view (back, headings, step labels, toggle labels) | `i18n/{en,fi,ru}.json` | ✅ |
| 2 | Define `recipeReader` interface + `recipeHandlers.Show`, view-model projection, `formatAmount` | `internal/handler/recipe.go` | ✅ |
| 3 | Wire `recipeHandlers` into the router, replace `rd.Recipe` | `internal/handler/router.go` | ✅ |
| 4 | Test router uses a `stubRecipeReader` with fixture recipes | `internal/handler/language_test.go` | ✅ |
| 5 | Fullscreen recipe template — page/content split, two-column grid, step list with `data-recipe-id`, `add` FuncMap helper | `templates/recipe.gohtml`, `internal/handler/render.go` | ✅ |
| 6 | Nordic Kitchen CSS for `.recipe*` + `.recipe-step*`, landscape media query, dark mode via existing tokens | `static/css/app.css` | ✅ |
| 7 | `cooking-steps.js` — vanilla JS step tracker, idempotent IIFE, `sessionStorage` with try/catch | `static/js/cooking-steps.js` | ✅ |
| 8 | Tests: full view, blank id, repository ErrNotFound, generic 500 (no leak), HTMX fragment vs full page, step-tracker markup, all three languages | `internal/handler/recipe_test.go` | ✅ |
| 9 | Full validation pass (gofmt, vet, golangci-lint, test, build) | — | ✅ |

## Validation Results

| Check | Command | Result |
|-------|---------|--------|
| Format | `gofmt -s -l .` | ✅ no output |
| Vet | `go vet ./...` | ✅ clean |
| Lint | `golangci-lint run ./...` | ✅ 0 issues |
| Tests | `go test ./...` | ✅ all packages pass |
| Tests (race) | `go test -race ./internal/handler/...` | ✅ pass |
| Build (CGO off) | `CGO_ENABLED=0 go build ./cmd/server` | ✅ 31.9 MB binary |
| `govulncheck` | n/a in sandbox | ⏭ deferred to CH-21 / networked host |

## End-to-End Verification

Live server boot + HTTP smoke test (port 18080, fresh SQLite, one seeded
`Pan-Seared Salmon` recipe with id `abc123`, 5 steps, 6 ingredients including
one with zero-amount unit "pinch"):

| Probe | Result |
|-------|--------|
| `GET /healthz` | 200 |
| `GET /recipe/abc123` (full page) | 200, 5023 bytes, contains title / `25 min` / `4 servings` / `Ingredients` / `Steps` / `Step 1` / `Step 5` / `Mark done` / `data-recipe-id="abc123"` / `<!doctype` / `app-header` |
| `GET /recipe/abc123` (HX-Request) | 200, fragment-only — `<!doctype` absent, title present |
| `GET /recipe/abc123` (`Accept-Language: fi`) | 200, contains `Ainekset`, `Vaiheet`, `Vaihe 1`, `Takaisin viikkoon` |
| `GET /recipe/abc123` (`Accept-Language: ru`) | 200, contains `Ингредиенты`, `Шаги`, `Шаг 1` |
| `GET /recipe/does-not-exist` | 404, body = "Recipe not found" |
| `GET /static/js/cooking-steps.js` | 200, `text/javascript`, contains `addEventListener` + `sessionStorage` |
| `GET /static/css/app.css` | contains `.recipe__grid`, `@media (orientation: landscape) and (min-width: 900px)`, no new hardcoded colors |
| Leak check on the rendered body | no `repository:`, no `sql.ErrNoRows`, no `db down` (the synthetic internal error) |

**Visual iPad QA** (font sizes from 50 cm, landscape vs portrait split,
prefers-color-scheme swap, smooth scroll, touch-target taps) — **deferred to
Mac mini over tailnet HTTPS** per CLAUDE.md › Validation › Sandbox notes. The
CSS uses only existing Nordic Kitchen variables, so the dark-mode swap at
`static/css/app.css` (the `@media (prefers-color-scheme: dark)` block) flips
the new selectors automatically.

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `i18n/en.json` | UPDATE | +6 |
| `i18n/fi.json` | UPDATE | +6 |
| `i18n/ru.json` | UPDATE | +6 |
| `internal/handler/recipe.go` | UPDATE | +98 / -22 (full rewrite of the stub) |
| `internal/handler/router.go` | UPDATE | +2 / -1 |
| `internal/handler/render.go` | UPDATE | +5 / -3 (added `add` to FuncMap) |
| `internal/handler/recipe_test.go` | UPDATE | +151 / -50 (new test cases) |
| `internal/handler/language_test.go` | UPDATE | +42 / -2 (stub + fixture) |
| `templates/recipe.gohtml` | UPDATE | +41 / -5 (fullscreen layout) |
| `static/css/app.css` | UPDATE | +111 / -0 (`.recipe*` selectors) |
| `static/js/cooking-steps.js` | CREATE | +56 |

Total: **10 files updated, 1 file created**, +524 / -83 lines.

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/handler/recipe_test.go` | `TestRecipeRendersFullscreenView` (title, meta, ingredients, steps, step labels, tracker markup all in one body); `TestRecipeBlankIDNotFound`; `TestRecipeHTMXReturnsFragment`; `TestRecipeNotFoundFromRepository`; `TestRecipeRepositoryErrorIs500` (no leak); `TestRecipeRendersStepsMarkup` (data-recipe-id, data-step-index, aria-pressed hooks); `TestRecipeRendersInAllThreeLanguages` (en/fi/ru table-driven) |
| `internal/handler/language_test.go` | `stubRecipeReader` + `testRecipes()` fixture used by every test that hits the shared router |

All previously-existing tests (home, generate, settings, profile, language,
static, health) still pass. The race detector finds no issues.

## Deviations from Plan

1. **`formatAmount` helper added in the handler.** The plan only sketched
   "Amount float64" on the view model. To keep numeric formatting out of the
   template (and produce `"250"`/`"0.5"` cleanly), the handler projects amounts
   to strings via `strconv.FormatFloat(a, 'f', -1, 64)`. Zero amounts collapse
   to `""` so an ingredient like *"pinch of salt"* renders just the unit.
2. **`main:has(> .recipe)` selector** instead of an additional template-level
   class. iPad Safari supports `:has()`, so the fullscreen override of the
   centered `main` is scoped to the recipe page only, without touching the
   shared `main` rule.
3. **No `data-done` toggle test in the JS file.** The plan only required the
   markup hooks (`data-recipe-id`, `data-step-index`, `aria-pressed`) to be
   present so the client script can find them — that's covered. Adding a JS
   unit test would require a headless browser runtime that isn't part of the
   stack (no npm/node).
4. **Compile-time `var _ recipeReader = (*repository.Store)(nil)` guard**
   added in the test file. Catches drift between the narrow interface and the
   real Store implementation at compile time, mirroring the spirit of the
   plan's "verify integration" requirement.

None of the deviations changes behavior described in the AC; each is a small
implementation detail.

## Acceptance Criteria Status (issue #11)

- [x] Tap on a card opens fullscreen — `/recipe/{id}` already linked from
      `templates/generate.gohtml:6`, now renders the new view (verified via
      HTTP smoke test).
- [x] Body ≥18pt, headings ≥24pt — inherited from `:root` in
      `static/css/app.css:4-40`; new selectors do not override.
- [x] Two columns on iPad landscape, one on portrait — `@media (orientation:
      landscape) and (min-width: 900px) { .recipe__grid { grid-template-columns:
      1fr 1.4fr; } }`.
- [x] Active step visually highlighted + mark-done capability — `.recipe-step
      [aria-current="step"]` outline + `.recipe-step[data-done="true"]`
      strikethrough, toggled by `static/js/cooking-steps.js`.
- [x] Dark mode via `prefers-color-scheme` — all new selectors use
      `var(--bg)`, `var(--text)`, `var(--surface)`, `var(--accent)`,
      `var(--border)`, `var(--text-muted)`, `var(--success)` — every one of
      which is remapped in the existing `@media (prefers-color-scheme: dark)`
      block.
- [x] Smooth one-finger scroll, touch targets ≥44px — `min-height: var(--touch)`
      on every interactive element; `-webkit-tap-highlight-color: transparent`
      for tap feedback; no hover-only affordances.
- [ ] **Visual iPad QA** — deferred to the Mac mini / tailnet HTTPS gate
      (CH-21). HTML/CSS markup correctness is verified above, but the "50 cm
      reading distance" claim and dark-mode visual check require real device
      sign-off.
