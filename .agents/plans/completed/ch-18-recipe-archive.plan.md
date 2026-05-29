# Plan: Recipe Archive with Search & "Cook Again" (CH-18 / F-8)

## Summary

Add a recipe **archive** screen, reachable from the main-menu header, that lists every
recipe the household has ever generated (newest first) with read-only feedback icons, a
debounced substring **title search** (HTMX), and a **"Cook again"** action that drops a
chosen archived recipe into the current weekly plan — replacing one of the three (the user
picks which via a small dialog) and rebuilding the shopping list. Listing and search are
pure repository reads available unconditionally; "cook again" reuses the existing atomic
`SwapRecipeInPlan` transaction and the `ShoppingBuilder`, so it is wired only when the LLM
is configured (the same `canGenerate` gate the per-card swap already uses, because the
shopping rebuild can fall through to an LLM categorize call). Archive read failures degrade
to a friendly in-page banner, never a whole-page 500.

## User Story

As a member of the household
I want to browse, search, and re-cook any recipe we've generated before
So that a dish we liked can come back into this week's menu without regenerating from scratch.

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY |
| Complexity | MEDIUM |
| Systems Affected | repository, service (generation), handler, templates, i18n, router |
| GitHub Issue | #18 (CH-18) |

---

## Key Design Decisions (read before implementing)

1. **"Cook again" copies the recipe, it does not re-reference it.** `SwapRecipeInPlan`
   calls `insertRecipe`, which assigns a fresh UUID — passing an existing recipe's ID would
   collide on the primary key. So cook-again builds a **copy** of the archived recipe with
   `ID=""`, `HouseholdID=h.ID`, `Source=domain.SourceHistory`, `Feedback=nil`, and lets the
   existing swap transaction insert it. This is exactly what the unused `SourceHistory` enum
   (`internal/domain/recipe.go:22`) was modeled for, and it reuses the atomic
   recipe-insert + recipe_ids-rotate + shopping-list-replace transaction verbatim
   (`internal/repository/weeklyplan.go:99-148`).
2. **Cook-again reuses `GenerationService`** (it already owns plan mutation + the
   `ShoppingBuilder` + `SwapRecipeInPlan`). A new `CookAgain` method sits beside `SwapRecipe`.
   No hard-constraint re-validation (dislikes / portions / protein variety) — the user is
   explicitly choosing this dish, unlike an LLM generation.
3. **Cook-again is gated behind `canGenerate`**, like swap, because the shopping rebuild
   (`ShoppingBuilder.Build` → `shopping.Consolidate` re-categorizes by *name*;
   dictionary misses fall through to an LLM `RoleCategorize` call —
   `internal/service/shopping.go:58-86`, `internal/shopping/consolidate.go:74`). Listing,
   search, and feedback icons are **always** available; the "Cook again" button renders only
   when `canGenerate` is true.
4. **No DB migration.** The `recipe` table and its `title` column already exist; search is a
   `LIKE` query. A single `SearchRecipes(householdID, query, limit)` serves both the initial
   full list (empty query ⇒ matches all) and the HTMX search fragment.

---

## Patterns to Follow

### Naming — narrow handler dependency interfaces
```go
// SOURCE: internal/handler/disliked.go:17-27
type dislikedProfiles interface {
	Current(ctx context.Context, defaultLang domain.Language) (*domain.HouseholdProfile, error)
	AddDisliked(ctx context.Context, id, term string) (*domain.HouseholdProfile, error)
	RemoveDisliked(ctx context.Context, id, term string) (*domain.HouseholdProfile, error)
}
type recipeHistory interface {
	RecentRecipes(ctx context.Context, householdID string, limit int) ([]domain.Recipe, error)
}
```

