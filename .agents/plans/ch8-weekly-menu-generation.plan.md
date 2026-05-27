# Plan: CH-8 Weekly Menu Generation (F-1)

## Summary

Add the first real generation feature: a "Generate week" button on the home
screen issues an HTMX `POST /generate`, which builds a provider-agnostic LLM
prompt from the household profile (disliked, pantry, family size) plus recent
recipe history/feedback, asks the model for **3 distinct recipes** portioned to
cover `7 days × family size`, validates the hard constraints (dislikes 100%
excluded, ≥2 protein categories, portions sufficient), persists the
`WeeklyPlan` + the 3 `Recipe`s in **one transaction**, and renders 3 recipe
cards (title, cook time, short description, protein emoji) swapped into
`#content`. The LLM is wired once in `main.go` by provider (env-selected); call
sites stay provider-neutral via `internal/llm`.

## User Story

As a household member
I want to tap one button and get a 3-recipe week portioned for 7 days
So that I don't have to plan meals by hand.
(US-1 / PRD F-1)

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY |
| Complexity | HIGH |
| Systems Affected | `internal/llm/prompts`, `internal/service`, `internal/repository`, `internal/handler`, `templates`, `i18n`, `cmd/server` |
| GitHub Issue | #8 (CH-8) |

---

## Scope boundaries (explicitly NOT in this story)

- **Shopping-list building (F-3 / its own story).** `CreateWeeklyPlan` accepts a
  shopping list, but CH-8 persists the plan with an **empty** `ShoppingList`.
  Consolidation/categorization is out of scope here.
- **Persisting current-week display across GET `/` reloads.** CH-8 renders the
  cards in the `POST /generate` response only. Showing the saved current plan on
  a fresh home load is CH-9+. (Consequence: the transient per-recipe protein tag
  used for the card emoji does not need a DB column.)
- **Recipe swap / regenerate (F-2 / CH-9).**
- **Feedback capture UI (F-5).** We *read* existing feedback for the prompt; we
  do not add new feedback-writing here.

---

## Patterns to Follow

### Service: narrow repo interface + domain errors + fake-backed tests
```go
// SOURCE: internal/service/household.go:14-46
var ErrInvalidFamilySize = errors.New("service: family size out of range")

type householdRepo interface {
	FirstHousehold(ctx context.Context) (*domain.HouseholdProfile, error)
	// ...
}
type HouseholdService struct{ repo householdRepo }
func NewHouseholdService(repo householdRepo) *HouseholdService { return &HouseholdService{repo: repo} }
```
```go
// SOURCE: internal/service/household_test.go:13-50  — in-memory fake, no DB
type fakeRepo struct { rows map[string]*domain.HouseholdProfile; nextID int }
func (f *fakeRepo) FirstHousehold(_ context.Context) (*domain.HouseholdProfile, error) { ... }
```

### LLM: provider-agnostic typed generation
```go
// SOURCE: internal/llm/client.go:41-98  — Request{Role,System,Prompt,Schema}; Generate[T] decodes + repairs once
out, err := llm.Generate[generatedWeek](ctx, client, llm.Request{
	Role: llm.RoleGenerate, System: systemBlock, Prompt: trigger, Schema: schemaJSON,
	MaxTokens: 4096, RequestID: requestID,
})
```
```go
// SOURCE: internal/llm/prompts/embed.go:14-22  — prompts loaded by versioned filename
txt, err := prompts.Load("generate_week.v1.txt")
```

### Repository: timeout, UUID-on-empty, withTx for atomic multi-table writes
```go
// SOURCE: internal/repository/weeklyplan.go:17-55 — single tx for plan + items
return s.withTx(ctx, func(tx *sql.Tx) error { ... ExecContext ... })
// SOURCE: internal/repository/recipe.go:16-46 — CreateRecipe assigns UUID + timestamps
// SOURCE: internal/repository/store.go:29-53 — withTx commit/rollback/panic-safe
```

