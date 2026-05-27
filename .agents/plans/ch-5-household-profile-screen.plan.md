# Plan: CH-5 Household Profile Screen

## Summary

Add a household profile screen reachable from settings where the user sets family
composition (adults 1–6, kids 0–6) and UI language. Values persist to SQLite through
the existing CH-3 repository so the next week generation portions correctly. On first
access the singleton household is auto-created with defaults (2 adults, 0 kids, language
detected from `Accept-Language`). This introduces the project's first **service layer**
(`internal/service/household.go`) sitting between the new handlers and the repository,
plus a `FirstHousehold` repository query to locate the single MVP household.

## User Story

As a user
I want to specify my family composition (adults and kids) and language
So that portions in the weekly meal plan are calculated correctly.

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY |
| Complexity | LOW |
| Systems Affected | `internal/handler`, `internal/service`, `internal/repository`, `templates`, `i18n` |
| GitHub Issue | #5 |

---

## Design Decisions

- **Singleton household (MVP).** PRD/issue specify one profile per household. There is no
  "current household" concept yet. The service resolves it via a new
  `Store.FirstHousehold(ctx)` (oldest row) and creates one with defaults if none exists —
  a get-or-create called `Current`.
- **First service layer.** `internal/service` is currently only `doc.go`. CH-5 adds
  `HouseholdService` to keep get-or-create + range validation out of the handler and
  repository, honoring `handler → service → repository → domain`.
- **Validation in two places.** Handler validates the range at the HTTP boundary (per
  CLAUDE.md "validate all external input at the handler boundary") and re-renders the form
  with an error on violation. The service re-validates as defense in depth and returns a
  domain sentinel error.
- **Handler depends on an interface, not the concrete service**, so handler tests use a
  stub and need no database. The `*service.HouseholdService` satisfies it at wiring time.
- **Language cookie kept in sync.** Saving the profile also writes the existing `lang`
  cookie (reusing `languageCookie`) so the UI language immediately reflects the saved
  value — the DB row and the cookie that drives `languageMiddleware` stay consistent.
- **No new CSS.** The project has no stylesheet yet (home page is unstyled markup). The
  profile page mirrors `layout.gohtml`'s minimal semantic structure with proper `<label>`s
  and `min`/`max` on number inputs. Nordic Kitchen styling is deferred to a later styling
  story — flagged as a known deviation, not introduced here.

---

## Patterns to Follow

### Naming / repository query (mirror GetHousehold)
```go
// SOURCE: internal/repository/household.go:49-87
func (s *Store) GetHousehold(ctx context.Context, id string) (*domain.HouseholdProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	const q = `SELECT id, language, family_adults, family_kids, disliked_ingredients, pantry_basics, created_at, updated_at
		FROM household_profile WHERE id = ?`
	// ... scan, map JSON columns, ErrNotFound on sql.ErrNoRows ...
}
```

### Error handling (wrap with context; domain sentinels)
```go
// SOURCE: internal/repository/errors.go:1-6 and internal/repository/household.go:43-45
var ErrNotFound = errors.New("repository: not found")
// ...
return fmt.Errorf("create household: %w", err)
```

### Handler form parsing + validation + safe redirect
```go
// SOURCE: internal/handler/language.go:46-65
func SetLanguage(bundle *i18n.Bundle) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := domain.Language(r.FormValue("lang"))
		if !bundle.Has(lang) {
			http.Error(w, "unsupported language", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: languageCookie, Value: string(lang), Path: "/", MaxAge: languageCookieMaxAge, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.Redirect(w, r, redirectTarget(r), http.StatusSeeOther)
	}
}
```

### Rendering (buffer-first, per-request translator) and view model
```go
// SOURCE: internal/handler/home.go:16-30 + internal/handler/render.go:31-49
type homeData struct { Lang string; CategoryKeys []string }
func (rd *renderer) Home(w http.ResponseWriter, r *http.Request) {
	data := homeData{Lang: string(LanguageFromContext(r.Context())), CategoryKeys: categoryKeys}
	rd.render(w, r, "layout", data)
}
```