### Repository — shared SELECT list + scanRecipe, newest-first, scoped by household
```go
// SOURCE: internal/repository/recipe.go:125-153 (RecentRecipes — mirror for SearchRecipes)
q := `SELECT ` + recipeColumns + `
	FROM recipe WHERE household_id = ? ORDER BY created_at DESC LIMIT ?`
rows, err := s.db.QueryContext(ctx, q, householdID, limit)
if err != nil { return nil, fmt.Errorf("recent recipes: %w", err) }
defer func() { _ = rows.Close() }()
for rows.Next() {
	r, err := scanRecipe(rows)
	...
}
```

### Service — swap that reuses SwapRecipeInPlan + builder (mirror for CookAgain)
```go
// SOURCE: internal/service/generation.go:298-315
newRecipe, protein := mapRecipe(reply.Recipe, h)
allRecipes := append(append(make([]domain.Recipe, 0, len(kept)+1), kept...), newRecipe)
list, err := g.builder.Build(ctx, allRecipes, h.PantryBasics)
if err != nil { return nil, fmt.Errorf("swap recipe: %w", err) }
if err := g.repo.SwapRecipeInPlan(ctx, plan.ID, oldRecipeID, &newRecipe, list); err != nil {
	return nil, fmt.Errorf("swap recipe: %w", err)
}
plan.RecipeIDs[idx] = newRecipe.ID
plan.ShoppingList = list
```

### Handler — graceful degradation on a non-critical read
```go
// SOURCE: internal/handler/disliked.go:59-68 (degrade, don't 500)
var suggestions []string
if recipes, err := dh.history.RecentRecipes(r.Context(), h.ID, suggestionLimit); err == nil {
	suggestions = distinctIngredientSuggestions(recipes, h.DislikedIngredients)
}
dh.rd.render(w, r, "disliked", dislikedData{ ... })
```

### Handler — buffered, localized fragment render for HTMX partials
```go
// SOURCE: internal/handler/render.go:72-91 ; internal/handler/shopping.go:201-212
sh.rd.renderFragment(w, r, http.StatusOK, "shopping/item", toShoppingItemView(*item))
```

### Template — page/content pair + HTMX list with debounced search
```gohtml
{{- /* SOURCE: templates/disliked.gohtml:1-39 (page/content pair) + */ -}}
{{- /* hx-trigger debounce idiom from CLAUDE.md AC: "keyup changed delay:200ms" */ -}}
<input type="search" name="q" hx-get="/archive/search"
       hx-trigger="keyup changed delay:200ms, search"
       hx-target="#archive-list" hx-swap="outerHTML">
```

### Read-only feedback icons (display, not the interactive control)
```gohtml
{{- /* SOURCE: templates/recipe.gohtml:78-111 uses interactive control; the archive */ -}}
{{- /* list only needs static icons for set flags, e.g. */ -}}
{{- if .Liked }}<span aria-label="{{ t "recipe.feedback_like" }}">👍</span>{{ end }}
```

### Tests — repository round-trip + handler stub style
```go
// SOURCE: internal/repository/recipe_test.go (open a temp DB via testStore, insert, assert)
// SOURCE: internal/handler/disliked_test.go (stub the narrow interface, assert status + body)
// SOURCE: internal/service/generation_test.go:34-117 (fakeGenRepo + fakeBuilder + newTestGenService)
```

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `internal/repository/recipe.go` | UPDATE | Add `SearchRecipes(ctx, householdID, query, limit)` + `escapeLike` helper |
| `internal/repository/recipe_test.go` | UPDATE | Cover search: substring match, newest-first order, empty-query=all, LIKE-wildcard escaping, household scoping |
| `internal/service/generation.go` | UPDATE | Add `GetRecipe` to `generationRepo`; add `CookAgain(ctx, h, plan, oldRecipeID, sourceRecipeID)` |
| `internal/service/generation_test.go` | UPDATE | Add `GetRecipe` to `fakeGenRepo`; test `CookAgain` (copy tagged history, builder+swap called, bad oldID, missing source) |
| `internal/handler/archive.go` | CREATE | `archiveHandlers`: `Show`, `Search`, `CookAgainDialog`, `CookAgain`; view models + projections |
| `internal/handler/archive_test.go` | CREATE | Stub-backed tests: list renders newest-first + icons; search fragment; degradation banner on read error; dialog; cook-again success/no-plan |
| `templates/archive.gohtml` | CREATE | `archive/page` + `archive/content` + `archive/list` + `archive/dialog` + `archive/done` fragments |
| `templates/base.gohtml` | UPDATE | Add Archive link to the header nav |
| `internal/handler/router.go` | UPDATE | Construct `archiveHandlers`; register `GET /archive`, `GET /archive/search` always; register cook-again routes inside `if canGenerate` |
| `i18n/en.json` | UPDATE | Add `nav.archive`, `archive.*` keys |
| `i18n/fi.json` | UPDATE | Same keys, Finnish |
| `i18n/ru.json` | UPDATE | Same keys, Russian |

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Repository — `SearchRecipes`

