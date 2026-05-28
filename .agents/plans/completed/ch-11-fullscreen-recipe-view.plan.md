# Plan: CH-11 Fullscreen Recipe View (F-4)

## Summary

Replace the current stub recipe page (`templates/recipe.gohtml` echoes the ID only)
with a real fullscreen recipe view for iPad cooking. The handler loads a `domain.Recipe`
from the existing repository, renders title / metadata / ingredients / steps with
Nordic Kitchen typography (≥18pt body, ≥24pt headings, 44px touch targets), uses a
two-column iPad-landscape layout that collapses to one column on portrait, and tracks
"active step" + "step done" state in the browser via a tiny inline script
(no server round-trip — cooking state is ephemeral). The existing recipe-card link on
the home cards already points at `/recipe/{id}` with `hx-target="#content"`, so the
fullscreen view is reachable by tap from day one.

## User Story

As a home cook on an iPad
I want to open a recipe in a large-print fullscreen view and track which step I'm on
So that I can read the recipe from 50cm away without zooming and not lose my place mid-cook

## Metadata

| Field | Value |
|-------|-------|
| Type | ENHANCEMENT (the recipe screen exists as a stub; this turns it into the real F-4 view) |
| Complexity | MEDIUM |
| Systems Affected | `internal/handler/recipe.go`, `internal/handler/recipe_test.go`, `templates/recipe.gohtml`, `static/css/app.css`, `i18n/{en,fi,ru}.json`, `internal/handler/router.go` (inject recipe loader), `internal/handler/language_test.go` (test router wiring) |
| GitHub Issue | #11 |

---

## Acceptance Criteria (from issue #11)

- [ ] Tapping a card opens the fullscreen mode (`/recipe/{id}` already wired from `templates/generate.gohtml:6` — verify the destination renders the new view, not the stub).
- [ ] Body text ≥18pt, headings ≥24pt — Nordic Kitchen already defines this in `static/css/app.css:29-40`; new components must inherit, not override.
- [ ] Ingredients + steps in two columns on iPad landscape, one column on portrait — use a CSS Grid with a landscape media query (`@media (orientation: landscape) and (min-width: 900px)`).
- [ ] Active step is visually highlighted; user can mark a step done — inline JS using `data-*` attributes and `aria-current="step"`; persistence is `sessionStorage` keyed by recipe id (survives refresh during the cook, not across cooks).
- [ ] Dark mode respected via `prefers-color-scheme` — the CSS variables in `static/css/app.css:104-112` already swap; reuse `var(--bg) / --text / --accent`, never hardcoded colors.
- [ ] Smooth one-finger scroll, ≥44×44pt touch targets — every interactive element uses `min-height: var(--touch)`; no hover-only affordances.

---

## Patterns to Follow

### Handler (load entity → view model → render; pull dependency through a narrow interface)
```go
// SOURCE: internal/handler/generate.go:17-35 + 78-102
type weekGenerator interface {
    GenerateWeek(ctx context.Context, h *domain.HouseholdProfile) (*service.GeneratedWeek, error)
    // ...
}
type recipeLoader interface {
    RecipesByIDs(ctx context.Context, ids []string) ([]domain.Recipe, error)
}

func (gh *generateHandlers) Generate(w http.ResponseWriter, r *http.Request) {
    // load → build view model → renderFragment
    gh.rd.renderFragment(w, r, http.StatusOK, "generate/cards", cardsData{Cards: cards})
}
```
Apply this verbatim shape: a `recipeReader` interface with the single method we need
(`GetRecipe(ctx, id) (*domain.Recipe, error)`), a `recipeData` view model populated by
the handler, and the existing `renderer.render` path (page/content split is automatic).