### Handler: depend on an interface, render via renderer, validate at boundary
```go
// SOURCE: internal/handler/profile.go:16-49 — handler depends on a service interface (stub-testable)
type householdProfiles interface { Current(...) (...); UpdateProfile(...) (...) }
// SOURCE: internal/handler/render.go:38-62 — renderStatus picks "<page>/content" for HTMX, buffers first
// SOURCE: internal/handler/router.go:27-45 — NewRouter wires svc, registers routes, middleware chain
```

### Templates: page/content split + HTMX attributes; i18n via t()
```gohtml
{{/* SOURCE: templates/home.gohtml:18-30 + base.gohtml:11-16 */}}
<a hx-get="/settings" hx-target="#content" hx-push-url="true">…</a>
{{ t "home.heading" }}
```

### Tests: table-driven, `errors.Is` on sentinels, httptest for handlers
```go
// SOURCE: internal/service/household_test.go:117-145 (table) ; internal/handler/recipe_test.go:44 (HX-Request header)
```

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `internal/llm/prompts/generate_week.v1.txt` | CREATE | Versioned system/trigger prompt + JSON contract for week generation |
| `internal/service/generation.go` | CREATE | `GenerationService`: build prompt, call LLM, validate constraints, dislike post-process (1 retry), persist atomically |
| `internal/service/generation_test.go` | CREATE | Unit tests with fake `llm.Client` + fake repo: constraint validation, dislike retry, persistence call |
| `internal/repository/recipe.go` | UPDATE | Add `RecentRecipes(ctx, householdID, limit)` (history + feedback for the prompt) |
| `internal/repository/weeklyplan.go` | UPDATE | Add `CreateWeekWithRecipes(ctx, plan, recipes)` — recipes + plan in one tx |
| `internal/repository/weeklyplan_test.go` | UPDATE | Test the combined atomic write + rollback |
| `internal/repository/recipe_test.go` | UPDATE | Test `RecentRecipes` ordering/limit |
| `internal/handler/generate.go` | CREATE | `POST /generate` handler + protein-emoji mapping + card view model |
| `internal/handler/generate_test.go` | CREATE | Handler test with stub generation service (success + LLM-error → friendly 503/200 fragment) |
| `internal/handler/render.go` | UPDATE | Add `renderFragment` to execute a named partial template directly |
| `internal/handler/home.go` | UPDATE | Pass a `CanGenerate` flag (LLM configured) to the home view model |
| `internal/handler/router.go` | UPDATE | Accept an `llm.Client` (or generation service), register `POST /generate`; nil client ⇒ generation disabled |
| `templates/home.gohtml` | UPDATE | Add the generate button + empty cards target |
| `templates/generate.gohtml` | CREATE | `generate/cards` fragment: 3 recipe cards + error partial |
| `i18n/en.json`, `i18n/ru.json`, `i18n/fi.json` | UPDATE | Keys: generate button, generating, error, card labels |
| `cmd/server/main.go` | UPDATE | Construct provider client from env (ANTHROPIC_API_KEY → anthropic, else OPENAI_API_KEY → openai; pin base URL by not reading `*_BASE_URL`); pass into `NewRouter` |

---

## Design detail

### Generation DTO (service-internal, not domain)
The model returns JSON; parse into a transient struct, then map to `domain.Recipe`.
Protein tag is transient (used for validation + card emoji), **not persisted**.
```go
type generatedIngredient struct {
	Name string `json:"name"`; Amount float64 `json:"amount"`; Unit string `json:"unit"`; Category string `json:"category"`
}
type generatedRecipe struct {
	Title string `json:"title"`; Description string `json:"description"`
	CookTimeMinutes int `json:"cook_time_minutes"`; Servings int `json:"servings"`
	Protein string `json:"protein"`            // e.g. poultry|red_meat|pork|fish|seafood|vegetarian|other
	Ingredients []generatedIngredient `json:"ingredients"`; Steps []string `json:"steps"`
}
type generatedWeek struct { Recipes []generatedRecipe `json:"recipes"` }
```
Service result returned to the handler:
```go
type GeneratedWeek struct {
	Plan     *domain.WeeklyPlan
	Recipes  []domain.Recipe
	Proteins []string   // parallel to Recipes; drives the card emoji
}
```