- **File**: `internal/repository/recipe.go`
- **Action**: UPDATE
- **Implement**:
  - Add `func (s *Store) SearchRecipes(ctx context.Context, householdID, query string, limit int) ([]domain.Recipe, error)`.
  - Mirror `RecentRecipes` exactly (context timeout, `recipeColumns`, `scanRecipe`, `rows.Err`, error wrapping `"search recipes: %w"`).
  - SQL: `SELECT ` + recipeColumns + ` FROM recipe WHERE household_id = ? AND title LIKE ? ESCAPE '\' ORDER BY created_at DESC LIMIT ?`.
  - Build the pattern as `"%" + escapeLike(strings.TrimSpace(query)) + "%"` so an empty query matches all.
  - Add unexported `func escapeLike(s string) string` — replace `\`→`\\`, then `%`→`\%`, `_`→`\_` (backslash first).
- **Mirror**: `internal/repository/recipe.go:125-153`
- **Validate**: `gofmt -s -l . && go build ./... && go vet ./...`

### Task 2: Repository — search tests

- **File**: `internal/repository/recipe_test.go`
- **Action**: UPDATE
- **Implement**: Using the existing temp-DB test harness in this file, insert several recipes for one household (and one for a second household), then assert:
  - empty query returns all of household A's recipes, newest first (control `CreatedAt` via the insert path / sleep as existing tests do);
  - a substring matches case-insensitively per SQLite `LIKE` default (ASCII);
  - a query containing `%`/`_` is treated literally (escaping works);
  - results never include household B's recipes;
  - `limit` caps the result count.
- **Mirror**: existing cases in `internal/repository/recipe_test.go`
- **Validate**: `go test ./internal/repository/...`

### Task 3: Service — `CookAgain`

- **File**: `internal/service/generation.go`
- **Action**: UPDATE
- **Implement**:
  - Add `GetRecipe(ctx context.Context, id string) (*domain.Recipe, error)` to the `generationRepo` interface (`*repository.Store` already provides it).
  - Add:
    ```go
    func (g *GenerationService) CookAgain(ctx context.Context, h *domain.HouseholdProfile, plan *domain.WeeklyPlan, oldRecipeID, sourceRecipeID string) (*SwappedRecipe, error)
    ```
  - Wrap with `context.WithTimeout(ctx, generationTimeout)`.
  - `idx := indexOfString(plan.RecipeIDs, oldRecipeID)`; if `< 0` return `fmt.Errorf("cook again: %w", ErrGenerationInvalid)`.
  - Load source via `g.repo.GetRecipe(ctx, sourceRecipeID)` (wrap errors, including `repository.ErrNotFound`, as `"cook again: %w"`).
  - Build kept-ID slice (all plan IDs except idx); `kept, _ := g.repo.RecipesByIDs(ctx, keptIDs)` (wrap err).
  - Build the copy: `newRecipe := *source; newRecipe.ID = ""; newRecipe.HouseholdID = h.ID; newRecipe.Source = domain.SourceHistory; newRecipe.Feedback = nil`.
  - `allRecipes := append(append(make([]domain.Recipe,0,len(kept)+1), kept...), newRecipe)`; `list, err := g.builder.Build(ctx, allRecipes, h.PantryBasics)`.
  - `g.repo.SwapRecipeInPlan(ctx, plan.ID, oldRecipeID, &newRecipe, list)`; then `plan.RecipeIDs[idx] = newRecipe.ID; plan.ShoppingList = list`.
  - Return `&SwappedRecipe{Plan: plan, Recipe: newRecipe, Protein: inferProtein(newRecipe)}`.
- **Mirror**: `internal/service/generation.go:241-315` (SwapRecipe tail)
- **Validate**: `go build ./... && go vet ./...`

### Task 4: Service — `CookAgain` tests + fake update

- **File**: `internal/service/generation_test.go`
- **Action**: UPDATE
- **Implement**:
  - Add `GetRecipe` to `fakeGenRepo` (return a configurable recipe / `repository.ErrNotFound`).
  - Tests: (a) happy path — copy is inserted via `SwapRecipeInPlan` with `Source=history`, empty ID, `h.ID`, nil feedback; builder called with kept+copy; `plan.RecipeIDs[idx]` updated; (b) `oldRecipeID` not in plan ⇒ `ErrGenerationInvalid` and `SwapRecipeInPlan` not called; (c) source not found ⇒ error surfaced.
- **Mirror**: `internal/service/generation_test.go:34-118` and existing swap tests
- **Validate**: `go test ./internal/service/...`

### Task 5: Handler — archive screen

- **File**: `internal/handler/archive.go`
- **Action**: CREATE
- **Implement**:
  - Narrow interfaces:
    ```go
    type archiveLister interface {
    	SearchRecipes(ctx context.Context, householdID, query string, limit int) ([]domain.Recipe, error)
    }
    type cookAgainService interface { // satisfied by *service.GenerationService; nil when LLM off
    	CurrentPlan(ctx context.Context, householdID string) (*domain.WeeklyPlan, error)
    	CookAgain(ctx context.Context, h *domain.HouseholdProfile, plan *domain.WeeklyPlan, oldRecipeID, sourceRecipeID string) (*service.SwappedRecipe, error)
    }
    type recipeLoader interface { RecipesByIDs(...) } // reuse existing one in generate.go
    ```
  - `archiveHandlers struct { rd *renderer; recipes archiveLister; households householdProfiles; plans recipeLoader; cook cookAgainService; canCook bool }`.
  - `const archiveLimit = 200`.
  - View models: `archiveData{ Lang string; CanCook bool; Query string; LoadError bool; Recipes []archiveRow }`; `archiveRow{ ID, Title, Description string; CookTime int; Liked, Disliked, CookAgain bool }`; projection `toArchiveRow(domain.Recipe)`.
  - `Show`: load household (`fail` on error); `SearchRecipes(h.ID, "", archiveLimit)` — **on error, render the page with `LoadError: true` and empty list at HTTP 200** (graceful degradation, not `fail`); else render rows.
  - `Search`: load household; read `q := r.FormValue("q")`; `SearchRecipes(h.ID, q, archiveLimit)`; on error render the `archive/list` fragment with `LoadError: true`; else render `archive/list` fragment.
  - `CookAgainDialog` (`GET /archive/cook-again/{id}`): load household + `plans`/`cook.CurrentPlan`; if no current plan, render `archive/dialog` with a "no active week" state; else load the plan's 3 recipes via `RecipesByIDs` and render a dialog listing them as buttons that `hx-post /archive/cook-again/{sourceID}` with `hx-vals='{"old":"<currentRecipeID>"}'`.
  - `CookAgain` (`POST /archive/cook-again/{id}`): `sourceID := PathValue("id")`; `oldID := r.FormValue("old")`; load household + current plan (no plan ⇒ render dialog error fragment, 200); call `cook.CookAgain(...)`; on error log + render a localized error fragment (HTTP 200, HTMX-swappable — mirror `generate.renderError`); on success render `archive/done` (confirmation + link to `/`).
  - All localized error/detail handling mirrors existing handlers: log real error, never echo internals.
- **Mirror**: `internal/handler/disliked.go`, `internal/handler/generate.go:81-107,228-248`, `internal/handler/shopping.go:201-212`
- **Validate**: `go build ./... && go vet ./...`

### Task 6: Handler — archive tests

- **File**: `internal/handler/archive_test.go`
- **Action**: CREATE
- **Implement**: stub `archiveLister`, `householdProfiles`, `recipeLoader`, `cookAgainService`. Assert:
  - `Show` lists newest-first and renders feedback icons for set flags; `CanCook` toggles the button;
  - `Show` with a search-store error renders the page with the degradation banner at **200** (not 500);
  - `Search` returns the `archive/list` fragment for a query;
  - `CookAgainDialog` with no current plan renders the no-active-week state;
  - `CookAgain` success renders the done fragment; service error renders a localized error fragment at 200.
- **Mirror**: `internal/handler/disliked_test.go`, `internal/handler/generate_test.go`
- **Validate**: `go test ./internal/handler/...`

### Task 7: Template — archive

- **File**: `templates/archive.gohtml`
- **Action**: CREATE
- **Implement**: define `archive/page` (doctype + head + header + `#content` + htmx + sw-register, mirroring `disliked/page`), `archive/content` (heading, `<input type="search">` with `hx-get="/archive/search" hx-trigger="keyup changed delay:200ms, search" hx-target="#archive-list" hx-swap="outerHTML"`, then `{{ template "archive/list" . }}`), `archive/list` (`<ul id="archive-list">`: empty state, `LoadError` banner `role="alert"`, else rows with title/desc/cook-time, static feedback icons, recipe link, and — `{{ if $.CanCook }}` — a "Cook again" button `hx-get="/archive/cook-again/{{ .ID }}"` targeting a per-row dialog container), `archive/dialog` (lists the 3 current recipes as cook-again buttons, or a no-active-week message), `archive/done` (confirmation + link back to `/`). Every string via `{{ t "..." }}`; touch targets/sizes inherit the Nordic Kitchen CSS already in `static/css/app.css`.
- **Mirror**: `templates/disliked.gohtml`, `templates/generate.gohtml`, `templates/recipe.gohtml:78-111`
- **Validate**: `go test ./internal/handler/...` (templates are embedded + parsed; handler tests exercise them)

