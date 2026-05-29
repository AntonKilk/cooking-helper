# Plan: Disliked Ingredients Management (CH-15 / F-7)

## Summary

Add a **disliked-ingredients management screen** reachable from Settings. The user
can add a disliked ingredient via a text field (with autosuggest sourced from past
recipe ingredients), and remove an existing one with a per-item button. The list is
already stored on `HouseholdProfile.DislikedIngredients`, already passed into every
generation/swap prompt as a hard constraint, and already post-validated (CH-10) — so
this story is the **CRUD/UI surface only**. We add two service methods
(`AddDisliked` / `RemoveDisliked`), a `dislikedHandlers` set with three routes
(show / add / remove) rendering HTMX fragments in the established server-side-render
style, a `disliked.gohtml` template, a Settings hub link, and i18n strings in ru/fi/en.
No migration and no prompt change is required.

## User Story

As a household member
I want to maintain a list of ingredients I dislike from Settings
So that they never appear in a generated weekly menu.

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY (UI surface over existing data) |
| Complexity | LOW |
| Systems Affected | `internal/service` (household), `internal/handler`, `templates/`, `i18n/`, `router.go` |
| GitHub Issue | #15 (CH-15) |

---

## Existing wiring already satisfying acceptance criteria

These criteria need **no new code** — verify, don't rebuild:

- "Список передаётся в промпт каждой генерации как hard constraint" — `internal/service/generation.go:358,423` pass `h.DislikedIngredients` into the prompt; CH-10 post-validation (`dislikeViolations`, `:160`,`:280`) enforces exclusion with retry.
- "Изменения применяются к следующей генерации" — `generateHandlers` calls `households.Current(...)` per request, so a freshly persisted list is read on the next generate.

The remaining criteria (screen reachable from settings, add via text field w/ autosuggest, remove via button, empty list valid) are the work below.

---

## Patterns to Follow

### Narrow handler interface + handler struct
```go
// SOURCE: internal/handler/profile.go:16-26
type householdProfiles interface {
	Current(ctx context.Context, defaultLang domain.Language) (*domain.HouseholdProfile, error)
	UpdateProfile(ctx context.Context, id string, lang domain.Language, adults, kids int) (*domain.HouseholdProfile, error)
}
type profileHandlers struct {
	rd     *renderer
	bundle *i18n.Bundle
	svc    householdProfiles
}
```

### Service method: load → mutate → persist, wrap errors
```go
// SOURCE: internal/service/household.go:71-87
func (s *HouseholdService) UpdateProfile(ctx context.Context, id string, lang domain.Language, adults, kids int) (*domain.HouseholdProfile, error) {
	if adults < minAdults || adults > maxAdults || kids < minKids || kids > maxKids {
		return nil, ErrInvalidFamilySize
	}
	h, err := s.repo.GetHousehold(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("update profile: %w", err)
	}
	h.Language = lang
	h.FamilySize = domain.FamilySize{Adults: adults, Kids: kids}
	if err := s.repo.UpdateHousehold(ctx, h); err != nil {
		return nil, fmt.Errorf("update profile: %w", err)
	}
	return h, nil
}
```

### HTMX action handler: validate path/form → call store → render fragment
```go
// SOURCE: internal/handler/shopping.go:165-181 (Remove)
func (sh *shoppingHandlers) Remove(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := sh.store.SetShoppingItemRemoved(r.Context(), id, true); err != nil { ... }
	sh.rd.renderFragment(w, r, http.StatusOK, "shopping/removed", shoppingItemView{ID: id})
}
```

### Template page/content pair + HTMX item with hx-post button
```gohtml
{{/* SOURCE: templates/shopping.gohtml:63-67 */}}
<button type="button" class="shopping-item__remove"
        hx-post="/shopping/item/{{ .ID }}/remove"
        hx-target="closest .shopping-item"
        hx-swap="outerHTML"
        aria-label="{{ t "shopping.remove" }}">{{ t "shopping.remove" }}</button>
```