### Tests — repository (temp migrated DB) and handler (router + httptest)
```go
// SOURCE: internal/repository/db_test.go:10-25 (newTestStore) ; household_test.go:12-71
store := newTestStore(t)            // fresh migrated SQLite in t.TempDir()
// SOURCE: internal/handler/language_test.go:35-42 (newTestRouter)
rd := &renderer{tmpl: testTemplates(t), bundle: testBundle(t)}
mux := http.NewServeMux(); mux.HandleFunc("GET /{$}", rd.Home)
return languageMiddleware(rd.bundle, mux)
```

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `internal/repository/household.go` | UPDATE | Add `FirstHousehold(ctx)`; extract shared `scanHousehold` helper used by it and `GetHousehold` |
| `internal/repository/household_test.go` | UPDATE | Test `FirstHousehold` (empty → ErrNotFound; after create → returns oldest) |
| `internal/service/household.go` | CREATE | `HouseholdService` with `Current` (get-or-create defaults) and `UpdateProfile` (range validation); `ErrInvalidFamilySize` sentinel |
| `internal/service/household_test.go` | CREATE | Test get-or-create defaults, idempotent second call, range validation |
| `internal/handler/profile.go` | CREATE | `householdProfiles` interface + `profileHandlers` with `Show` (GET) and `Save` (POST) |
| `internal/handler/profile_test.go` | CREATE | GET renders current values; POST valid updates+redirects+sets cookie; POST out-of-range → 400, no persist |
| `internal/handler/router.go` | UPDATE | Build `repository.New(db)` + `service.NewHouseholdService(...)`; register `GET`/`POST /settings/profile` |
| `templates/profile.gohtml` | CREATE | `define "profile"` full page: adults/kids number inputs, language radios, save button, optional error banner |
| `templates/layout.gohtml` | UPDATE | Add a link to `/settings/profile` in the settings section |
| `i18n/en.json` | UPDATE | Profile strings (heading, adults, kids, language, save, range error, nav link) |
| `i18n/fi.json` | UPDATE | Same keys, Finnish |
| `i18n/ru.json` | UPDATE | Same keys, Russian |

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Add `FirstHousehold` repository query

- **File**: `internal/repository/household.go`
- **Action**: UPDATE
- **Implement**: Extract the column scan from `GetHousehold` into a private
  `scanHousehold(scanner) (*domain.HouseholdProfile, error)` helper (handles JSON/time
  decoding). Add `func (s *Store) FirstHousehold(ctx context.Context) (*domain.HouseholdProfile, error)`
  that runs `SELECT <cols> FROM household_profile ORDER BY created_at ASC LIMIT 1`, applies
  `queryTimeout`, returns `ErrNotFound` on `sql.ErrNoRows`, and wraps other errors with
  `fmt.Errorf("first household: %w", err)`. Refactor `GetHousehold` to use the helper.
- **Mirror**: `internal/repository/household.go:49-87`
- **Validate**: `go build ./... && go test ./internal/repository/...`

### Task 2: Test `FirstHousehold`

- **File**: `internal/repository/household_test.go`
- **Action**: UPDATE
- **Implement**: Add `TestFirstHousehold`: on a fresh store, `FirstHousehold` returns
  `ErrNotFound`; after `CreateHousehold`, it returns the created row (assert ID + FamilySize).
- **Mirror**: `internal/repository/household_test.go:12-86`
- **Validate**: `go test ./internal/repository/...`

### Task 3: Create `HouseholdService`