### Task 8: Template — main-menu nav link

- **File**: `templates/base.gohtml`
- **Action**: UPDATE
- **Implement**: in the `header` nav (`base.gohtml:14-17`), add an Archive link before Settings: `<a href="/archive" hx-get="/archive" hx-target="#content" hx-push-url="true" class="app-header__archive">{{ t "nav.archive" }}</a>`.
- **Mirror**: `templates/base.gohtml:15`
- **Validate**: `go test ./internal/handler/...`

### Task 9: Router — wire archive

- **File**: `internal/handler/router.go`
- **Action**: UPDATE
- **Implement**:
  - Construct `ah := &archiveHandlers{rd: rd, recipes: store, households: svc, plans: store, canCook: canGenerate}`.
  - Register always: `mux.HandleFunc("GET /archive", ah.Show)` and `mux.HandleFunc("GET /archive/search", ah.Search)`.
  - Inside the existing `if canGenerate {` block, after the generation service is built, set `ah.cook = gh.gen` (the `*GenerationService`) and register `mux.HandleFunc("GET /archive/cook-again/{id}", ah.CookAgainDialog)` and `mux.HandleFunc("POST /archive/cook-again/{id}", ah.CookAgain)`.
  - Note ordering: `ah` must be constructed before the `if canGenerate` block so its `cook` field can be assigned there.