### Handler-level projection helper (precedent for distinct-name extraction in handler)
```go
// SOURCE: internal/handler/shopping.go:99-116 (groupShoppingItems) — pure projection lives in handler, not service
```

### Service test with in-memory fake repo
```go
// SOURCE: internal/service/household_test.go:12-50 (fakeRepo) — reuse for AddDisliked/RemoveDisliked tests
```

### Handler test with stub + httptest router
```go
// SOURCE: internal/handler/profile_test.go:37-54 (newProfileRouter / defaultStub)
```

---

## Design decisions

- **Remove via button, not swipe.** The codebase has no swipe interaction anywhere; the shopping list uses an `hx-post` button. AC says "свайп **или** кнопку" — button satisfies it and stays consistent. No new JS.
- **Identity is the term string, not an ID.** Disliked entries are bare strings. The remove route carries the term as a form value (`POST /settings/disliked/remove`, body `ingredient=<term>`), not a path id (terms contain spaces/unicode).
- **Re-render the whole list fragment** (`disliked/list`) after add/remove, targeting `#disliked-list` with `hx-swap="outerHTML"`. Simpler than per-row surgery given string identity.
- **Autosuggest via native `<datalist>`** populated from distinct ingredient names of recent recipes (`store.RecentRecipes`). No JS, works in iPad Safari. Already-disliked terms are excluded from suggestions.
- **Normalization:** trim the input; reject empty (localized error, HTTP 400, like profile range error). Dedup case-insensitively (compare `strings.ToLower(strings.TrimSpace(...))`); store the trimmed original-case term. Removal matches case-insensitively. Adding a duplicate is a no-op success (idempotent).
- **Empty list is valid** — no minimum; the screen shows an empty-state line.

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `internal/service/household.go` | UPDATE | Add `AddDisliked` / `RemoveDisliked`; add `ErrEmptyIngredient`; helper to normalize/dedup. |
| `internal/service/household_test.go` | UPDATE | Unit tests for add (new, dup, empty), remove (present, absent, case-insensitive), empty-list validity. |
| `internal/handler/disliked.go` | CREATE | `dislikedHandlers` (Show/Add/Remove) + view model + `distinctIngredientSuggestions` projection. |
| `internal/handler/disliked_test.go` | CREATE | Handler tests: show renders list+datalist, add persists & re-renders, empty add → 400, remove re-renders. |
| `internal/handler/router.go` | UPDATE | Construct `dislikedHandlers`; register `GET/POST /settings/disliked` and `POST /settings/disliked/remove`. |
| `templates/disliked.gohtml` | CREATE | `disliked/page`, `disliked/content`, `disliked/list` fragment defines. |
| `templates/settings.gohtml` | UPDATE | Add a section/link to `/settings/disliked` in the settings hub. |
| `internal/handler/settings_test.go` | UPDATE | Assert settings page links to `/settings/disliked`. |
| `i18n/en.json` | UPDATE | Add `settings.disliked`, `disliked.*` keys. |
| `i18n/ru.json` | UPDATE | Same keys, Russian. |
| `i18n/fi.json` | UPDATE | Same keys, Finnish. |

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Service — Add/Remove disliked ingredients

- **File**: `internal/service/household.go`
- **Action**: UPDATE
- **Implement**:
  - Add `var ErrEmptyIngredient = errors.New("service: ingredient is empty")`.
  - `AddDisliked(ctx, id, term string) (*domain.HouseholdProfile, error)`: trim term; if empty return `ErrEmptyIngredient`; `GetHousehold`; if no existing entry matches case-insensitively, append the trimmed term; `UpdateHousehold`; return profile. Duplicate (case-insensitive) → no append, still persist-free success (return current profile, nil). Wrap errors `fmt.Errorf("add disliked: %w", err)`.
  - `RemoveDisliked(ctx, id, term string) (*domain.HouseholdProfile, error)`: trim term; `GetHousehold`; rebuild slice excluding entries equal case-insensitively to term; `UpdateHousehold` (only if changed, but unconditional update is fine and simpler — keep it); return profile. Wrap `fmt.Errorf("remove disliked: %w", err)`.
  - Add unexported helper `equalFoldTrim(a, b string) bool` or inline `strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))`.