### Constraint validation (in service, order matters)
1. Exactly **3** recipes parsed (else `ErrGenerationInvalid`).
2. **Dislikes 100% excluded** — case-insensitive substring match of each disliked
   term against every ingredient name across all 3 recipes. On violation:
   **retry once** with a clarifying hint listing the offending terms; second
   violation ⇒ `ErrDislikeViolation`. (CLAUDE.md: max 1 retry on hard-constraint
   violation.)
3. **Portions** — `sum(servings) ≥ 7 × (adults + kids)` ⇒ else `ErrPortionsShort`.
4. **Variety** — `len(distinct non-empty protein tags) ≥ 2` ⇒ else `ErrProteinVariety`.

> Note: `llm.Generate[T]` already retries **once** on invalid JSON. The dislike
> retry above is a *separate* semantic retry at the service layer (re-call
> `Generate` with an augmented trigger). Keep total LLM calls bounded (≤ 4 worst
> case) and within the 30 s budget via `context.WithTimeout`.

### Persistence (one transaction — CH-8 technical note)
Add `CreateWeekWithRecipes(ctx, plan *domain.WeeklyPlan, recipes []domain.Recipe)`
to the repository: inside one `withTx`, insert the 3 recipes (assign UUIDs +
timestamps), set `plan.RecipeIDs` to those IDs, insert the plan row (empty
shopping list for CH-8). This supersedes calling `CreateRecipe`×3 +
`CreateWeeklyPlan` separately (which would be 4 independent non-atomic writes).
`source = "llm"`, `week_start = Monday of the current week (UTC)`,
`language = household.Language`.

### Prompt structure (`generate_week.v1.txt`)
Single file holding the **system** block (stable/cacheable: role, hard rules,
JSON schema, store categories, protein-tag enum) and a `---`-delimited **trigger**
template (variable: family size, disliked list, pantry list, recent recipe
titles + feedback, target portions). Service splits on the delimiter; the system
half is passed as `Request.System` (cache breakpoint), the rendered trigger as
`Request.Prompt`. Recipes generated in `household.Language`. Echo the JSON schema
into `Request.Schema` for the repair hint.

### Handler / rendering
- `POST /generate`: load `Current` household, call `svc.GenerateWeek`, on success
  `renderFragment(w, r, 200, "generate/cards", cardsData)`; on `ErrDislikeViolation`
  / `ErrProteinVariety` / `ErrPortionsShort` / LLM transient error, render the
  error partial with a localized message (HTTP 200 fragment so HTMX swaps it; log
  the real error via `rd.fail`-style logging, never leak details — CLAUDE.md
  Security/Errors).
- Protein → emoji map lives in the handler (emoji are language-neutral):
  `poultry 🍗, red_meat 🥩, pork 🐷, fish 🐟, seafood 🦐, vegetarian 🥬, other 🍽`.
- `CanGenerate=false` (no LLM key) ⇒ button renders disabled with a hint key.

### Wiring (`main.go` → `NewRouter`)
```
if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" { client = anthropic.New(k, anthropic.WithLogger(logger)) }
else if k := os.Getenv("OPENAI_API_KEY"); k != "" { client = openai.New(k, openai.WithLogger(logger)) }
else { client = nil }  // generation disabled, rest of app works
```
Do **not** read `*_BASE_URL` from env (CLAUDE.md Security: pin base URL).
`NewRouter` builds the generation service only when `client != nil`.

---

## Risks