- **Mirror**: `internal/handler/router.go:30-74`
- **Validate**: `go build ./... && go vet ./...`

### Task 10: i18n keys (all three languages)

- **File**: `i18n/en.json`, `i18n/fi.json`, `i18n/ru.json`
- **Action**: UPDATE
- **Implement**: add the same key set to each, translated: `nav.archive`, `archive.heading`, `archive.search_placeholder`, `archive.empty`, `archive.load_error`, `archive.cook_again`, `archive.dialog_heading` ("Replace which recipe?"), `archive.no_active_week`, `archive.done` ("Added to this week"), `archive.view_week`, `archive.error` (generic cook-again failure). Keep wording consistent with existing tone; reuse existing `recipe.feedback_*` keys for the icon aria-labels.
- **Mirror**: `i18n/en.json:51-57` (disliked block)
- **Validate**: `go test ./internal/i18n/...` (the bundle test asserts key parity across languages — keep all three in sync)

---

## Risks

| Risk | Mitigation |
|------|------------|
| Cook-again referencing an existing recipe ID would collide on PK insert | Copy the recipe (`ID=""`, `Source=history`) so `insertRecipe` assigns a fresh UUID — confirmed against `weeklyplan.go:128` / `recipe.go:30-33` |
| Shopping rebuild needs the LLM when an ingredient name misses the dictionary | Gate cook-again behind `canGenerate` (reuse `ShoppingBuilder`); listing/search/icons stay available without the LLM |
| Archive read failure 500-ing the whole page | `Show`/`Search` degrade to a 200 page/fragment with a localized banner (AC explicit) |
| `LIKE` wildcard injection from the search box | `escapeLike` + `ESCAPE '\'`; query is a bound parameter (no string concatenation into SQL) |
| i18n key drift across ru/fi/en | Add identical keys to all three; `internal/i18n` bundle test enforces parity |
| `generationRepo` interface change breaks the fake | Update `fakeGenRepo` with `GetRecipe` in the same change (Task 4) |
| Cook-again `SwappedRecipe.Protein` for a stored recipe (no protein column) | Use the existing `inferProtein` keyword heuristic, same as swap kept-cards |

