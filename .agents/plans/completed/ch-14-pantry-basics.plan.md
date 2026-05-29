# Plan: CH-14 Pantry Basics Management (F-6)

## Summary

Add a settings-reachable screen to edit the household's "always at home" pantry-basics
list (salt, pepper, oils, flour, sugar, …) so those staples are excluded from the
shopping list. The **persistence and shopping-list consumption already exist**:
`HouseholdProfile.PantryBasics` is stored by the repository
(`CreateHousehold`/`UpdateHousehold`) and consumed at build time by
`ShoppingBuilder.Build` → `shopping.Consolidate` (drops matching ingredients). This work
adds (1) a localized default list seeded on household creation, (2) add/remove service
operations on the list, (3) a `pantryHandlers` HTTP path with HTMX add/remove fragments,
and (4) a `pantry.gohtml` screen linked from settings. **No migration** — the
`pantry_basics` column already exists with default `'[]'`.

## User Story

As a household member
I want to edit the list of staples I always have at home
So that salt, pepper, and oil never clutter my weekly shopping list

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY |
| Complexity | LOW |
| Systems Affected | domain, service, handler, templates, i18n, router |
| GitHub Issue | #14 |

---

## Patterns to Follow

### Naming — narrow handler-facing interface + handler struct
```go
// SOURCE: internal/handler/profile.go:14-34
type householdProfiles interface {
	Current(ctx context.Context, defaultLang domain.Language) (*domain.HouseholdProfile, error)
	UpdateProfile(ctx context.Context, id string, lang domain.Language, adults, kids int) (*domain.HouseholdProfile, error)
}

type profileHandlers struct {
	rd     *renderer
	bundle *i18n.Bundle
	svc    householdProfiles
}

type profileData struct {
	Lang   string
	Adults int
	Kids   int
	Error  string // i18n key of a validation error, empty when none
}
```

### Service — load, mutate, persist; defaults at creation; domain errors
```go
// SOURCE: internal/service/household.go:48-87
// Current creates with defaults on first access; UpdateProfile loads the
// existing profile, mutates only its own fields (preserving the rest), persists.
var ErrInvalidFamilySize = errors.New("service: family size out of range")

h, err := s.repo.GetHousehold(ctx, id)
if err != nil {
	return nil, fmt.Errorf("update profile: %w", err)
}
h.Language = lang
if err := s.repo.UpdateHousehold(ctx, h); err != nil {
	return nil, fmt.Errorf("update profile: %w", err)
}
return h, nil
```

### HTMX fragment re-render after a mutation (no full-page reload)
```go
// SOURCE: internal/handler/shopping.go:165-181, internal/handler/render.go:72-91
// PathValue/FormValue at the boundary; ErrNotFound degrades to a benign no-op;
// renderFragment swaps a named partial back into the page.
sh.rd.renderFragment(w, r, http.StatusOK, "shopping/removed", shoppingItemView{ID: id})
```

### Template page/content/fragment trio
```gohtml
{{- define "profile/page" -}} ... {{ template "profile/content" . }} ... {{- end -}}
{{- define "profile/content" -}}
  <form method="post" action="/settings/profile"> ... </form>
{{- end -}}
// SOURCE: templates/profile.gohtml:1-44 — page wraps content; renderStatus picks
// "<page>/content" for HTMX navigations, "<page>/page" otherwise.
```

### Handler test — hand-written stub recording calls
```go
// SOURCE: internal/handler/profile_test.go:14-54
type stubProfiles struct { current *domain.HouseholdProfile; updateCalls int; ... }
func (s *stubProfiles) Current(...) (*domain.HouseholdProfile, error) { return s.current, nil }
func newProfileRouter(t *testing.T, stub *stubProfiles) http.Handler { ... }
```

### Service test — in-memory fake repo
```go
// SOURCE: internal/service/household_test.go:12-50
type fakeRepo struct { rows map[string]*domain.HouseholdProfile; nextID int }
// implements FirstHousehold/CreateHousehold/GetHousehold/UpdateHousehold
```

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `internal/domain/household.go` | UPDATE | Add `DefaultPantryBasics(lang) []string` — localized PRD §15 default list (domain data, not UI chrome). |
| `internal/service/household.go` | UPDATE | Seed defaults in `Current()` at creation; add `AddPantryBasic` / `RemovePantryBasic` + `ErrEmptyIngredient`. |
| `internal/service/household_test.go` | UPDATE | Assert seeded defaults; cover add (dedupe, trim, empty) and remove. |
| `internal/handler/pantry.go` | CREATE | `pantryHandlers` (Show / Add / Remove) + `pantryData` view model. |
| `internal/handler/pantry_test.go` | CREATE | Stub-driven handler tests (show, add, remove, empty-input rejection). |
| `internal/handler/profile.go` | UPDATE | Extend `householdProfiles` interface with the two new methods (shared interface). |
| `internal/handler/profile_test.go` | UPDATE | Extend `stubProfiles` to satisfy the widened interface. |
| `internal/handler/router.go` | UPDATE | Wire `pantryHandlers`; register `GET /settings/pantry`, `POST /settings/pantry/add`, `POST /settings/pantry/remove`. |
| `templates/pantry.gohtml` | CREATE | `pantry/page`, `pantry/content`, `pantry/list` (swappable) fragments. |
| `templates/settings.gohtml` | UPDATE | Add a "Pantry basics" link section. |
| `i18n/en.json`, `i18n/ru.json`, `i18n/fi.json` | UPDATE | `settings.pantry`, `pantry.*` UI keys (heading, description, add label/placeholder/button, remove, empty, error_empty). |