### Renderer call (page vs HTMX fragment is automatic; pass data, not strings)
```go
// SOURCE: internal/handler/recipe.go:18-26 (current stub)
func (rd *renderer) Recipe(w http.ResponseWriter, r *http.Request) {
    lang := string(LanguageFromContext(r.Context()))
    id := strings.TrimSpace(r.PathValue("id"))
    if id == "" {
        rd.renderStatus(w, r, http.StatusNotFound, "recipe", recipeData{Lang: lang, NotFound: true})
        return
    }
    rd.render(w, r, "recipe", recipeData{Lang: lang, ID: id})
}
```
The new handler keeps this shape but moves off `*renderer` (which has no DB access) onto
a dedicated `recipeHandlers` struct holding `rd` + `recipes` — mirroring `homeHandlers`
and `generateHandlers`. The `*renderer` method becomes obsolete and is deleted.

### Not-found and error path (404 on missing, generic 500 on infra failures)
```go
// SOURCE: internal/repository/recipe.go:97-100 + internal/handler/render.go:97-100
if errors.Is(err, sql.ErrNoRows) { return nil, ErrNotFound }
// handler:
http.Error(w, "internal server error", http.StatusInternalServerError)
```
- Empty path id → 404 with `recipeData{NotFound: true}` (already in stub).
- `repository.ErrNotFound` from `GetRecipe` → 404 with the same not-found view model.
- Anything else → `rd.fail` (logs + generic 500). Never echo internal detail.

### Template structure (page/content pair, i18n via `t`, HTMX-friendly partials)
```gohtml
{{- define "recipe/page" -}}
<!doctype html><html lang="{{ .Lang }}"><head>{{ template "head" . }}</head>
<body>{{ template "header" . }}
  <main id="content">{{ template "recipe/content" . }}</main>
  <script src="/static/js/htmx.min.js"></script>
  {{ template "sw-register" . }}
</body></html>
{{- end -}}

{{- define "recipe/content" -}} ... {{- end -}}
```
The page/content split is enforced by `internal/handler/render.go:48-52` —
HTMX nav gets `<page>/content`, full nav gets `<page>/page`. Keep that contract.

### CSS (extend Nordic Kitchen tokens; no new colors, no inline styles)
```css
/* SOURCE: static/css/app.css:4-18 + :80-87 */
:root { --bg: #F5EFE6; --text: #2B2118; --accent: #C2603A; --touch: 44px; ... }
button, input { min-height: var(--touch); font-size: 1.125rem; }
```
All new selectors live in `static/css/app.css` (single stylesheet today). Use existing
CSS variables; respect `prefers-color-scheme` automatically because the variables flip
in the media query at `:104-112`.

### Tests (table-driven, full router + direct handler, HTMX vs full page asserted)
```go
// SOURCE: internal/handler/recipe_test.go:10-59
func TestRecipeRendersID(t *testing.T) {
    srv := newTestRouter(t)
    req := httptest.NewRequest(http.MethodGet, "/recipe/abc123", nil)
    rec := httptest.NewRecorder()
    srv.ServeHTTP(rec, req)
    // assert status + body contains expected localized strings
}
```
Update `newTestRouter` in `language_test.go:35-45` to also wire the new recipe handler
(with an in-memory `stubRecipeLoader`), the same way `generate_test.go:72-82` does.

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `internal/handler/recipe.go` | UPDATE | Replace stub with `recipeHandlers` that loads a recipe via a `recipeReader` interface and builds the view model. |
| `internal/handler/router.go` | UPDATE | Construct `recipeHandlers{rd, recipes: store}` and register `GET /recipe/{id}` on it (replacing `rd.Recipe`). |
| `templates/recipe.gohtml` | UPDATE | Replace the stub markup with the real fullscreen layout: header (title/meta/back link), ingredients column, steps column with per-step toggle. |
| `static/css/app.css` | UPDATE | Add `.recipe-*` classes for the fullscreen layout, the landscape→portrait grid, and the active/done step states. No new color values; reuse tokens. |
| `i18n/en.json` | UPDATE | Add keys: `recipe.back`, `recipe.ingredients_heading`, `recipe.steps_heading`, `recipe.step_label` (e.g. "Step %d"), `recipe.step_done`, `recipe.step_undone`, `recipe.mark_done`, `recipe.mark_undone`. |
| `i18n/fi.json` | UPDATE | Finnish translations of the same new keys. |
| `i18n/ru.json` | UPDATE | Russian translations of the same new keys. |
| `internal/handler/recipe_test.go` | UPDATE | Cover: full recipe rendering, ingredients + steps in body, HTMX fragment vs full page, ErrNotFound → 404, infra error → 500 (no leak), step-toggle markup present (data attrs / aria), localized headings. |
| `internal/handler/language_test.go` | UPDATE | Wire a `stubRecipeReader` into `newTestRouter` so existing recipe tests still pass after the handler shape change. |
| `static/js/cooking-steps.js` | CREATE | ~30 lines of vanilla JS: read recipe id from a `data-recipe-id` attribute on the steps `<ol>`, toggle `aria-current`/`data-done` on click/tap, persist to `sessionStorage[recipe:{id}]`. No framework. |
| `templates/recipe.gohtml` | (additional) | Include `<script src="/static/js/cooking-steps.js" defer></script>` inside `recipe/page` only (HTMX fragment swap leaves the existing one running). |