## Environment & Verification

| Verification | Runs in env? | If blocked: where/when verified |
|--------------|--------------|---------------------------------|
| `gofmt -s -l .` | yes | — |
| `go vet ./...` | yes | — |
| `golangci-lint run ./...` | yes | — |
| `go test ./...` (incl. new repo/service/handler tests) | yes | — |
| Static `CGO_ENABLED=0 go build` | yes | — |
| `govulncheck ./...` | no (vuln.go.dev 403 in sandbox) | networked host / CH-21 deploy gate |
| Service-Worker / PWA behavior over HTTPS | no (needs tailnet HTTPS) | Mac mini tailnet / CH-21 |
| Live LLM cook-again shopping rebuild (only on dictionary miss) | maybe (egress varies) | re-probe at run time; else defer to networked host / CH-21 |

---

## Validation

```bash
gofmt -s -l .            # formatting (no output = clean)
go vet ./...             # vet
golangci-lint run ./...  # lint
go test ./...            # tests
# govulncheck ./...       # deferred in sandbox (vuln.go.dev 403) — gated at CH-21
```

---

## Acceptance Criteria

- [ ] Archive screen reachable from the main-menu header nav (`nav.archive` link)
- [ ] All household recipes listed newest-first (`SearchRecipes`, `ORDER BY created_at DESC`)
- [ ] Substring title search via HTMX with ~200ms debounce (`keyup changed delay:200ms`)
- [ ] "Cook again" adds the recipe to the current `WeeklyPlan`, replacing a user-chosen one of the 3 (dialog), and rebuilds the shopping list (reuses `SwapRecipeInPlan` + `ShoppingBuilder`)
- [ ] Feedback icons visible in the list (read-only, per set flag)
- [ ] Archive read error degrades gracefully (in-page banner, HTTP 200 — no whole-page 500)
- [ ] All tasks completed; `go build`/`go vet`/`golangci-lint`/`go test` pass
- [ ] i18n keys present and in sync across ru/fi/en
- [ ] Follows existing handler/service/repository/template patterns
- [ ] Environment-blocked checks (`govulncheck`, SW-over-HTTPS, live LLM) recorded for the CH-21 deploy gate
```