| Risk | Mitigation |
|------|------------|
| Live LLM call cannot run in the web sandbox (no key; `api.openai.com` blocked) | All logic unit-tested with a **fake `llm.Client`**; real end-to-end generation deferred to a networked host / Mac mini, gated at **CH-21** (see Environment table) |
| Model returns 2 or 4 recipes, or wrong JSON | `llm.Generate` repairs JSON once; service validates exactly 3 and returns `ErrGenerationInvalid`; render friendly error |
| Dislike substring match too crude (false positives/negatives) | Case-insensitive trimmed substring is acceptable for MVP; document limitation; pantry/disliked are short user-curated lists |
| 30 s budget blown by retries | `context.WithTimeout` on the whole `GenerateWeek`; bound semantic retries to 1; per-call timeout already in provider client (30 s default — lower it for generation if needed) |
| Changing `NewRouter` signature breaks existing tests | Update `router.go` callers/tests; keep `client` optional (nil-safe) so non-generation tests need no LLM |
| Protein variety unverifiable if model omits `protein` | Treat empty/unknown as a distinct bucket only for the emoji; for variety count **non-empty distinct** tags, fail closed with `ErrProteinVariety` |

---

## Environment & Verification

| Verification | Runs in env? | If blocked: where/when verified |
|--------------|--------------|---------------------------------|
| `gofmt -s -l .` | yes | — |
| `go vet ./...` | yes | — |
| `golangci-lint run ./...` | yes | — |
| `go test ./...` (service/handler/repo with fakes + httptest) | yes | — |
| Static `CGO_ENABLED=0 go build ./...` | yes | — |
| **Live LLM week generation** (real provider call) | **no** (no key; OpenAI host blocked) | networked dev host / Mac mini; deploy-gated by **CH-21** |
| `govulncheck ./...` | no | vuln.go.dev 403 in sandbox; run on networked host (no new deps expected) |
| PWA / Service-Worker behavior of new fragment | no | tailnet HTTPS on Mac mini (CH-21) |

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Write the generation prompt
- **File**: `internal/llm/prompts/generate_week.v1.txt`
- **Action**: CREATE
- **Implement**: System block (role, hard rules incl. 100% dislike exclusion and ≥2 protein categories, the JSON schema, store-category enum, protein-tag enum) + `---` delimiter + trigger template with placeholders for family size, disliked, pantry, recent recipes+feedback, target portions. Recipes must be in the household language.
- **Mirror**: `internal/llm/prompts/embed.go:14-22` (versioned filename, loaded via `prompts.Load`)
- **Validate**: `go test ./internal/llm/...`

### Task 2: Repository — RecentRecipes
- **File**: `internal/repository/recipe.go`
- **Action**: UPDATE
- **Implement**: `func (s *Store) RecentRecipes(ctx context.Context, householdID string, limit int) ([]domain.Recipe, error)` — `SELECT ... WHERE household_id = ? ORDER BY created_at DESC LIMIT ?`, reuse the existing scan logic (extract a shared `scanRecipe(rowScanner)` helper from `GetRecipe`).
- **Mirror**: `internal/repository/recipe.go:48-96` (scan), `internal/repository/household.go:104-112` (FirstHousehold ordering pattern)
- **Validate**: `go test ./internal/repository/...`

### Task 3: Repository — atomic week+recipes write
- **File**: `internal/repository/weeklyplan.go`
- **Action**: UPDATE
- **Implement**: `func (s *Store) CreateWeekWithRecipes(ctx context.Context, p *domain.WeeklyPlan, recipes []domain.Recipe) error` — one `withTx`: insert each recipe (UUID + timestamps + `source`), collect IDs into `p.RecipeIDs`, insert the plan + its (possibly empty) shopping list. Factor the recipe INSERT so `CreateRecipe` and this share it.
- **Mirror**: `internal/repository/weeklyplan.go:17-55`, `internal/repository/recipe.go:16-46`, `internal/repository/store.go:29-53`
- **Validate**: `go test ./internal/repository/...`

