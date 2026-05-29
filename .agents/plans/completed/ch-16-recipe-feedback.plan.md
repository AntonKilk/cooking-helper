# Plan: CH-16 Recipe Feedback Collection (F-5)

## Summary

Add like / dislike / cook-again feedback to recipes. The domain model
(`domain.Feedback`) and the full repository persistence layer
(`feedback_*` columns, `UpdateRecipe`, `scanFeedback`) **already exist** — this
story wires the **service → handler → UI** path on top of them. We add a thin
`RecipeService.SetFeedback` (read-modify-write with timestamp + clear-on-empty
rules), a single idempotent `POST /recipe/{id}/feedback` endpoint, and a shared
`recipe/feedback` template fragment rendered in **two** places: the fullscreen
recipe detail view and the current-week recipe cards. State is persisted with a
timestamp and is freely changeable. Each button posts the **absolute** desired
state of all three flags (not a toggle) so a Service-Worker replay is a no-op —
mirroring the shopping checkbox pattern exactly.

## User Story

As a household cook
I want to mark a recipe liked / disliked / cook-again after cooking it
So that the system learns our tastes (feeding CH-17's next-generation prompt)

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY (thin slice over existing persistence) |
| Complexity | LOW–MEDIUM |
| Systems Affected | `internal/service`, `internal/handler`, `templates`, `i18n`, `static/css` |
| GitHub Issue | #16 (CH-16) |

---

## Scope & Boundaries (read first)

- **Delivered now:** feedback controls on (1) the recipe **detail** view
  (`recipe/content`) and (2) the **current-week cards** (`generate/cards`), the
  `POST /recipe/{id}/feedback` endpoint, the `RecipeService.SetFeedback`
  read-modify-write, i18n in RU/FI/EN, and CSS.
- **Archive icons (AC bullet 4) are partially out of scope.** The recipe
  **archive UI does not exist yet — it is CH-18** (`stories.md:506`). The shared
  `recipe/feedback` fragment is built to be **reused as-is by CH-18**, which
  satisfies "feedback icons visible in the archive" when the archive lands.
  Record this in the deferral table; do **not** build an archive screen here.
- **Existing behavior:** the home `#week` section is empty on initial page load
  and is populated only by the `POST /generate` / `POST /generate/swap`
  fragments (`templates/home.gohtml:37`). CH-16 adds feedback to the cards those
  fragments render; it does **not** add a current-week-on-load view (that is not
  part of this story).
- **Idempotency (Technical Note):** absolute state, never a server-side toggle —
  replay-safe. This is a hard requirement from the issue.

---

## Patterns to Follow

### Narrow handler interface + idempotent POST that re-renders a fragment
```go
// SOURCE: internal/handler/shopping.go:13-20, 138-160
type shoppingStore interface {
	CurrentWeeklyPlan(ctx context.Context, householdID string) (*domain.WeeklyPlan, error)
	GetShoppingItem(ctx context.Context, id string) (*domain.ShoppingListItem, error)
	SetShoppingItemChecked(ctx context.Context, id string, checked bool) error
	...
}
// Check persists the ABSOLUTE checked state (idempotent; SW-replay safe),
// treats a vanished row as a benign 200 no-op, then re-renders the row partial.
func (sh *shoppingHandlers) Check(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" { http.Error(w, "bad request", http.StatusBadRequest); return }
	checked := r.FormValue("checked") == "true"
	if err := sh.store.SetShoppingItemChecked(r.Context(), id, checked); err != nil {
		if errors.Is(err, repository.ErrNotFound) { w.WriteHeader(http.StatusOK); return }
		sh.rd.fail(w, r, "set shopping item checked", err); return
	}
	sh.renderItem(w, r, id)
}
```

### Service constructor + narrow repo dependency (mirror for RecipeService)
```go
// SOURCE: internal/handler/router.go:32-38 (wiring) + service.NewHouseholdService(store)
store := repository.New(db)
svc := service.NewHouseholdService(store)
rh := &recipeHandlers{rd: rd, recipes: store}
```

### Fragment rendering with request-scoped t()
```go
// SOURCE: internal/handler/render.go:72-91 ; internal/handler/shopping.go:180,211
sh.rd.renderFragment(w, r, http.StatusOK, "shopping/item", toShoppingItemView(*item))
```

### View-model projection (keep domain/sql/HTTP types out of templates)
```go
// SOURCE: internal/handler/recipe.go:36-52, 80-98
type recipeView struct { ID, Title, Description string; CookTime, Servings int; ... }
func toRecipeView(r *domain.Recipe) *recipeView { ... }
```

### Repository read-modify-write already available (reuse — do NOT add SQL)
```go
// SOURCE: internal/repository/recipe.go:59-73, 204-230, 254-283
func (s *Store) GetRecipe(ctx, id) (*domain.Recipe, error)         // ErrNotFound on miss
func (s *Store) UpdateRecipe(ctx, *domain.Recipe) error            // persists Feedback + bumps updated_at
// feedbackColumns / scanFeedback already map *domain.Feedback (nil => all-NULL).
```

### Tests: narrow stub + compile-time guard + table-driven 3-language render
```go
// SOURCE: internal/handler/shopping_test.go:18-72, 185-225 ; recipe_test.go:189-194
var _ shoppingStore = (*repository.Store)(nil)   // real store satisfies interface
var _ shoppingStore = (*stubShoppingStore)(nil)  // stub satisfies interface
// idempotency test: apply checked=true twice, assert state + rendered class both times.
// SOURCE: internal/handler/language_test.go:39-85 (testRecipes, testTemplates, newTestRouter)
```

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `internal/service/recipe.go` | CREATE | `RecipeService` + `SetFeedback` read-modify-write (timestamp, clear-on-all-false) |
| `internal/service/recipe_test.go` | CREATE | unit tests for `SetFeedback` (set/change/clear, load + save errors) |
| `internal/handler/recipe.go` | UPDATE | add `feedbackSetter` dep, `Feedback` handler, `feedbackView`, populate feedback in `recipeView` |
| `internal/handler/generate.go` | UPDATE | add feedback fields to `recipeCard`; populate from `domain.Recipe.Feedback` in Generate + Swap |
| `internal/handler/router.go` | UPDATE | wire `service.NewRecipeService(store)` into `recipeHandlers`; register `POST /recipe/{id}/feedback` |
| `templates/recipe.gohtml` | UPDATE | define `recipe/feedback` fragment; render it in `recipe/content` |
| `templates/generate.gohtml` | UPDATE | render the `recipe/feedback` fragment (icon variant) inside each card |
| `i18n/en.json` | UPDATE | feedback strings (EN) |
| `i18n/fi.json` | UPDATE | feedback strings (FI) |
| `i18n/ru.json` | UPDATE | feedback strings (RU) |
| `static/css/app.css` | UPDATE | `.feedback` BEM block (44×44 targets, terracotta active state) |
| `internal/handler/recipe_test.go` | UPDATE | feedback endpoint tests (set, idempotent replay, render in detail) + update guards |
| `internal/handler/language_test.go` | UPDATE | give `recipeHandlers` a feedback stub in `newTestRouter`; add a fixture recipe with feedback |
| `internal/handler/generate_test.go` | UPDATE | assert feedback control renders on cards (only if it adds value; fields are additive) |

---

## Design Detail (the contract every task must honor)

**Endpoint:** `POST /recipe/{id}/feedback`
**Form fields (absolute, not toggle):** `liked`, `disliked`, `cook_again` — each
`"true"` or absent/`"false"`. The button the user taps carries, via `hx-vals`,
the **full** desired tri-state computed in the template from the current
feedback (the unchanged two + the flipped one). Replaying the same POST writes
the same state → idempotent.

**`RecipeService.SetFeedback(ctx, recipeID string, fb domain.Feedback) (*domain.Recipe, error)`**
1. `GetRecipe` (propagate `ErrNotFound`).
2. If all three flags are false → set `rec.Feedback = nil` (clears the row's
   feedback). Else set `rec.Feedback = &fb` with `fb.CreatedAt = time.Now().UTC()`
   (timestamp reflects the most recent reaction — CH-17 orders by recency).
3. `UpdateRecipe`; return the reloaded/updated recipe.

> like/dislike are kept **independent** booleans per the story ("три независимых
> булевых поля") — do not auto-clear one when the other is set.

**Handler `Feedback`:** parse `id` (blank → 400), parse three bools from the
form, call `SetFeedback`, on `ErrNotFound` return a benign `200` (SW replay of a
write for a deleted recipe), on other error `rd.fail` (500, detail logged only).
On success render the `recipe/feedback` fragment with the updated state.

**Shared fragment `recipe/feedback`** takes a `feedbackView{ RecipeID string;
Liked, Disliked, CookAgain bool }`. It renders three `<button>`s
(👍 / 👎 / 🔁) each with:
- `hx-post="/recipe/{{.RecipeID}}/feedback"`, `hx-target="closest .feedback"`,
  `hx-swap="outerHTML"`;
- `hx-vals` = the full desired tri-state with this button's flag flipped;
- `aria-pressed="true|false"` and a `feedback__btn--active` class when the flag
  is set; localized `aria-label`/text via `t`.

Wrap in `<form class="feedback" ...>` or `<div class="feedback">` so
`closest .feedback` resolves and the same markup works on a card and on the
detail page.

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: RecipeService.SetFeedback
- **File**: `internal/service/recipe.go`
- **Action**: CREATE
- **Implement**: `RecipeService` struct with a narrow repo interface
  (`GetRecipe`, `UpdateRecipe`), `NewRecipeService(repo) *RecipeService`, and
  `SetFeedback` per the Design Detail (load → clear-on-all-false / set with
  `time.Now().UTC()` → update → return). Wrap errors `fmt.Errorf("set feedback: %w", err)`.
- **Mirror**: `internal/service/household.go` (constructor + repo-interface shape); timestamp/UTC like `internal/repository/recipe.go:34,209`.
- **Validate**: `gofmt -s -l . && go vet ./... && go build ./...`

### Task 2: RecipeService unit tests
- **File**: `internal/service/recipe_test.go`
- **Action**: CREATE
- **Implement**: in-memory repo stub; assert (a) setting `Liked` persists with a
  non-zero `CreatedAt`; (b) changing flags later updates state (not final);
  (c) all-false clears `Feedback` to nil; (d) `GetRecipe` ErrNotFound and
  `UpdateRecipe` error propagate wrapped. Add compile-time guard that
  `*repository.Store` satisfies the service's repo interface.
- **Mirror**: `internal/service/household_test.go`, stub style from `shopping_test.go:18-72`.
- **Validate**: `go test ./internal/service/...`

### Task 3: Feedback handler + view models
- **File**: `internal/handler/recipe.go`
- **Action**: UPDATE
- **Implement**: add `feedbackSetter` interface
  (`SetFeedback(ctx, id string, fb domain.Feedback) (*domain.Recipe, error)`)
  and a `feedback feedbackSetter` field on `recipeHandlers`. Add
  `feedbackView{RecipeID string; Liked, Disliked, CookAgain bool}` + a
  `toFeedbackView(r *domain.Recipe)` helper (nil Feedback → all false). Add the
  three feedback bools to `recipeView` (populate in `toRecipeView`). Add
  `Feedback(w, r)` handler per Design Detail (blank id → 400, parse 3 bools,
  call service, ErrNotFound → 200 no-op, other err → `rd.fail`, success →
  `renderFragment(... "recipe/feedback", toFeedbackView(updated))`).
- **Mirror**: `internal/handler/shopping.go:143-160` (idempotent POST + benign-404 + re-render fragment).
- **Validate**: `go build ./... && go vet ./...`

### Task 4: Feedback fields on the week cards
- **File**: `internal/handler/generate.go`
- **Action**: UPDATE
- **Implement**: add `Liked, Disliked, CookAgain bool` to `recipeCard`; in
  `Generate` populate them from `week.Recipes[i].Feedback`; in `Swap` populate
  the new card as no-feedback and kept cards from their loaded
  `domain.Recipe.Feedback`. Add a `feedbackView` accessor or inline fields the
  card template can pass to the `recipe/feedback` partial.
- **Mirror**: existing `recipeCard` build at `internal/handler/generate.go:91-101, 157-178`.
- **Validate**: `go build ./...`

### Task 5: Router wiring + route
- **File**: `internal/handler/router.go`
- **Action**: UPDATE
- **Implement**: `rh := &recipeHandlers{rd: rd, recipes: store, feedback: service.NewRecipeService(store)}`;
  register `mux.HandleFunc("POST /recipe/{id}/feedback", rh.Feedback)` in the
  **unconditional** block (feedback must work without an LLM wired).
- **Mirror**: `internal/handler/router.go:37,43-47`.
- **Validate**: `go build ./...`

### Task 6: Templates — shared feedback fragment + detail + cards
- **File**: `templates/recipe.gohtml`, `templates/generate.gohtml`
- **Action**: UPDATE
- **Implement**: in `recipe.gohtml` define `{{ define "recipe/feedback" }}` per
  Design Detail and render it inside `recipe/content` (e.g. below the meta line,
  integrated — not an overlay, per Technical Note). In `generate.gohtml` render
  the same fragment inside each `<li class="recipe-card">` (outside the `<a>` so
  taps don't navigate), passing the card's feedback fields. Use emoji 👍/👎/🔁
  (no `<img>` — emoji + typography per CLAUDE.md). Keep `aria-pressed` + active
  class + `hx-vals` flip logic.
- **Mirror**: `templates/recipe.gohtml:48-58` (data-attr + aria-pressed buttons), `templates/generate.gohtml:5-18` (card + hx-post pattern).
- **Validate**: `go test ./internal/handler/... -run Recipe`

### Task 7: i18n strings (RU/FI/EN)
- **File**: `i18n/en.json`, `i18n/fi.json`, `i18n/ru.json`
- **Action**: UPDATE
- **Implement**: add the same keys to all three files (keep key order/style):
  - `recipe.feedback_heading` — EN "How was it?" · FI "Miten meni?" · RU "Как получилось?"
  - `recipe.feedback_like` — EN "Like" · FI "Tykkää" · RU "Нравится"
  - `recipe.feedback_dislike` — EN "Dislike" · FI "En tykkää" · RU "Не нравится"
  - `recipe.feedback_cook_again` — EN "Cook again" · FI "Tee uudelleen" · RU "Готовить снова"
- **Mirror**: `i18n/en.json:13-24`. Note `internal/i18n/bundle_test.go` likely
  asserts key-parity across languages — all three files must match.
- **Validate**: `go test ./internal/i18n/...`

### Task 8: CSS — Nordic Kitchen feedback control
- **File**: `static/css/app.css`
- **Action**: UPDATE
- **Implement**: `.feedback`, `.feedback__btn`, `.feedback__btn--active`
  (terracotta `#C2603A` active fill/border), ≥44×44px targets, ≥18pt, no
  hover-only affordance, respect existing dark-mode/`prefers-color-scheme`
  conventions already in the file.
- **Mirror**: existing `.recipe-card__swap` / `.recipe-step__toggle` rules in `static/css/app.css`.
- **Validate**: `gofmt -s -l .` (CSS not compiled; visual check deferred — see table)

### Task 9: Handler & helper tests
- **File**: `internal/handler/recipe_test.go`, `internal/handler/language_test.go`, `internal/handler/generate_test.go`
- **Action**: UPDATE
- **Implement**: add `stubFeedbackSetter` (records last `fb`, returns updated
  recipe or injected err); update `newRecipeHandler`, `newTestRouter`, and any
  `recipeHandlers{...}` literal to set `feedback`. Add a fixture recipe with
  `Feedback` to `testRecipes`. New tests:
  - `POST /recipe/{id}/feedback` with `liked=true` → 200, body contains
    `feedback__btn--active` and `aria-pressed="true"`;
  - **idempotent replay**: same POST twice → both 200, same state (mirror
    `TestShoppingCheckPersistsAndIsIdempotent`);
  - missing recipe → benign 200;
  - service error → 500 with no internal detail leaked;
  - detail view (`GET /recipe/abc123`) now contains the feedback controls;
  - (cards) optional: card fragment contains the feedback control.
  Update compile-time guards: `var _ feedbackSetter = (*service.RecipeService)(nil)`.
- **Mirror**: `internal/handler/shopping_test.go:185-225`, `recipe_test.go:28-61`.
- **Validate**: `go test ./...`

---

## Risks

| Risk | Mitigation |
|------|------------|
| Toggle semantics break SW-replay idempotency | Endpoint takes **absolute** tri-state via `hx-vals`; never flip server-side. Add an explicit double-POST idempotency test (Task 9). |
| i18n key-parity test fails (missing key in one language) | Add all 4 keys to all 3 files in the same task (Task 7); run `go test ./internal/i18n/...`. |
| Card feedback button taps trigger card navigation (`<a>` wraps card) | Render `.feedback` **outside** the `<a class="recipe-card__link">`, as a sibling like `recipe-card__swap` already is (`generate.gohtml:12`). |
| Existing `recipeHandlers{...}` literals fail to compile after adding `feedback` field | Sweep all construction sites: `router.go`, `language_test.go:78`, `recipe_test.go:19` (Tasks 5 & 9). |
| "Archive icons" AC cannot be fully met (no archive UI) | Out of scope — CH-18 builds the archive and reuses this fragment. Recorded in deferral table. |
| Clearing feedback to nil on all-false loses the timestamp/record | Intended: nil ⇒ all-NULL columns (matches `feedbackColumns`); "no opinion" is a valid state and is changeable again later. |

## Environment & Verification

| Verification | Runs in env? | If blocked: where/when verified |
|--------------|--------------|---------------------------------|
| `gofmt -s -l .`, `go vet ./...` | yes | — |
| `go build ./...` (incl. `CGO_ENABLED=0` static) | yes | — |
| `go test ./...` (handler/service/i18n incl. 3-language render + idempotency) | yes | — |
| `golangci-lint run ./...` | yes | — |
| `govulncheck ./...` | no | `vuln.go.dev` 403 in sandbox → CH-21 / networked host |
| Visual CSS / 44px targets / dark-mode on iPad Safari | no | tailnet HTTPS on Mac mini (CH-21 deploy gate) |
| Service-Worker offline feedback-queue replay (HTTPS) | no | tailnet HTTPS; idempotency proven at HTTP level by Go test |
| Archive feedback icons (AC bullet 4) | n/a | **CH-18** (archive UI reuses `recipe/feedback`) |

---

## Validation

```bash
gofmt -s -l .            # formatting (no output = clean)
go vet ./...             # vet
golangci-lint run ./...  # lint
go test ./...            # tests
# govulncheck ./...      # DEFERRED in sandbox (vuln.go.dev 403) -> CH-21
```

---

## Acceptance Criteria

- [ ] Three independent boolean controls on the **card** and the **detail** view
- [ ] State persists to `Recipe.feedback` with a timestamp (`SetFeedback` sets `CreatedAt`)
- [ ] Feedback is changeable later (absolute writes; all-false clears) — not a final action
- [ ] Current-week cards show feedback icons (active state styled)
- [ ] Endpoint is idempotent (SW-replay safe) — proven by a double-POST test
- [ ] RU/FI/EN strings added (i18n key-parity test passes)
- [ ] `gofmt`, `go vet`, `golangci-lint`, `go test` all pass
- [ ] Archive icons (AC bullet 4) recorded as delivered via CH-18 (fragment reused)
- [ ] Environment-blocked checks (`govulncheck`, SW/HTTPS, iPad visual) recorded for CH-21
```