- **File**: `internal/service/household.go`
- **Action**: CREATE
- **Implement**:
  - Define a minimal repo interface the service needs (`FirstHousehold`, `CreateHousehold`,
    `GetHousehold`, `UpdateHousehold`) so the service is unit-testable; `*repository.Store`
    satisfies it.
  - `var ErrInvalidFamilySize = errors.New("service: family size out of range")`.
  - Constants `minAdults=1, maxAdults=6, minKids=0, maxKids=6`; default `2` adults / `0` kids.
  - `Current(ctx, defaultLang domain.Language) (*domain.HouseholdProfile, error)`:
    `FirstHousehold`; if `errors.Is(err, repository.ErrNotFound)` create one with defaults
    + `defaultLang` and return it; wrap other errors.
  - `UpdateProfile(ctx, id string, lang domain.Language, adults, kids int) (*domain.HouseholdProfile, error)`:
    validate ranges → `ErrInvalidFamilySize`; load via `GetHousehold`; set Language/FamilySize
    (preserve disliked/pantry); `UpdateHousehold`; return the updated profile.
- **Mirror**: error/wrapping style from `internal/repository/household.go:43-45`; package doc in `internal/service/doc.go`
- **Validate**: `go build ./...`

### Task 4: Test `HouseholdService`

- **File**: `internal/service/household_test.go`
- **Action**: CREATE
- **Implement**: Use a small in-memory fake of the repo interface (map keyed by ID) — no DB.
  Cases: `Current` on empty creates defaults (2/0) with passed language; second `Current`
  returns the same row (no duplicate); `UpdateProfile` with valid values persists; with
  adults 0, adults 7, kids 7 returns `ErrInvalidFamilySize` and does not mutate.
- **Mirror**: assertion style in `internal/repository/household_test.go:12-71`
- **Validate**: `go test ./internal/service/...`

### Task 5: Add profile i18n strings (all three languages)

- **File**: `i18n/en.json`, `i18n/fi.json`, `i18n/ru.json`
- **Action**: UPDATE
- **Implement**: Add keys to each dictionary:
  `settings.profile` (nav link / heading), `profile.heading`, `profile.adults`,
  `profile.kids`, `profile.language`, `profile.save`, `profile.error_range`.
  EN values e.g. "Household profile", "Adults", "Kids", "Language", "Save",
  "Adults must be 1–6 and kids 0–6." FI and RU equivalents.
- **Mirror**: `i18n/en.json:1-15`
- **Validate**: `go test ./internal/i18n/...`

### Task 6: Create profile template

- **File**: `templates/profile.gohtml`
- **Action**: CREATE
- **Implement**: `{{ define "profile" }}` full HTML doc (mirror `layout.gohtml`): `<html lang="{{ .Lang }}">`,
  title/h1 `{{ t "profile.heading" }}`. A `<form method="post" action="/settings/profile">` with:
  `<label>{{ t "profile.adults" }}<input type="number" name="adults" min="1" max="6" value="{{ .Adults }}"></label>`,
  same for `kids` (min 0 max 6), language as three radios (ru/fi/en, `checked` when matching `.Lang`),
  and a submit button `{{ t "profile.save" }}`. Conditionally render an error banner
  `{{ if .Error }}<p role="alert">{{ t .Error }}</p>{{ end }}` (Error holds an i18n key).
  Include a back link to `/`.
- **Mirror**: `templates/layout.gohtml:1-31`
- **Validate**: `go build ./... && go test ./internal/handler/...`

### Task 7: Create profile handlers

- **File**: `internal/handler/profile.go`
- **Action**: CREATE
- **Implement**:
  - `type householdProfiles interface { Current(ctx, domain.Language) (*domain.HouseholdProfile, error); UpdateProfile(ctx, string, domain.Language, int, int) (*domain.HouseholdProfile, error) }`.
  - `type profileHandlers struct { rd *renderer; svc householdProfiles }`.
  - `profileData struct { Lang string; Adults, Kids int; Error string }`.
  - `Show(w, r)`: `svc.Current(ctx, LanguageFromContext(ctx))`; on error `rd.fail`; render
    `"profile"` with current values, no error.
  - `Save(w, r)`: parse `adults`/`kids` via `strconv.Atoi` and `lang`; boundary-validate
    ranges (1–6 / 0–6) and `bundle.Has(lang)` — on invalid, re-render `"profile"` with
    `Error: "profile.error_range"` and HTTP 400, echoing submitted values, **not** persisting;
    on valid, `svc.UpdateProfile(...)`, then set the `lang` cookie (reuse `languageCookie`
    et al. from `language.go`) and `http.Redirect` 303 to `/settings/profile`.
  - On `errors.Is(err, service.ErrInvalidFamilySize)` from the service, treat as 400 + re-render
    (defense in depth).