### Task 4: Repository tests
- **File**: `internal/repository/recipe_test.go`, `internal/repository/weeklyplan_test.go`
- **Action**: UPDATE
- **Implement**: `RecentRecipes` ordering + limit; `CreateWeekWithRecipes` persists 3 recipes + plan with matching `RecipeIDs`, and rolls back fully on a forced failure.
- **Mirror**: existing tests in those files; `internal/service/household_test.go:117-145` for table style
- **Validate**: `go test ./internal/repository/...`

### Task 5: Generation service
- **File**: `internal/service/generation.go`
- **Action**: CREATE
- **Implement**: `GenerationService` depending on narrow interfaces (`llm.Client`, a `generationRepo` with `RecentRecipes` + `CreateWeekWithRecipes`). `GenerateWeek(ctx, h *domain.HouseholdProfile) (*GeneratedWeek, error)`: load recent recipes, render prompt from `prompts.Load`, `llm.Generate[generatedWeek]`, validate (3 recipes → dislikes → portions → protein variety) with one semantic dislike-retry, map DTO→`domain.Recipe`, persist via `CreateWeekWithRecipes`. Define sentinel errors `ErrGenerationInvalid`, `ErrDislikeViolation`, `ErrPortionsShort`, `ErrProteinVariety`. Wrap errors `fmt.Errorf("generate week: %w", err)`.
- **Mirror**: `internal/service/household.go:14-87` (errors, constructor, narrow repo iface)
- **Validate**: `go test ./internal/service/...`

### Task 6: Generation service tests
- **File**: `internal/service/generation_test.go`
- **Action**: CREATE
- **Implement**: fake `llm.Client` returning canned JSON; fake repo capturing the persisted week. Cases: happy path (3 recipes, ≥2 proteins, portions ok → persisted); dislike violation then clean retry → success; persistent dislike → `ErrDislikeViolation`; <3 recipes → `ErrGenerationInvalid`; short portions → `ErrPortionsShort`; single protein → `ErrProteinVariety`.
- **Mirror**: `internal/service/household_test.go:13-50` (fakes), `:117-145` (table + `errors.Is`)
- **Validate**: `go test ./internal/service/...`

### Task 7: renderFragment helper
- **File**: `internal/handler/render.go`
- **Action**: UPDATE
- **Implement**: `func (rd *renderer) renderFragment(w, r, status, name string, data any)` — clone, rebind `t`, execute the exact `name` template into a buffer, write. (For partials returned to HTMX that are not full page/content pairs.)
- **Mirror**: `internal/handler/render.go:38-62`
- **Validate**: `go build ./...`

### Task 8: Generate handler + card view model
- **File**: `internal/handler/generate.go`
- **Action**: CREATE
- **Implement**: `generationService` interface (`GenerateWeek`), `generateHandlers` struct, `Generate(w, r)` for `POST /generate`: get `Current` household, call service, build `[]recipeCard{Title,Description,CookTime,Emoji,ID}` (emoji from protein map), `renderFragment(... "generate/cards" ...)`; on known service errors render the error partial (localized, HTTP 200) and log internally; never leak error details.
- **Mirror**: `internal/handler/profile.go:16-49,54-89` (iface + boundary handling), `internal/handler/recipe.go`
- **Validate**: `go test ./internal/handler/...`

### Task 9: Wire router + home flag
- **File**: `internal/handler/router.go`, `internal/handler/home.go`
- **Action**: UPDATE
- **Implement**: `NewRouter(... , client llm.Client, ...)`; if `client != nil` build `service.NewGenerationService(client, repository.New(db))` and register `mux.HandleFunc("POST /generate", gh.Generate)`. Add `CanGenerate bool` to `homeData` (= client configured). Keep all existing routes.
- **Mirror**: `internal/handler/router.go:27-45`, `internal/handler/home.go:16-30`
- **Validate**: `go build ./... && go test ./internal/handler/...`

### Task 10: Handler test
- **File**: `internal/handler/generate_test.go`
- **Action**: CREATE
- **Implement**: stub `generationService`; assert success renders 3 cards (titles + emoji present) for an `HX-Request` POST; assert a service error renders the localized error partial without leaking details.
- **Mirror**: `internal/handler/recipe_test.go` (httptest + `HX-Request` header at :44), `internal/handler/profile_test.go`
- **Validate**: `go test ./internal/handler/...`

