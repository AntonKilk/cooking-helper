# Plan: CH-19 Onboarding Flow

## Summary

Add a first-run onboarding wizard (3 steps) shown the first time the app's home
screen is opened, then never again. Step 1 sets family size + language (reusing the
existing profile-update logic), Step 2 shows the seeded pantry basics with the option
to edit or keep the defaults (reusing the existing pantry add/remove fragment), and
Step 3 explains the "generate → cook → feedback" cycle. A **Skip** control is present
on every step; both Skip and Finish persist an `onboarded` flag on the household so the
wizard never reappears. A new `onboarded` column is added to `household_profile` via a
golang-migrate migration; the home handler gates on it and redirects to `/onboarding`
when the household has not yet onboarded.

## User Story

As a new user
I want a short introduction to the app on first launch
So that I understand how to use the core features (family setup, pantry, the weekly cycle)

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY |
| Complexity | MEDIUM |
| Systems Affected | migrations, domain, repository, service, handler, templates, i18n |
| GitHub Issue | #19 (CH-19) |

---

## Design Decisions

- **First-run detection** lives on the home handler (`GET /{$}`), the natural entry
  point. `homeHandlers` gains a narrow `homeProfiles` reader (`Current` only); when the
  loaded household has `Onboarded == false`, Home issues a `303` redirect to
  `/onboarding`. The reader is nil-tolerant (skips the gate when unset) so isolated
  handler tests that don't wire a profile service keep working.
- **Wizard navigation** mirrors the existing profile screen: plain `method="post"`
  forms + `303` redirects, **not** an HTMX-driven flow. Step is selected by a
  `?step=N` (1–3) query param on `GET /onboarding`. Step 2 embeds the existing
  `pantry/list` partial, whose own HTMX add/remove (targeting `#pantry-list`) works
  unchanged on a full page.
- **Flag persistence**: a single `POST /onboarding/complete` route backs both the
  per-step Skip button and the Step-3 Finish button — both call
  `CompleteOnboarding` and redirect home. Idempotent: completing an already-onboarded
  household is a harmless re-write.
- **Default pantry basics** are already seeded on household creation
  (`HouseholdService.Current` → `domain.DefaultPantryBasics`), so "keep the default" in
  Step 2 requires no extra work — the list is simply shown.
- **No new dependencies**; no new schema beyond one boolean column.

---

## Patterns to Follow

### Domain field + defaults
```go
// SOURCE: internal/domain/household.go:39-47
type HouseholdProfile struct {
	ID                  string
	Language            Language
	FamilySize          FamilySize
	DislikedIngredients []string
	PantryBasics        []string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
```

### Repository: column list, INSERT/UPDATE, scan (must all stay in sync)
```go
// SOURCE: internal/repository/household.go:51, :37-42, :131-136, :62-93
const householdColumns = `id, language, family_adults, family_kids, disliked_ingredients, pantry_basics, created_at, updated_at`
// INSERT (...) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
// UPDATE ... SET language = ?, family_adults = ?, ... updated_at = ? WHERE id = ?
// scanHousehold scans columns in householdColumns order, decoding JSON/timestamps
```
> Adding `onboarded` means editing the const, the INSERT column+placeholder+arg, the
> UPDATE set-list+arg, and `scanHousehold` (scan into a local `int`, set
> `h.Onboarded = v != 0`). SQLite has no bool — store `0/1` via a small `boolToInt`
> helper / inline conversion.