---

## Design Decisions

- **No migration.** `household_profile.pantry_basics TEXT NOT NULL DEFAULT '[]'` already
  exists (`migrations/000001_init.up.sql:7`) and the repository already JSON-encodes/
  decodes `PantryBasics`. Do **not** add a schema change.
- **Default list is domain data, not UI strings.** Pantry items are persisted, free-text,
  user-editable values matched against recipe ingredient names — like recipe ingredients,
  they are not chrome and do not go through `t()`. Put the localized PRD §15 default list
  in `domain.DefaultPantryBasics(lang)`. Only the *UI labels* (heading, buttons) go through
  i18n. Seed defaults **in the household's language at creation** (recipes are generated in
  that language, so matching stays in-language); switching UI language later does not
  re-translate the stored list — same accepted trade-off as recipes.
- **List items have no DB IDs** (they are strings in a slice). Add dedupes
  case-insensitively so stored entries are unique; remove matches case-insensitively by the
  submitted value. Add/remove operate on the whole list and persist via `UpdateHousehold`.
- **Validation at the handler boundary**: trim input, reject empty before calling the
  service; the service also trims/dedupes/validates as defense in depth (returns
  `ErrEmptyIngredient`).
- **HTMX**: add/remove forms post and swap the re-rendered `#pantry-list` fragment
  (`hx-target="#pantry-list" hx-swap="outerHTML"`), clearing the add input on each render.
  Non-HTMX POSTs still work (the fragment is valid standalone markup); a `303` redirect
  fallback is unnecessary because the fragment render is self-contained.
- **Applies to next generation automatically** — no generation/shopping code changes:
  `ShoppingBuilder.Build` reads `household.PantryBasics` at build time
  (`internal/service/shopping.go:58`).

### Default pantry list (PRD §15 Appendix, line 457)
| RU | FI | EN |
|----|----|----|
| соль | suola | salt |
| чёрный перец | mustapippuri | black pepper |
| растительное масло | kasviöljy | vegetable oil |
| сливочное масло | voi | butter |
| мука пшеничная | vehnäjauho | wheat flour |
| сахар | sokeri | sugar |

---

## Risks

| Risk | Mitigation |
|------|------------|
| Widening shared `householdProfiles` interface breaks existing stubs/compile | Update `stubProfiles` in `profile_test.go` in the same change; `shoppingHandlers` also uses the interface but only calls `Current` — adding methods is additive, no behavior change. |
| Existing household rows (pre-change) have empty `pantry_basics` | Acceptable for pre-deploy MVP (single household, fresh data). Defaults seed only on *creation*; an already-created empty household can add items manually. Do NOT retroactively mutate on read (would clobber an intentional empty list). |
| Duplicate / whitespace-only entries pollute the list | Service trims, rejects empty (`ErrEmptyIngredient`), dedupes case-insensitively. |
| Pantry term in one language won't match recipes generated in another after a language switch | Documented trade-off (same as recipes keeping their language); out of scope for CH-14. |
| `t()` on data values would be wrong | Defaults live in `domain`, not i18n; only labels are translated. |

## Environment & Verification