- **Mirror**: `internal/handler/language.go:46-65`, `internal/handler/home.go:24-30`, `internal/handler/render.go:51-54`
- **Validate**: `go build ./...`

### Task 8: Wire routes in the router

- **File**: `internal/handler/router.go`
- **Action**: UPDATE
- **Implement**: In `NewRouter`, build `store := repository.New(db)` and
  `svc := service.NewHouseholdService(store)`; construct `ph := &profileHandlers{rd: rd, svc: svc}`.
  Register `mux.HandleFunc("GET /settings/profile", ph.Show)` and
  `mux.HandleFunc("POST /settings/profile", ph.Save)`. Keep existing routes and middleware
  chain unchanged. (Signature stays `(logger, db, bundle, tmpl)`.)
- **Mirror**: `internal/handler/router.go:23-32`
- **Validate**: `go build ./...`

### Task 9: Link profile from settings

- **File**: `templates/layout.gohtml`
- **Action**: UPDATE
- **Implement**: In the settings `<section>`, add `<a href="/settings/profile">{{ t "settings.profile" }}</a>`.
- **Mirror**: `templates/layout.gohtml:21-28`
- **Validate**: `go test ./internal/handler/...`

### Task 10: Handler tests for profile

- **File**: `internal/handler/profile_test.go`
- **Action**: CREATE
- **Implement**: A stub `householdProfiles` (records calls, returns a canned profile).
  Build a router with the profile routes + `languageMiddleware` (reuse `testTemplates`/`testBundle`).
  Cases: `GET /settings/profile` → 200 and body contains the current adults/kids values;
  `POST` with adults=3,kids=2,lang=fi → 303 to `/settings/profile`, `lang=fi` cookie set,
  stub's `UpdateProfile` received (3,2,fi); `POST` adults=0 (and a 7 case) → 400, body shows
  the error string, stub `UpdateProfile` NOT called.
- **Mirror**: `internal/handler/language_test.go:35-131`
- **Validate**: `go test ./internal/handler/...`

---

## Risks

| Risk | Mitigation |
|------|------------|
| Two validation sites drift (handler vs service) | Share the same constants/range; service is defense-in-depth, handler owns the user-facing message |
| Profile language vs `lang` cookie diverge | `Save` writes the cookie on success so middleware-driven UI matches the stored row |
| Refactoring `GetHousehold` for the shared scan breaks existing tests | Run `go test ./internal/repository/...` after Task 1; `TestHouseholdCRUD` covers it |
| No CSS / Nordic Kitchen styling | Out of scope for CH-5; semantic markup + `min`/`max` inputs only, styling deferred (documented above) |
| Handler→DB coupling in tests | Handler depends on `householdProfiles` interface; tests use a stub, no DB needed |

---

## Validation

```bash
gofmt -s -l .            # formatting (no output = clean)
go vet ./...             # vet
golangci-lint run ./...  # lint
go test ./...            # tests
```

---

## Acceptance Criteria

- [ ] Profile screen reachable from settings (link in `layout.gohtml`)
- [ ] Fields: adults (1–6), kids (0–6), language; out-of-range rejected with a localized error
- [ ] Changes persist to SQLite via the CH-3 repository and survive reload
- [ ] First access auto-creates the singleton with defaults: 2 adults, 0 kids, language from `Accept-Language`
- [ ] All tasks completed
- [ ] `go build ./...` and `go test ./...` pass
- [ ] `gofmt -s -l .`, `go vet ./...`, `golangci-lint run ./...` clean
- [ ] Follows existing handler/service/repository patterns
```