### Task 11: Templates
- **File**: `templates/home.gohtml`, `templates/generate.gohtml`
- **Action**: UPDATE / CREATE
- **Implement**: home — add a `hx-post="/generate" hx-target="#week" hx-indicator` button (disabled when `not .CanGenerate`) + an empty `<section id="week">`. `generate.gohtml` — `define "generate/cards"` (range 3 cards: emoji, title, `t "recipe.cook_time" .CookTime`, description, link to `/recipe/{ID}`) and `define "generate/error"`. Respect Nordic Kitchen sizing (≥18pt body, 44px targets) via existing `app.css` classes.
- **Mirror**: `templates/home.gohtml:18-30`, `templates/base.gohtml:11-16`, `templates/recipe.gohtml`
- **Validate**: `go test ./internal/handler/...` (templates parse + render in handler tests)

### Task 12: i18n keys (RU/FI/EN)
- **File**: `i18n/en.json`, `i18n/ru.json`, `i18n/fi.json`
- **Action**: UPDATE
- **Implement**: add `home.generate`, `home.generating`, `home.generate_disabled`, `generate.error`, `generate.error_dislikes`, `generate.error_portions`, `generate.error_variety`, `recipe.cook_time` (`"%d min"` / localized), `recipe.servings`. Replace the placeholder `home.subtitle` ("Coming soon"). Keep all three dictionaries in sync.
- **Mirror**: `i18n/en.json:1-29`
- **Validate**: `go test ./internal/i18n/...`

### Task 13: main.go provider wiring
- **File**: `cmd/server/main.go`
- **Action**: UPDATE
- **Implement**: select provider from env (ANTHROPIC_API_KEY → `anthropic.New`, else OPENAI_API_KEY → `openai.New`, else nil), log which provider (or "generation disabled"), pass `client` into `handler.NewRouter`. Do not read `*_BASE_URL`.
- **Mirror**: `cmd/server/main.go:41-83`, `internal/llm/anthropic/client.go:66-77`
- **Validate**: `CGO_ENABLED=0 go build ./... && go test ./...`

### Task 14: Full validation pass
- **File**: —
- **Action**: —
- **Implement**: run the full sandbox-runnable suite; record deferred checks (live LLM, govulncheck, SW/HTTPS) against CH-21.
- **Validate**: see Validation section.

---

## Validation

```bash
gofmt -s -l .            # formatting — no output = clean
go vet ./...             # vet
golangci-lint run ./...  # lint
go test ./...            # tests
CGO_ENABLED=0 go build ./cmd/server   # static build sanity
# Deferred (cannot run in sandbox — gated at CH-21 / networked host):
#   govulncheck ./...         # vuln.go.dev 403
#   live week generation      # real provider call (no key / OpenAI host blocked)
#   Service-Worker over HTTPS # tailnet HTTPS on Mac mini
```

---

## Acceptance Criteria

- [ ] "Generate week" button on home triggers `POST /generate` (HTMX) and swaps in cards
- [ ] Prompt includes household profile, disliked, pantry, recent feedback, and week history
- [ ] Response parsed into 3 `domain.Recipe` (PRD §15 schema)
- [ ] Portions sum ≥ `7 × (adults + kids)` (else friendly error)
- [ ] ≥ 2 distinct protein categories across the 3 recipes (else friendly error)
- [ ] Disliked ingredients 100% excluded (validated; 1 semantic retry)
- [ ] `WeeklyPlan` + 3 `Recipe`s saved in one transaction
- [ ] 3 cards render (title, cook time, short description, protein emoji)
- [ ] `gofmt`/`vet`/`lint`/`test`/static build pass
- [ ] Follows existing service/handler/repo/template patterns
- [ ] Deferred verifications (live generation, govulncheck, SW/HTTPS) recorded against CH-21
```