| Verification | Runs in env? | If blocked: where/when verified |
|--------------|--------------|---------------------------------|
| `gofmt -s -l .` | yes | — |
| `go vet ./...` | yes | — |
| `golangci-lint run ./...` | yes | — |
| `go test ./...` (domain, service, handler, template render) | yes | — |
| `govulncheck ./...` | n/a | No new dependencies added; `vuln.go.dev` 403 in sandbox — defer to CH-21 if deps ever change (they don't here). |
| Service Worker / HTTPS | n/a | Feature uses no SW/PWA surface. |

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Localized default pantry list (domain)

- **File**: `internal/domain/household.go`
- **Action**: UPDATE
- **Implement**: Add `func DefaultPantryBasics(lang Language) []string` returning the
  localized default list from the table above; default to EN for an unknown language.
  Return a fresh slice on each call (callers may mutate). Document it as the PRD §15
  default.
- **Mirror**: existing domain constants/types in `internal/domain/household.go:9-31`.
- **Validate**: `go build ./internal/domain/...`

### Task 2: Seed defaults + add/remove service operations

- **File**: `internal/service/household.go`
- **Action**: UPDATE
- **Implement**:
  - In `Current()`, when creating the default profile, set
    `PantryBasics: domain.DefaultPantryBasics(defaultLang)`.
  - Add `var ErrEmptyIngredient = errors.New("service: empty ingredient")`.
  - Add `AddPantryBasic(ctx, id, item string) (*domain.HouseholdProfile, error)`:
    trim `item`; if empty → `ErrEmptyIngredient`; load via `GetHousehold`; skip if a
    case-insensitive match already exists; else append; `UpdateHousehold`; return profile.
  - Add `RemovePantryBasic(ctx, id, item string) (*domain.HouseholdProfile, error)`:
    load; rebuild the slice excluding the case-insensitive match of trimmed `item`;
    `UpdateHousehold`; return profile. Removing an absent item is a no-op (still persists/
    returns current — keep idempotent).
  - Wrap errors `fmt.Errorf("add pantry basic: %w", err)` etc.
- **Mirror**: `internal/service/household.go:48-87` (load→mutate→persist; domain errors).
- **Validate**: `go build ./internal/service/...`

### Task 3: Service tests

- **File**: `internal/service/household_test.go`
- **Action**: UPDATE
- **Implement**:
  - Extend `TestCurrentCreatesDefaults` to assert
    `h.PantryBasics` equals `domain.DefaultPantryBasics(domain.LanguageFI)`.
  - `TestAddPantryBasicAppendsAndDedupes`: add "Olive Oil", then "olive oil" → length
    unchanged on the second; trimmed; persisted to `repo.rows`.
  - `TestAddPantryBasicRejectsEmpty`: `"   "` → `errors.Is(err, ErrEmptyIngredient)`,
    list unchanged.
  - `TestRemovePantryBasicCaseInsensitive`: seed, remove with different case → gone;
    removing an absent item is a no-op (no error).
- **Mirror**: `internal/service/household_test.go:12-50` (fakeRepo), `:52-115`.
- **Validate**: `go test ./internal/service/...`

### Task 4: Extend shared householdProfiles interface

- **File**: `internal/handler/profile.go`
- **Action**: UPDATE
- **Implement**: Add to the `householdProfiles` interface:
  ```go
  AddPantryBasic(ctx context.Context, id, item string) (*domain.HouseholdProfile, error)
  RemovePantryBasic(ctx context.Context, id, item string) (*domain.HouseholdProfile, error)
  ```
- **Mirror**: `internal/handler/profile.go:16-19`.
- **Validate**: `go build ./internal/handler/...` (will fail until Task 8 stub updated — run after Task 8, or expect the stub gap; sequence Task 8 before building).

### Task 5: Pantry handlers

- **File**: `internal/handler/pantry.go`
- **Action**: CREATE
- **Implement**:
  - `type pantryData struct { Lang string; Items []string; Error string }`.
  - `type pantryHandlers struct { rd *renderer; bundle *i18n.Bundle; svc householdProfiles }`.
  - `Show`: `svc.Current(...)`; render `"pantry"` with `Items: h.PantryBasics`.
  - `Add`: read `item := strings.TrimSpace(r.FormValue("item"))`; if empty →
    re-render `pantry/list` fragment with `Error: "pantry.error_empty"` and the current
    items (HTTP 400 via `renderFragment` status arg); else `svc.Current` → `AddPantryBasic`
    → render `pantry/list` fragment with updated items (200).
  - `Remove`: `item := strings.TrimSpace(r.FormValue("item"))`; `svc.Current` →
    `RemovePantryBasic` → render `pantry/list` fragment (200).
  - On `ErrEmptyIngredient` from the service, re-render the fragment with the error key.
  - Use `rd.fail` for unexpected errors.
- **Mirror**: `internal/handler/profile.go:38-49` (Show) + `internal/handler/shopping.go:143-181`
  (boundary trim, fragment render) + `internal/handler/render.go:72-91` (`renderFragment`).
- **Validate**: `go build ./internal/handler/...`

### Task 6: Pantry templates

- **File**: `templates/pantry.gohtml`
- **Action**: CREATE
- **Implement**: Define `pantry/page` (wraps content, like `profile/page`),
  `pantry/content` (`<h1>{{ t "pantry.heading" }}</h1>`, description, then
  `{{ template "pantry/list" . }}`), and `pantry/list` — a `<div id="pantry-list">`
  containing:
  - the add form: `<form hx-post="/settings/pantry/add" hx-target="#pantry-list"
    hx-swap="outerHTML" method="post" action="/settings/pantry/add">` with a text input
    `name="item"` (`placeholder` = `t "pantry.add_placeholder"`, `aria-label`/label =
    `t "pantry.add_label"`) and a submit `{{ t "pantry.add" }}`;
  - `{{- if .Error }}<p role="alert">{{ t .Error }}</p>{{- end }}`;
  - the items: `{{ range .Items }}` a row with the name and a remove form
    `<form hx-post="/settings/pantry/remove" hx-target="#pantry-list" hx-swap="outerHTML"
    method="post" action="/settings/pantry/remove"><input type="hidden" name="item"
    value="{{ . }}"><button>{{ t "pantry.remove" }}</button></form>`;
  - empty state `{{ if not .Items }}{{ t "pantry.empty" }}{{ end }}`.
  - Include `<script src="/static/js/htmx.min.js">` and `{{ template "sw-register" . }}`
    in `pantry/page` (mirror `profile/page`).
- **Mirror**: `templates/profile.gohtml:1-44`, fragment swap pattern in `templates/shopping.gohtml`.
- **Validate**: covered by handler render tests in Task 9 (`go test ./internal/handler/...`).

### Task 7: Settings link

- **File**: `templates/settings.gohtml`
- **Action**: UPDATE
- **Implement**: Add a `<section>` with `<h2>{{ t "settings.pantry" }}</h2>` and an
  HTMX link to `/settings/pantry` mirroring the profile link
  (`hx-get`/`hx-target="#content"`/`hx-push-url="true"`).
- **Mirror**: `templates/settings.gohtml:21-24`.
- **Validate**: `go test ./internal/handler/...`

### Task 8: i18n keys (all three languages)

- **File**: `i18n/en.json`, `i18n/ru.json`, `i18n/fi.json`
- **Action**: UPDATE
- **Implement**: Add `settings.pantry`, `pantry.heading`, `pantry.description`,
  `pantry.add_label`, `pantry.add_placeholder`, `pantry.add`, `pantry.remove`,
  `pantry.empty`, `pantry.error_empty` to each file with the matching translation.
  EN values e.g. `"settings.pantry": "Pantry basics"`, `"pantry.heading": "Pantry basics"`,
  `"pantry.description": "Staples you always have at home — they stay off the shopping list."`,
  `"pantry.add": "Add"`, `"pantry.remove": "Remove"`, `"pantry.empty": "No pantry basics yet."`,
  `"pantry.error_empty": "Enter an ingredient."`. Provide RU/FI equivalents.
  Keep all three key sets identical (the i18n bundle test asserts key parity — verify).
- **Mirror**: `i18n/en.json:29-36`.
- **Validate**: `go test ./internal/i18n/...`

### Task 9: Handler tests + wire router

- **File**: `internal/handler/pantry_test.go` (CREATE), `internal/handler/profile_test.go`
  (UPDATE), `internal/handler/router.go` (UPDATE)
- **Action**: CREATE / UPDATE
- **Implement**:
  - `router.go`: build `pantryHandlers{rd, bundle, svc}` (reuse the existing `svc`) and
    register `GET /settings/pantry` → `Show`, `POST /settings/pantry/add` → `Add`,
    `POST /settings/pantry/remove` → `Remove`.
  - `profile_test.go`: add `AddPantryBasic`/`RemovePantryBasic` methods to `stubProfiles`
    (record last item, mutate the stub's `current.PantryBasics`) so it still satisfies the
    widened interface.
  - `pantry_test.go`: a `newPantryRouter` helper + tests: Show renders current items;
    Add posts `item` → service called, fragment contains the new item; Add empty `item`
    → 400 + `pantry.error_empty`, service add not called; Remove posts `item` → service
    called, item gone from fragment.
- **Mirror**: `internal/handler/profile_test.go:14-139`.
- **Validate**: `go test ./internal/handler/...`

---

## Validation

```bash
gofmt -s -l .            # formatting — no output = clean
go vet ./...             # vet
golangci-lint run ./...  # lint
go test ./...            # all tests
# govulncheck ./...      # SKIP: no new deps; vuln.go.dev 403 in sandbox (CH-21 gate)
```

---

## Acceptance Criteria

- [ ] Pantry-basics screen reachable from settings (`GET /settings/pantry`, linked in `settings/content`)
- [ ] Localized default list seeded on household creation (соль/перец/масла/мука/сахар, per PRD §15)
- [ ] Add ingredient via text field (`POST /settings/pantry/add`), trimmed + deduped
- [ ] Remove ingredient via button (`POST /settings/pantry/remove`)
- [ ] Changes persist to `HouseholdProfile.PantryBasics` and apply to the next shopping-list build (no generation code change needed)
- [ ] All three i18n files carry the new keys with identical key sets
- [ ] `gofmt`/`go vet`/`golangci-lint`/`go test` all pass
- [ ] Follows existing handler/service/template/i18n patterns
- [ ] No schema migration (column already exists)