> Why client-side step state, not server: cooking-state is per-session and ephemeral.
> PRD §15 has no `cooked_step` column, and adding one costs a migration + DB write per
> tap (every step) for no durable value. `sessionStorage` survives a refresh during the
> cook (the real failure mode) without any new schema.

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Add i18n keys for the recipe view

- **Files**: `i18n/en.json`, `i18n/fi.json`, `i18n/ru.json`
- **Action**: UPDATE
- **Implement**: Add the new keys listed in *Files to Change* to all three dictionaries. EN reference values:
  - `recipe.back`: "Back to week"
  - `recipe.ingredients_heading`: "Ingredients"
  - `recipe.steps_heading`: "Steps"
  - `recipe.step_label`: "Step %d"
  - `recipe.mark_done`: "Mark done"
  - `recipe.mark_undone`: "Undo"
  - `recipe.servings` (already exists at `i18n/en.json:17`) — reuse, do not duplicate.
  - `recipe.cook_time` (already exists at `i18n/en.json:16`) — reuse.
- **Mirror**: `i18n/en.json:1-41` — same flat shape, same key order grouping.
- **Validate**: `gofmt -s -l .` (no Go changes yet) + `go vet ./...` (still clean).

### Task 2: Define a recipeReader interface and recipeHandlers struct

- **File**: `internal/handler/recipe.go`
- **Action**: UPDATE
- **Implement**:
  - Define `type recipeReader interface { GetRecipe(ctx context.Context, id string) (*domain.Recipe, error) }`.
  - Define `type recipeHandlers struct { rd *renderer; recipes recipeReader }`.
  - Define the view model with named fields: `Lang string`, `NotFound bool`, `Recipe *recipeView`, where `recipeView` carries pre-formatted fields the template needs (`ID`, `Title`, `Description`, `CookTime int`, `Servings int`, `Ingredients []ingredientView`, `Steps []string`). Keep `domain.Recipe` out of templates — convert in the handler.
  - Move the existing stub-style `(rd *renderer) Recipe` to `(h *recipeHandlers) Show`. Logic:
    1. `id` empty → render not-found 404 (current behavior).
    2. `h.recipes.GetRecipe(ctx, id)` → on `errors.Is(err, repository.ErrNotFound)` render 404; on other error `h.rd.fail(...)`; on success build the view model.
    3. Always pass `Lang` from `LanguageFromContext`.
  - Delete the old `(rd *renderer) Recipe` method.
- **Mirror**: `internal/handler/generate.go:25-35` for the loader interface + struct shape; `internal/handler/recipe.go:18-26` (current) for the not-found early return; `internal/repository/recipe.go:97-100` for the `ErrNotFound` sentinel.
- **Validate**: `go build ./...` (compile only).

### Task 3: Wire recipeHandlers in the router