- **Mirror**: `internal/service/household.go:71-87` (UpdateProfile load→mutate→persist), `internal/service/household.go:14` (error var).
- **Validate**: `go build ./... && go vet ./...`

### Task 2: Service tests

- **File**: `internal/service/household_test.go`
- **Action**: UPDATE
- **Implement**: Seed a household (via `Current`) on `fakeRepo`, then:
  - `AddDisliked` new term → present in `DislikedIngredients`.
  - `AddDisliked` duplicate (different case) → length unchanged.
  - `AddDisliked` blank/whitespace → `errors.Is(err, ErrEmptyIngredient)`, list unchanged.
  - `RemoveDisliked` existing (mixed case) → gone.
  - `RemoveDisliked` absent term → no error, list unchanged.
  - Removing the last term leaves an empty (non-nil-or-nil, both valid) slice — assert len 0.
- **Mirror**: `internal/service/household_test.go:12-69`.
- **Validate**: `go test ./internal/service/...`

### Task 3: Disliked handlers

- **File**: `internal/handler/disliked.go`
- **Action**: CREATE
- **Implement**:
  - Interface `dislikedProfiles` with `Current`, `AddDisliked`, `RemoveDisliked`.
  - Interface `recipeHistory` with `RecentRecipes(ctx, householdID string, limit int) ([]domain.Recipe, error)` (satisfied by `*repository.Store`).
  - Struct `dislikedHandlers{ rd *renderer; profiles dislikedProfiles; history recipeHistory }`.
  - View model `dislikedData{ Lang string; Items []string; Suggestions []string; Error string }` and a list-fragment model (can reuse `dislikedData` for the `disliked/list` fragment — it only needs `Items`).
  - `Show`: `profiles.Current` → gather suggestions via `history.RecentRecipes(ctx, h.ID, 20)` then `distinctIngredientSuggestions(recipes, h.DislikedIngredients)`; render `disliked` page. If `RecentRecipes` errors, **degrade gracefully**: log via `rd.fail`? No — per CLAUDE.md graceful degradation, render the screen with empty suggestions instead of 500. Catch the error, set suggestions nil, continue.
  - `Add`: read `ingredient` form value; call `profiles.AddDisliked`; on `ErrEmptyIngredient` re-render the list fragment with an inline error (HTTP 400 via `renderStatus`/`renderFragment` with status) — or simplest: re-render `disliked/content` with `Error: "disliked.error_empty"` at 400. Match profile's invalid pattern. On success render the `disliked/list` fragment (200) so HTMX swaps the updated list.
  - `Remove`: read `ingredient` form value; call `profiles.RemoveDisliked`; render `disliked/list` fragment.
  - Helper `distinctIngredientSuggestions(recipes []domain.Recipe, disliked []string) []string`: collect ingredient names, dedup case-insensitively, drop any already in `disliked` (case-insensitive), sort for stable output.
- **Mirror**: `internal/handler/shopping.go:165-211` (action handlers + fragment render + graceful ErrNotFound), `internal/handler/profile.go:38-49` (Show), `internal/handler/shopping.go:99-116` (projection helper).
- **Validate**: `go build ./...`

### Task 4: Templates — disliked screen