### Service: load-mutate-persist, wrapped errors
```go
// SOURCE: internal/service/household.go:77-93
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

### Handler: narrow service interface, Current(), localized validation error, redirect
```go
// SOURCE: internal/handler/profile.go:16-21, :40-51, :56-91
type householdProfiles interface {
	Current(ctx context.Context, defaultLang domain.Language) (*domain.HouseholdProfile, error)
	UpdateProfile(ctx context.Context, id string, lang domain.Language, adults, kids int) (*domain.HouseholdProfile, error)
	...
}
// Show: svc.Current(...) then rd.render(...); Save: validate at boundary, UpdateProfile, 303 redirect
```

### Template page/content pair + partial reuse
```gohtml
{{/* SOURCE: templates/pantry.gohtml:1-22 */}}
{{- define "pantry/page" -}} ... <main id="content">{{ template "pantry/content" . }}</main> ... {{- end -}}
{{- define "pantry/content" -}} ... {{ template "pantry/list" . }} {{- end -}}
{{/* pantry/list uses .Lang .Items .Error — onboardingData mirrors these names for reuse */}}
```

### Handler test: stub interface, httptest, assert calls + status
```go
// SOURCE: internal/handler/profile_test.go:16-70, :91-122
type stubProfiles struct { current *domain.HouseholdProfile; updateCalls int; ... }
// newProfileRouter wires renderer{testTemplates, testBundle} + handler + ServeMux + languageMiddleware
// assert rec.Code, Location header, stub call counts
```

### Migration: add column up, DROP COLUMN down
```sql
-- SOURCE: migrations/000002_weekly_plan_archive.up.sql:4 and .down.sql
ALTER TABLE weekly_plan ADD COLUMN archived_at TIMESTAMP NULL;        -- up
ALTER TABLE weekly_plan DROP COLUMN archived_at;                     -- down (SQLite >= 3.35)
```

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `migrations/000004_household_onboarded.up.sql` | CREATE | Add `onboarded INTEGER NOT NULL DEFAULT 0` to `household_profile` |
| `migrations/000004_household_onboarded.down.sql` | CREATE | Reverse: `ALTER TABLE household_profile DROP COLUMN onboarded` |
| `internal/domain/household.go` | UPDATE | Add `Onboarded bool` field to `HouseholdProfile` |
| `internal/repository/household.go` | UPDATE | Add `onboarded` to columns const, INSERT, UPDATE, and `scanHousehold` |
| `internal/repository/household_test.go` | UPDATE | Assert `Onboarded` round-trips through create/get/update |
| `internal/service/household.go` | UPDATE | Add `CompleteOnboarding(ctx, id)` setting `Onboarded=true` |
| `internal/service/household_test.go` | UPDATE | Test `CompleteOnboarding` persists the flag |
| `internal/handler/onboarding.go` | CREATE | `onboardingHandlers`: Show (step 1–3), SaveProfile, Complete |
| `internal/handler/onboarding_test.go` | CREATE | Cover step rendering, profile save→step 2, complete sets flag + redirects, skip |
| `internal/handler/home.go` | UPDATE | Inject `homeProfiles` reader; redirect to `/onboarding` when not onboarded |
| `internal/handler/profile_test.go` | UPDATE | Add `CompleteOnboarding` to `stubProfiles` so it satisfies the extended interface (if shared) |
| `internal/handler/language_test.go` | UPDATE | `newTestRouter` wires a `homeProfiles` stub with `Onboarded=true` so home tests still render home |
| `internal/handler/router.go` | UPDATE | Construct `onboardingHandlers`, pass profiles reader to `homeHandlers`, register routes |
| `templates/onboarding.gohtml` | CREATE | `onboarding/page` + `onboarding/content` + per-step markup; reuse `pantry/list` in step 2 |
| `i18n/en.json` | UPDATE | `onboarding.*` keys |
| `i18n/fi.json` | UPDATE | `onboarding.*` keys |
| `i18n/ru.json` | UPDATE | `onboarding.*` keys |

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Add the `onboarded` migration

- **Files**: `migrations/000004_household_onboarded.up.sql`, `migrations/000004_household_onboarded.down.sql`
- **Action**: CREATE
- **Implement**:
  - up: `ALTER TABLE household_profile ADD COLUMN onboarded INTEGER NOT NULL DEFAULT 0;` with a comment explaining it gates the first-run wizard (CH-19); existing rows default to "not onboarded" — acceptable, an existing single household simply sees the wizard once.
  - down: comment (mirroring 000002) noting SQLite ≥ 3.35, then
    `ALTER TABLE household_profile DROP COLUMN onboarded;`
- **Mirror**: `migrations/000002_weekly_plan_archive.up.sql` / `.down.sql`
- **Validate**: `go test ./internal/repository/...` (RunMigrations runs in `db_test.go:20`)

### Task 2: Add `Onboarded` to the domain model

- **File**: `internal/domain/household.go`
- **Action**: UPDATE
- **Implement**: add `Onboarded bool` to `HouseholdProfile` with a doc comment ("true once the first-run onboarding wizard has been completed or skipped"). Place it before timestamps.
- **Mirror**: `internal/domain/household.go:39-47`
- **Validate**: `go build ./...`

### Task 3: Persist `onboarded` in the repository

- **File**: `internal/repository/household.go`
- **Action**: UPDATE
- **Implement**:
  - Append `onboarded` to `householdColumns`.
  - INSERT: add `onboarded` column, one more `?`, and `boolToInt(h.Onboarded)` arg.
  - UPDATE: add `onboarded = ?` to the SET list with the same arg (before `updated_at` is fine; keep arg order matching).
  - `scanHousehold`: add a local `var onboarded int`, append `&onboarded` to `Scan` in column order, then `h.Onboarded = onboarded != 0`.
  - Add a tiny unexported `boolToInt(b bool) int` helper (or inline a ternary-style conversion) in this file.
- **Mirror**: `internal/repository/household.go:37-47, 62-93, 116-141`
- **Validate**: `go build ./... && go test ./internal/repository/...`

### Task 4: Round-trip test for the flag

- **File**: `internal/repository/household_test.go`
- **Action**: UPDATE
- **Implement**: in an existing or new test, create a household, flip `Onboarded=true` via `UpdateHousehold`, re-`GetHousehold`, assert `Onboarded` is true; assert a freshly created household defaults to false.
- **Mirror**: existing household repo tests in the same file
- **Validate**: `go test ./internal/repository/...`

### Task 5: `CompleteOnboarding` service method

- **File**: `internal/service/household.go`
- **Action**: UPDATE
- **Implement**: add
  ```go
  func (s *HouseholdService) CompleteOnboarding(ctx context.Context, id string) (*domain.HouseholdProfile, error)
  ```
  that `GetHousehold`, sets `h.Onboarded = true`, `UpdateHousehold`, returns `h`, wrapping errors `"complete onboarding: %w"`. Idempotent (setting true on an already-true row is fine).
- **Mirror**: `internal/service/household.go:77-93`
- **Validate**: `go build ./... && go test ./internal/service/...`

### Task 6: Service test

- **File**: `internal/service/household_test.go`
- **Action**: UPDATE
- **Implement**: with the existing fake repo, assert `CompleteOnboarding` sets the flag and returns the updated profile; assert the underlying `UpdateHousehold` was called.
- **Mirror**: existing service tests in the same file
- **Validate**: `go test ./internal/service/...`

### Task 7: i18n keys

- **Files**: `i18n/en.json`, `i18n/fi.json`, `i18n/ru.json`
- **Action**: UPDATE
- **Implement**: add a consistent `onboarding.*` key set to all three (same keys, translated values). Suggested keys:
  `onboarding.welcome_heading`, `onboarding.welcome_body`,
  `onboarding.step_profile_heading`, `onboarding.step_pantry_heading`,
  `onboarding.step_pantry_intro`, `onboarding.step_cycle_heading`,
  `onboarding.cycle_generate`, `onboarding.cycle_cook`, `onboarding.cycle_feedback`,
  `onboarding.cycle_body`, `onboarding.next`, `onboarding.back`,
  `onboarding.skip`, `onboarding.finish`, `onboarding.step_indicator` (e.g. `"Step %d of 3"`).
  Reuse existing `profile.adults/kids/language`, `lang.*`, and `pantry.*` keys for the
  embedded fields rather than duplicating them.
- **Mirror**: existing flat key style in `i18n/en.json:1-76`
- **Validate**: `go test ./internal/i18n/... ./internal/handler/...` (bundle load + template render exercise keys)

### Task 8: Onboarding templates

- **File**: `templates/onboarding.gohtml`
- **Action**: CREATE
- **Implement**:
  - `onboarding/page` — doctype shell identical to `pantry/page`, `<main id="content">{{ template "onboarding/content" . }}</main>`.
  - `onboarding/content` — render a step indicator (`onboarding.step_indicator` with `.Step`), then branch on `{{ if eq .Step 1 }}…{{ else if eq .Step 2 }}…{{ else }}…{{ end }}`:
    - **Step 1**: a `method="post" action="/onboarding/profile"` form with the adults/kids number inputs + language radios (copy markup from `profile/content`, `templates/profile.gohtml:25-43`); submit button `onboarding.next`. Show `.Error` (`profile.error_range`) when set.
    - **Step 2**: intro text (`onboarding.step_pantry_intro`) + `{{ template "pantry/list" . }}` (reuses add/remove against `/settings/pantry/*`), then a plain link/anchor to `/onboarding?step=3` labeled `onboarding.next`.
    - **Step 3**: the generate→cook→feedback explanation (three lines), then a `method="post" action="/onboarding/complete"` form with a `onboarding.finish` submit button.
  - On every step, a `method="post" action="/onboarding/complete"` Skip control (`onboarding.skip`).
  - `onboardingData` must expose `.Lang .Step .Adults .Kids .Items .Error` so both the profile fields and the reused `pantry/list` partial resolve.
- **Mirror**: `templates/pantry.gohtml`, `templates/profile.gohtml`
- **Validate**: `go test ./internal/handler/...` (templates parsed via `testTemplates`)

### Task 9: Onboarding handler

- **File**: `internal/handler/onboarding.go`
- **Action**: CREATE
- **Implement**:
  - `onboardingProfiles` interface: `Current`, `UpdateProfile`, `CompleteOnboarding`.
  - `onboardingHandlers{ rd *renderer; bundle *i18n.Bundle; svc onboardingProfiles }`.
  - `onboardingData` struct (fields per Task 8).
  - `Show`: parse `?step=` (default 1, clamp/validate to 1–3, invalid → step 1), `svc.Current`, populate `Adults/Kids/Items/Lang`, `rd.render(w, r, "onboarding", data)`.
  - `SaveProfile` (POST `/onboarding/profile`): validate adults/kids/lang at the boundary exactly as `profileHandlers.Save` (`internal/handler/profile.go:56-65`); on invalid re-render step 1 with `onboarding`/`profile.error_range` at 400; on valid call `UpdateProfile`, set the language cookie (mirror `profile.go:82-89`), `303` redirect to `/onboarding?step=2`.
  - `Complete` (POST `/onboarding/complete`): `svc.Current` → `svc.CompleteOnboarding(ctx, h.ID)` → `303` redirect to `/`. Used by both Skip and Finish.
- **Mirror**: `internal/handler/profile.go`, `internal/handler/pantry.go`
- **Validate**: `go build ./... && go vet ./...`

### Task 10: Gate the home screen on onboarding

- **File**: `internal/handler/home.go`
- **Action**: UPDATE
- **Implement**:
  - Add a narrow `homeProfiles` interface (`Current(ctx, domain.Language) (*domain.HouseholdProfile, error)`).
  - Add `profiles homeProfiles` to `homeHandlers`.
  - In `Home`, if `hh.profiles != nil`: load `Current`; on error `rd.fail`; if `!h.Onboarded`, `http.Redirect(w, r, "/onboarding", http.StatusSeeOther)` and return. Otherwise render home as today.
  - Keep nil-tolerance so handlers wired without a profile service are unaffected.
- **Mirror**: `internal/handler/profile.go:40-51` for the Current+fail idiom
- **Validate**: `go build ./...`

### Task 11: Wire routes and dependencies

- **File**: `internal/handler/router.go`
- **Action**: UPDATE
- **Implement**:
  - Pass the household service into `homeHandlers`: `hh := &homeHandlers{rd: rd, canGenerate: canGenerate, profiles: svc}`.
  - Construct `oh := &onboardingHandlers{rd: rd, bundle: bundle, svc: svc}`.
  - Register: `GET /onboarding` → `oh.Show`; `POST /onboarding/profile` → `oh.SaveProfile`; `POST /onboarding/complete` → `oh.Complete`.
- **Mirror**: `internal/handler/router.go:55-82`
- **Validate**: `go build ./...`

### Task 12: Update affected stubs/tests

- **Files**: `internal/handler/language_test.go`, `internal/handler/profile_test.go`
- **Action**: UPDATE
- **Implement**:
  - `newTestRouter` (`language_test.go:84-96`): give `homeHandlers` a `profiles` stub whose `current.Onboarded = true` so existing home render tests stay on the home page (no redirect).
  - If `stubProfiles` is reused to satisfy `onboardingProfiles`, add a `CompleteOnboarding` method to it; otherwise leave `profile_test.go` untouched and give the onboarding test its own stub.
- **Mirror**: `internal/handler/profile_test.go:16-70`
- **Validate**: `go test ./internal/handler/...`

### Task 13: Onboarding handler tests

- **File**: `internal/handler/onboarding_test.go`
- **Action**: CREATE
- **Implement**:
  - `GET /onboarding` renders step 1 (contains the adults input + a Skip control).
  - `GET /onboarding?step=2` renders the pantry list; `?step=3` renders the cycle explanation + Finish.
  - `POST /onboarding/profile` with valid values → 303 to `/onboarding?step=2`, `UpdateProfile` called once, language cookie set; out-of-range → 400, no update, localized error present.
  - `POST /onboarding/complete` → `CompleteOnboarding` called once, 303 to `/`.
  - Gate: home handler with a stub returning `Onboarded=false` → `GET /` redirects 303 to `/onboarding`; with `Onboarded=true` → renders home.
- **Mirror**: `internal/handler/profile_test.go:53-122`
- **Validate**: `go test ./internal/handler/...`

### Task 14: Full validation pass

- **Action**: run the full suite
- **Validate**: see Validation section below.

---

## Risks

| Risk | Mitigation |
|------|------------|
| Adding a column desyncs the 4 SQL touch-points (const, INSERT, UPDATE, scan) | Task 3 edits all four together; Task 4 round-trip test catches a miss |
| SQLite has no native bool | Store `0/1` INTEGER via `boolToInt`; scan into `int`, compare `!= 0` |
| Injecting `profiles` into `homeHandlers` breaks existing home tests (nil panic / unexpected redirect) | Nil-tolerant gate + Task 12 wires a stub with `Onboarded=true` |
| Existing single household (pre-migration) defaults to `onboarded=0` and sees the wizard once | Acceptable & on-spec (first-run intro); Skip dismisses it permanently |
| Reusing `pantry/list` partial requires matching field names | `onboardingData` exposes `.Lang .Items .Error` exactly as `pantryData` |
| HTMX nav following a 303 from `Home` | First app open is a full-page GET; redirect is standard. Pantry add/remove inside step 2 keep their own `#pantry-list` target |
| Down-migration `DROP COLUMN` needs SQLite ≥ 3.35 | Already relied on by 000002; modernc.org/sqlite v1.50 bundles a recent SQLite |

---

## Environment & Verification

| Verification | Runs in env? | If blocked: where/when verified |
|--------------|--------------|---------------------------------|
| `gofmt -s -l .` | yes | — |
| `go vet ./...` | yes | — |
| `golangci-lint run ./...` | yes (v2 path under pinned toolchain) | — |
| `go test ./...` (incl. migration via repo test DB, handler HTTP tests) | yes | — |
| `CGO_ENABLED=0 go build ./...` | yes | — |
| `govulncheck ./...` | no (`vuln.go.dev` 403) | No new deps added; deferred to CH-21 deploy gate |
| Service-Worker / PWA over HTTPS | n/a (feature adds no SW changes) | — |
| Real-device kitchen/iPad render check (Nordic Kitchen sizing) | no | Mac mini / tailnet HTTPS; gated at CH-21 |

---

## Validation

```bash
gofmt -s -l .            # formatting (no output = clean)
go vet ./...             # vet
golangci-lint run ./...  # lint (v2 module path under GOTOOLCHAIN=go1.26.3)
go test ./...            # tests (migration + handlers + service + repo)
CGO_ENABLED=0 go build ./cmd/server   # static build sanity
# govulncheck ./...      # blocked in sandbox (vuln.go.dev 403); no deps changed → CH-21
```

---

## Acceptance Criteria

- [ ] First app open shows a 3-step onboarding wizard (AC: 3–4 screens)
- [ ] Step 1 sets family size + language and persists them
- [ ] Step 2 shows seeded pantry basics, editable, defaults kept if untouched
- [ ] Step 3 explains the generate → cook → feedback cycle
- [ ] A Skip control is present on every step and dismisses the wizard
- [ ] Onboarding never reappears once completed/skipped (`onboarded` flag in DB)
- [ ] All UI strings go through `t(...)` in ru/fi/en (no hardcoded text)
- [ ] All tasks completed
- [ ] `go vet`, `golangci-lint`, `go test`, and the static build pass; `gofmt` clean
- [ ] Follows existing handler/service/repository/template patterns
- [ ] Environment-blocked checks recorded: `govulncheck` (no deps changed) and the iPad render check → CH-21
```