- **File**: `internal/handler/router.go`
- **Action**: UPDATE
- **Implement**:
  - In `NewRouter`, after `store := repository.New(db)`, construct `rh := &recipeHandlers{rd: rd, recipes: store}`.
  - Replace `mux.HandleFunc("GET /recipe/{id}", rd.Recipe)` with `mux.HandleFunc("GET /recipe/{id}", rh.Show)`.
- **Mirror**: `internal/handler/router.go:32-45` for the construction-then-register shape used by `homeHandlers`, `profileHandlers`, `generateHandlers`.
- **Validate**: `go build ./...`.

### Task 4: Update test router and add a stub recipeReader

- **File**: `internal/handler/language_test.go`
- **Action**: UPDATE
- **Implement**:
  - Define `stubRecipeReader` near the top of the file (or in a new tiny `recipe_test.go` helper if cleaner — pick one and keep it consistent with `generate_test.go`'s pattern of stubs in the same file). Shape:
    ```go
    type stubRecipeReader struct {
        byID map[string]*domain.Recipe
        err  error
    }
    func (s stubRecipeReader) GetRecipe(_ context.Context, id string) (*domain.Recipe, error) {
        if s.err != nil { return nil, s.err }
        r, ok := s.byID[id]
        if !ok { return nil, repository.ErrNotFound }
        return r, nil
    }
    ```
  - In `newTestRouter`, replace `mux.HandleFunc("GET /recipe/{id}", rd.Recipe)` with `mux.HandleFunc("GET /recipe/{id}", (&recipeHandlers{rd: rd, recipes: stubRecipeReader{byID: testRecipes()}}).Show)`, where `testRecipes()` returns a small fixture map containing the id `"abc123"` used by `TestRecipeRendersID` and `TestRecipeHTMXReturnsFragment`.
- **Mirror**: `internal/handler/generate_test.go:54-70` (stubRecipeLoader pattern) and `internal/handler/language_test.go:35-45` (newTestRouter shape).
- **Validate**: `go build ./...`; `go test ./internal/handler/ -run TestRecipe -v` (Tasks 5 makes them pass; this task just must compile).

### Task 5: Rewrite the recipe template for the fullscreen view

- **File**: `templates/recipe.gohtml`
- **Action**: UPDATE
- **Implement**:
  - Keep the `recipe/page` + `recipe/content` define-pair (the renderer relies on it — see `internal/handler/render.go:48-52`).
  - `recipe/page`: same shell as `templates/home.gohtml:1-16`, plus `<script src="/static/js/cooking-steps.js" defer></script>` immediately before `sw-register`.
  - `recipe/content`:
    - If `.NotFound`, render `<h1>{{ t "recipe.heading" }}</h1><p role="alert">{{ t "recipe.not_found" }}</p>` (current stub behavior).
    - Otherwise, a `<article class="recipe">` containing:
      - `<a href="/" hx-get="/" hx-target="#content" hx-push-url="true" class="recipe__back">{{ t "recipe.back" }}</a>`
      - `<h1 class="recipe__title">{{ .Recipe.Title }}</h1>`
      - `<p class="recipe__meta">{{ t "recipe.cook_time" .Recipe.CookTime }} · {{ t "recipe.servings" .Recipe.Servings }}</p>`
      - `<p class="recipe__desc">{{ .Recipe.Description }}</p>`
      - `<div class="recipe__grid">` containing two `<section>`s:
        - `<section class="recipe__ingredients" aria-labelledby="ing-h"><h2 id="ing-h">{{ t "recipe.ingredients_heading" }}</h2><ul>...</ul></section>` — each `<li>` carries amount + unit + name.
        - `<section class="recipe__steps" aria-labelledby="step-h"><h2 id="step-h">{{ t "recipe.steps_heading" }}</h2><ol data-recipe-id="{{ .Recipe.ID }}">...</ol></section>` — each `<li>` is `<li class="recipe-step" data-step-index="{{ $i }}"><div class="recipe-step__body">…</div><button type="button" class="recipe-step__toggle" aria-pressed="false">{{ t "recipe.mark_done" }}</button></li>`. Use `{{ range $i, $s := .Recipe.Steps }}` so the index is one-based via `{{ add $i 1 }}` — register an `add` FuncMap entry in `handler/render.go` ParseFuncMap if not already present; if a `t "recipe.step_label" (add $i 1)` shape is cleaner, use that.
  - Decide once whether step labels use `{{ t "recipe.step_label" $i1 }}` or just the number — pick the former, register `add` in `ParseFuncMap`.
- **Mirror**: `templates/home.gohtml:1-16` for the page shell; `templates/generate.gohtml:1-26` for the i18n + HTMX attribute style; `internal/handler/render.go:18-20` for the `ParseFuncMap` shape.
- **Validate**: `go test ./internal/handler/ -run TestRecipe` (some still failing — Task 6 finishes them).

### Task 6: Add Nordic Kitchen styles for the fullscreen layout

- **File**: `static/css/app.css`
- **Action**: UPDATE (append; do not edit existing rules)
- **Implement** (selector sketch — content driven by AC):
  - `.recipe { max-width: none; }` — fullscreen, override the 48rem `main` cap from `static/css/app.css:46`.
  - `.recipe__back { display: inline-flex; align-items: center; min-height: var(--touch); ... }`
  - `.recipe__title { font-size: 2.5rem; }` (≥24pt → comfortably above).
  - `.recipe__meta, .recipe__desc { font-size: 1.25rem; color: var(--text-muted); }`
  - `.recipe__grid { display: grid; gap: var(--space); grid-template-columns: 1fr; }` — single column default (portrait).
  - `@media (orientation: landscape) and (min-width: 900px) { .recipe__grid { grid-template-columns: 1fr 1.4fr; } }` — ingredients narrower, steps wider.
  - `.recipe__ingredients li { padding: 0.5rem 0; border-bottom: 1px solid var(--border); font-size: 1.25rem; }`
  - `.recipe-step { display: flex; gap: 1rem; align-items: flex-start; padding: 1rem; border-radius: 12px; }`
  - `.recipe-step[aria-current="step"] { background: var(--surface); box-shadow: inset 4px 0 0 var(--accent); }`
  - `.recipe-step[data-done="true"] .recipe-step__body { text-decoration: line-through; color: var(--text-muted); }`
  - `.recipe-step__toggle { min-height: var(--touch); min-width: var(--touch); background: var(--surface); border: 1px solid var(--border); border-radius: 8px; }`
  - `html { -webkit-tap-highlight-color: transparent; }` (smooth tap feedback on iPad) — optional but cheap.
  - Dark mode requires no new rules: every color above is a CSS variable defined in `:root` and remapped at `static/css/app.css:104-112`.
- **Mirror**: `static/css/app.css:80-102` for the touch-target + button shape; `:4-18` for the CSS variable taxonomy.
- **Validate**: `go test ./internal/handler/ -run TestStaticFilesServesCSS -v` (already exists at `static_test.go:46-59`; confirm no regression — body still contains "Nordic Kitchen").

### Task 7: Add the cooking-steps client script

- **File**: `static/js/cooking-steps.js`
- **Action**: CREATE
- **Implement** (~30 lines, no framework, no `eval`, no remote fetch):
  ```js
  (function () {
    var ol = document.querySelector('.recipe__steps ol[data-recipe-id]');
    if (!ol) return;
    var key = 'recipe:' + ol.getAttribute('data-recipe-id');
    var done = {};
    try { done = JSON.parse(sessionStorage.getItem(key) || '{}'); } catch (_) {}

    var steps = ol.querySelectorAll('li.recipe-step');
    var active = 0;
    function paint() {
      steps.forEach(function (li, i) {
        li.toggleAttribute('data-done', !!done[i]);
        if (done[i]) li.setAttribute('data-done', 'true');
        else li.removeAttribute('data-done');
        if (i === active) li.setAttribute('aria-current', 'step');
        else li.removeAttribute('aria-current');
        var btn = li.querySelector('.recipe-step__toggle');
        if (btn) btn.setAttribute('aria-pressed', done[i] ? 'true' : 'false');
      });
    }
    steps.forEach(function (li, i) {
      li.addEventListener('click', function (e) {
        if (e.target.closest('.recipe-step__toggle')) {
          done[i] = !done[i];
          try { sessionStorage.setItem(key, JSON.stringify(done)); } catch (_) {}
        } else {
          active = i;
        }
        paint();
      });
    });
    paint();
  })();
  ```
  Key requirements:
  - No reliance on a framework — the project ships only HTMX + this file (see `static/js/htmx.min.js` is the only JS today).
  - The script is **idempotent** — re-running it via an HTMX fragment swap must not double-bind. The IIFE runs once per `<script src>` insertion; full-page nav re-runs it, fragment swap (`#content` only) leaves it. That matches the `defer` placement inside `recipe/page` (not `recipe/content`).
  - Failure of `sessionStorage` (Safari private mode) must not crash the script — hence the try/catch.
- **Mirror**: `static/sw.js:1-15` (existing vanilla-JS style, no build step).
- **Validate**: `go test ./internal/handler/ -run TestStaticFiles -v` — also confirm the file is embedded by `static/embed.go` (it uses `embed.FS` over the directory; new files are picked up automatically).

### Task 8: Round out the test suite

- **File**: `internal/handler/recipe_test.go`
- **Action**: UPDATE
- **Implement** new + updated cases. The existing `TestRecipeRendersID`, `TestRecipeBlankIDNotFound`, `TestRecipeHTMXReturnsFragment` need their fixtures aligned with the new view model:
  - `TestRecipeRendersID` — assert title, cook time, a known ingredient name, a known step text are all in the body.
  - `TestRecipeBlankIDNotFound` — same as today, against `(&recipeHandlers{rd: rd, recipes: stubRecipeReader{}}).Show`.
  - `TestRecipeHTMXReturnsFragment` — fragment must contain the recipe title and must NOT contain `<!doctype`.
  - **New**: `TestRecipeNotFoundFromRepository` — `recipes` returns `repository.ErrNotFound`; expect 404 + localized "not found" string; must not leak `ErrNotFound` to the body.
  - **New**: `TestRecipeRepositoryErrorIs500` — `recipes` returns a synthetic error; expect 500 and body == "internal server error\n" (matches `render.go:99`).
  - **New**: `TestRecipeRendersStepsMarkup` — body contains the `data-recipe-id="..."` attribute, at least one `class="recipe-step"`, and the `Mark done` button label. Confirms the client script will find its hooks.
  - **New**: `TestRecipeRendersInAllThreeLanguages` — table-driven; for `ru` / `fi` / `en` confirm the localized "Ingredients" / "Steps" heading appears.
- **Mirror**: `internal/handler/recipe_test.go:10-59` for the existing shape; `internal/handler/generate_test.go:240-271` for table-driven coverage of error paths.
- **Validate**: `go test ./internal/handler/...`.

### Task 9: Full validation pass

- **Files**: none
- **Action**: VALIDATE
- **Implement**: Run the project's full check set (see *Validation* section below) and confirm clean.
- **Validate**: `gofmt -s -l .` empty; `go vet ./...` empty; `golangci-lint run ./...` clean; `go test ./...` passes; `go build ./cmd/server` succeeds.

---

## Risks

| Risk | Mitigation |
|------|------------|
| HTMX fragment swap to `#content` re-runs `recipe/content` but not the `<script>` tag (it lives in `recipe/page` only) → step interactions break after intra-app nav | Place the `<script defer>` inside `recipe/page`. Verified by: a full GET → script runs; an HTMX nav into `/recipe/{id}` from the home card swaps the content and the script tag already loaded earlier still works against the *new* `<ol>` because the IIFE re-queries on each page load — but on an in-place fragment swap there's nothing to re-bind. **Mitigation**: keep the script tag *also* in `recipe/content` and make the IIFE idempotent (it queries by selector and binds — re-running over the same DOM after a swap is fine because the DOM is fresh). Document this in a one-line comment at the top of `cooking-steps.js`. |
| iPad Safari `sessionStorage` blocked in Private Browsing — `setItem` throws | Wrap each `sessionStorage` access in try/catch (already in the sketch). State degrades to in-memory only, which is acceptable. |
| `add` template function not registered → template parse error | Add `"add": func(a, b int) int { return a + b }` to `ParseFuncMap` in `internal/handler/render.go:18-20`. Cover with the existing `TestRecipeRendersStepsMarkup`. |
| Step state leaks across recipes when navigating between them | Key the sessionStorage entry by recipe id (`recipe:<id>`), as in the sketch. Confirmed: each recipe gets its own bucket. |
| Repository `GetRecipe` returns a recipe whose `HouseholdID` doesn't match the current household → cross-household leak in a future multi-user world | Out of scope for MVP (single household, no auth — see CLAUDE.md › Security). Track in a follow-up when multi-user lands; the schema already carries `household_id`. |
| Template field added but not localized → key shows up verbatim in UI | `i18n.Bundle.Translator` falls back to the key when missing; the language-coverage test in Task 8 catches this for the three new heading keys per language. |
| `recipe.gohtml` page/content split broken → renderer 500s on full nav OR HTMX nav | Keep the existing `{{- define "recipe/page" -}}` / `{{- define "recipe/content" -}}` pair. `internal/handler/render.go:48-52` decides which to execute; both tests `TestRecipeRendersID` (full) and `TestRecipeHTMXReturnsFragment` (fragment) already cover this. |

---

## Environment & Verification

CLAUDE.md › Validation › *Web / sandbox environment constraints* applies — Service Worker
activation needs HTTPS and isn't available in the sandbox, and `govulncheck` is blocked.
Both are non-blocking for CH-11 (no new deps, no SW changes).

| Verification | Runs in env? | If blocked: where/when verified |
|--------------|--------------|---------------------------------|
| `gofmt -s -l .` | yes | — |
| `go vet ./...` | yes | — |
| `golangci-lint run ./...` | yes | — |
| `go test ./...` | yes | — |
| `go build ./cmd/server` (CGO disabled — modernc.org/sqlite is pure Go) | yes | — |
| `govulncheck ./...` | no | networked host or CH-21 deploy gate |
| Visual check: iPad Safari landscape vs portrait, font sizes from 50 cm, dark-mode swap | no | Mac mini over tailnet HTTPS — Phase 2 manual QA gate |
| Service Worker still registers after the new page is added | no | tailnet HTTPS — gated by CH-21 |

---

## Validation

Use the exact commands from CLAUDE.md:

```bash
gofmt -s -l .          # formatting (no output = clean)
go vet ./...           # vet
golangci-lint run ./...# lint
go test ./...          # tests
# govulncheck ./...    # blocked in sandbox; defer to CH-21
```

---

## Acceptance Criteria (release-gate checklist)

- [ ] All 9 tasks completed.
- [ ] `gofmt`, `go vet`, `golangci-lint`, `go test ./...` all clean.
- [ ] `/recipe/{id}` renders title, cook-time, servings, ingredients list, numbered steps.
- [ ] Mark-done button toggles `aria-pressed` and a visual struck-through style on the step.
- [ ] Tapping a step body marks it as the active step (`aria-current="step"`).
- [ ] Dark-mode CSS swap confirmed by reading the variables in `app.css:104-112` (no new hardcoded colors anywhere in the diff).
- [ ] iPad landscape renders ingredients + steps in two columns; portrait collapses to one (visual check deferred to tailnet/Mac mini — recorded in the env table above).
- [ ] Existing tests still pass; new ones cover not-found from repo, infra-error 500, all three languages, step markup hooks.
- [ ] Issue #11 closes on push (commit message references the issue).
- [ ] Env-blocked verifications (visual iPad QA, govulncheck, SW re-check) noted in the PR description as deferred to CH-21 / Mac-mini gate.