- **File**: `templates/disliked.gohtml`
- **Action**: CREATE
- **Implement**: Three defines mirroring `settings.gohtml`/`shopping.gohtml`:
  - `disliked/page`: full doc, `head`, `header`, `<main id="content">` → `disliked/content`, htmx script, `sw-register`.
  - `disliked/content`: `<h1>{{ t "disliked.heading" }}</h1>`; optional `<p role="alert">{{ t .Error }}</p>`; an **add form** — `<form hx-post="/settings/disliked" hx-target="#disliked-list" hx-swap="outerHTML">` with `<input name="ingredient" list="disliked-suggestions" ...>`, a `<datalist id="disliked-suggestions">` ranging `.Suggestions`, and a submit button `{{ t "disliked.add_button" }}`; then `{{ template "disliked/list" . }}`.
  - `disliked/list`: `<ul id="disliked-list" class="disliked-list">` (the `hx-target`); empty-state `<li>{{ t "disliked.empty" }}</li>` when `len .Items` is 0; else range `.Items` → `<li>` with the term text and a remove `<form hx-post="/settings/disliked/remove" hx-target="#disliked-list" hx-swap="outerHTML"><input type="hidden" name="ingredient" value="{{ . }}"><button type="submit" aria-label="{{ t "disliked.remove" }}">{{ t "disliked.remove" }}</button></form>`.
  - Note: `disliked/list` must carry `id="disliked-list"` on its root element so `hx-swap="outerHTML"` replaces it in place.
- **Mirror**: `templates/settings.gohtml:1-34`, `templates/shopping.gohtml:48-69`.
- **Validate**: covered by handler tests in Task 5 (template parse + render).

### Task 5: Handler tests

- **File**: `internal/handler/disliked_test.go`
- **Action**: CREATE
- **Implement**: Stub implementing `dislikedProfiles` (records add/remove calls, holds a `*domain.HouseholdProfile`) and a stub `recipeHistory` returning a couple of recipes. Router helper like `newProfileRouter`. Tests:
  - Show → 200, body contains existing disliked terms, contains `<datalist`, suggestion from history present, already-disliked term NOT in datalist.
  - POST add valid → 200, AddDisliked called with the term, body contains new term.
  - POST add blank → 400, AddDisliked not called, body contains the localized empty error.
  - POST remove → 200, RemoveDisliked called, term absent from body.
- **Mirror**: `internal/handler/profile_test.go:16-106`, `internal/handler/settings_test.go`.
- **Validate**: `go test ./internal/handler/...`

### Task 6: Wire routes

- **File**: `internal/handler/router.go`
- **Action**: UPDATE
- **Implement**: After `sh := ...`, add `dh := &dislikedHandlers{rd: rd, profiles: svc, history: store}`. Register:
  - `mux.HandleFunc("GET /settings/disliked", dh.Show)`
  - `mux.HandleFunc("POST /settings/disliked", dh.Add)`
  - `mux.HandleFunc("POST /settings/disliked/remove", dh.Remove)`
  (Place the `/remove` route distinct from `POST /settings/disliked` — Go 1.22+ mux treats them as separate patterns.)
- **Mirror**: `internal/handler/router.go:33-51`.
- **Validate**: `go build ./... && go test ./internal/handler/...`

### Task 7: Settings hub link

- **File**: `templates/settings.gohtml`
- **Action**: UPDATE
- **Implement**: Add a `<section>` (between profile and language) with an `<a href="/settings/disliked" hx-get=... hx-target="#content" hx-push-url="true">{{ t "settings.disliked" }}</a>`.
- **Mirror**: `templates/settings.gohtml:21-24`.
- **Validate**: handled by Task 8 test.

### Task 8: Settings test asserts link

- **File**: `internal/handler/settings_test.go`
- **Action**: UPDATE
- **Implement**: Add assertion that body contains `href="/settings/disliked"`.
- **Mirror**: `internal/handler/settings_test.go:21-23`.
- **Validate**: `go test ./internal/handler/...`

### Task 9: i18n strings

- **File**: `i18n/en.json`, `i18n/ru.json`, `i18n/fi.json`
- **Action**: UPDATE
- **Implement**: Add to all three (translated):
  - `settings.disliked` — EN "Disliked ingredients"
  - `disliked.heading` — EN "Disliked ingredients"
  - `disliked.add_label` — EN "Add an ingredient to avoid"
  - `disliked.add_placeholder` — EN "e.g. mushrooms"
  - `disliked.add_button` — EN "Add"
  - `disliked.remove` — EN "Remove"
  - `disliked.empty` — EN "No disliked ingredients yet."
  - `disliked.error_empty` — EN "Enter an ingredient name."
  - RU: "Нелюбимые ингредиенты" / "Добавить ингредиент, которого избегать" / "напр. грибы" / "Добавить" / "Удалить" / "Пока нет нелюбимых ингредиентов." / "Введите название ингредиента."
  - FI: "Ei-toivotut ainekset" / "Lisää vältettävä aines" / "esim. sienet" / "Lisää" / "Poista" / "Ei vielä ei-toivottuja aineksia." / "Syötä aineksen nimi."
  - Keep keys in **identical order across all three files** (i18n bundle_test may assert key parity — verify).
- **Mirror**: `i18n/en.json:29-47` (settings/profile key block).
- **Validate**: `go test ./internal/i18n/...`

---

## Risks

| Risk | Mitigation |
|------|------------|
| i18n key parity test fails if a key is missing from one locale | Add all keys to ru/fi/en in the same task; run `go test ./internal/i18n/...`. |
| `hx-swap="outerHTML"` on the list needs the fragment's root to keep `id="disliked-list"` or the second swap loses its target | Put `id="disliked-list"` on the `<ul>` in the `disliked/list` define; confirm in handler test that the fragment contains the id. |
| Term with spaces/unicode as a path param would break routing | Carry the term in a form field (`ingredient`), not the URL path. |
| `RecentRecipes` failure shouldn't 500 the screen | Graceful degradation: on error render with empty suggestions (CLAUDE.md › Fault Tolerance). |
| Duplicate add inflating the list | Dedup case-insensitively in `AddDisliked`; idempotent success. |
| New template not picked up | `templates/embed.go` globs `*.gohtml`; new file auto-embeds. No code change needed. |

---

## Environment & Verification

| Verification | Runs in env? | If blocked: where/when verified |
|--------------|--------------|---------------------------------|
| `gofmt -s -l .` | yes | — |
| `go vet ./...` | yes | — |
| `golangci-lint run ./...` | yes | — |
| `go test ./...` (service + handler + i18n, httptest E2E) | yes | — |
| Static `CGO_ENABLED=0 go build` | yes | — |
| `govulncheck ./...` | no (vuln.go.dev 403) | networked host / CH-21 (no new deps added, so low risk) |
| Service Worker / PWA over HTTPS | no | tailnet HTTPS / Mac mini / CH-21 (this story doesn't touch the SW) |

No new third-party dependency is introduced (stdlib + existing `html/template`/HTMX only), so `govulncheck` is not gating for this change beyond the existing baseline.

---

## Validation

```bash
gofmt -s -l .            # formatting (no output = clean)
go vet ./...             # vet
golangci-lint run ./...  # lint
go test ./...            # tests
CGO_ENABLED=0 go build ./cmd/server   # static build sanity
# govulncheck ./...      # deferred: vuln.go.dev 403 in sandbox; gated at CH-21 (no new deps here)
```

---

## Acceptance Criteria

- [ ] Disliked screen reachable from Settings (`/settings/disliked` link present)
- [ ] Add via text field with autosuggest (`<datalist>` sourced from recipe history)
- [ ] Remove via per-item button (HTMX `hx-post`)
- [ ] Empty list is valid (empty-state renders, no error)
- [ ] List already feeds every generation prompt as a hard constraint (verified — existing wiring) and changes apply to the next generation (`Current` read per generate)
- [ ] All tasks completed
- [ ] `go build` / `go vet` pass
- [ ] `go test ./...` passes
- [ ] Follows existing handler/service/template/i18n patterns
- [ ] Environment-blocked verifications recorded with their CH-21 gate
