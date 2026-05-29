# Plan: CH-12 Shopping list builder with consolidation (F-3)

## Summary

Build the consolidated shopping list automatically whenever a `WeeklyPlan` is
created (and rebuild it on swap), so the household never tallies ingredients by
hand. Pure logic lives in `internal/shopping`: consolidate the three recipes'
ingredients by name + unit-family, sum compatible amounts, keep incompatible
units as separate lines, drop `pantry_basics`, and assign a store `category`
from a built-in multilingual dictionary. Unknown ingredients fall back to a
Haiku (`RoleCategorize`) LLM call via the existing `categorize_ingredient.v1.txt`
prompt, with results cached by normalized name in a new global DB table so the
LLM is never asked twice for the same ingredient. Orchestration of the
LLM+cache fallback sits in the service layer; `internal/shopping` stays pure
(no ctx, no SQL, no SDK), matching its current `normalize.go`.

## User Story

As a home cook planning the week
I want one shopping list auto-built from all three recipes, with identical
ingredients summed and store-categorized and my pantry basics removed
So that I can shop without recomputing quantities by hand. (US-4, US-7)

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY |
| Complexity | HIGH (Large) |
| Systems Affected | `internal/shopping`, `internal/service`, `internal/repository`, `migrations`, `cmd/server` |
| GitHub Issue | #12 (CH-12) |

---

## Patterns to Follow

### Naming — pure, dependency-free helpers in `internal/shopping`
```go
// SOURCE: internal/shopping/normalize.go:25-45
// Normalize lowercases s, drops combining diacritics, and replaces any rune that
// is not a letter or digit with a single space, then collapses whitespace runs.
func Normalize(s string) string { ... }
```
`internal/shopping` imports only `golang.org/x/text` + stdlib today. New
consolidation/units/dictionary code MUST stay equally pure — no `context`,
no `database/sql`, no `internal/llm`. Reuse `Normalize` / `ContainsTerm` for
name matching (pantry exclusion, dictionary lookup).

### Domain types already exist — do not add fields
```go
// SOURCE: internal/domain/plan.go:22-30
type ShoppingListItem struct {
	ID, Name        string
	Amount          float64
	Unit            string
	Category        IngredientCategory
	Checked, ManuallyRemoved bool
}
// SOURCE: internal/domain/recipe.go:8-15 — IngredientCategory consts
CategoryProduce, CategoryMeatFish, CategoryDairy, CategoryPantry, CategoryFrozen, CategoryOther
```

### Untrusted LLM output is coerced to a known enum, defaulting to "other"
```go
// SOURCE: internal/service/generation.go:655-663
func normalizeCategory(s string) domain.IngredientCategory {
	switch c := domain.IngredientCategory(strings.ToLower(strings.TrimSpace(s))); c {
	case domain.CategoryProduce, ... , domain.CategoryOther:
		return c
	default:
		return domain.CategoryOther
	}
}
```
Reuse this exact coercion for the LLM categorize reply. (Consider exporting it
or duplicating in the builder — see Task 6.)

### LLM call via the generic decoder, role-selected, schema-hinted
```go
// SOURCE: internal/service/generation.go:293-301
return llm.Generate[generatedSwap](ctx, g.client, llm.Request{
	Role:      llm.RoleGenerate,
	System:    system,
	Prompt:    trigger,
	Schema:    swapSchemaHint,
	MaxTokens: maxSwapTokens,
})
```
Categorization uses `Role: llm.RoleCategorize`, the `categorize_ingredient.v1.txt`
prompt (loaded via `prompts.Load`), and a small `{"category":string}` schema hint.

### Prompt loading + System/trigger split
```go
// SOURCE: internal/service/generation.go:368-382 ; internal/llm/prompts/embed.go:16
base, err := prompts.Load("categorize_ingredient.v1.txt")  // {{ingredient}} placeholder
```
`categorize_ingredient.v1.txt` is a single-ingredient prompt with one
`{{ingredient}}` slot — render it per uncategorized name with `text/template`.

### Repository: timeout + tx + RFC3339Nano timestamps, ErrNotFound sentinel
```go
// SOURCE: internal/repository/weeklyplan.go:17-24, store.go:35-59, store.go:66-68
ctx, cancel := context.WithTimeout(ctx, queryTimeout); defer cancel()
return s.withTx(ctx, func(tx *sql.Tx) error { ... })
... formatTime(time.Now().UTC()) // RFC3339Nano
```

### Tests: table-driven, fake LLM (canned replies), narrow fake repo
```go
// SOURCE: internal/service/generation_test.go:17-29 (fakeLLM), :34-91 (fakeGenRepo)
// SOURCE: internal/shopping/normalize_test.go:5-27 (table-driven, t.Run per case)
```
Repository tests use a real temp SQLite DB with migrations applied — mirror
`internal/repository/weeklyplan_test.go` setup.

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `migrations/000003_ingredient_category.up.sql` | CREATE | Global `ingredient_category` cache table |
| `migrations/000003_ingredient_category.down.sql` | CREATE | Drop the cache table |
| `internal/shopping/units.go` | CREATE | Unit families, canonical conversion, summation, display formatting |
| `internal/shopping/units_test.go` | CREATE | Unit-family + conversion + formatting tables |
| `internal/shopping/categories.go` | CREATE | Built-in multilingual ingredient→category dictionary (pure) |
| `internal/shopping/categories_test.go` | CREATE | Dictionary coverage on a 5-week fixture (≥95%) |
| `internal/shopping/consolidate.go` | CREATE | `Consolidate(recipes, pantryBasics) []domain.ShoppingListItem` (pure) |
| `internal/shopping/consolidate_test.go` | CREATE | Summation, incompatible-unit split, pantry exclusion, dictionary tagging |
| `internal/repository/ingredient_category.go` | CREATE | `CategoriesByNames` / `SaveCategory` cache access |
| `internal/repository/ingredient_category_test.go` | CREATE | Round-trip + miss behaviour |
| `internal/service/shopping.go` | CREATE | `ShoppingBuilder`: consolidate → cache → LLM fallback → cache write |
| `internal/service/shopping_test.go` | CREATE | Dictionary-hit (no LLM), cache-hit, LLM-fallback, LLM-fail→other |
| `internal/service/generation.go` | UPDATE | Inject builder; build list in `GenerateWeek`; rebuild on `SwapRecipe` |
| `internal/repository/weeklyplan.go` | UPDATE | `SwapRecipeInPlan` re-inserts rebuilt items (not just clears) |
| `internal/service/generation_test.go` | UPDATE | Fake builder; updated constructor + swap-repo signature |
| `internal/repository/weeklyplan_test.go` | UPDATE | Updated `SwapRecipeInPlan` signature in tests |
| `cmd/server/main.go` | UPDATE | Construct `ShoppingBuilder`, pass to `NewGenerationService` |

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Unit normalization (pure)

- **File**: `internal/shopping/units.go`
- **Action**: CREATE
- **Implement**:
  - Define three measurement families: **mass** (canonical `g`), **volume**
    (canonical `ml`), **count** (canonical `pcs`). An unrecognized/empty unit is
    its own opaque family keyed by its normalized string (only identical units merge).
  - `unitFamily(unit string) (familyID string, toCanonical float64, ok bool)` using
    `Normalize` on the unit token. Conversions:
    - mass: `g`→1, `kg`→1000.
    - volume: `ml`→1, `l`→1000, `dl`→100, `rkl`/`ст.л.`/`tbsp`/`stl`→15, `tl`/`ч.л.`/`tsp`→5.
      (Finnish: `dl`=100 ml, `rkl` (ruokalusikka)=15 ml, `tl` (teelusikka)=5 ml.)
    - count: `шт`, `kpl`, `pc`, `pcs`, `pce`, `pcs.`→1.
  - `displayAmount(family string, canonicalQty float64) (amount float64, unit string)`:
    mass → `kg` when ≥1000 else `g`; volume → `l` when ≥1000 else `ml`; count → `pcs`;
    opaque → return qty with the original unit. Round to 2 decimals, trim noise.
  - Keep all maps lowercase; match against `Normalize(unit)`.
- **Mirror**: `internal/shopping/normalize.go:15-45` (pure helpers, package-level maps)
- **Validate**: `gofmt -s -l internal/shopping && go vet ./internal/shopping`

### Task 2: Unit tests for normalization

- **File**: `internal/shopping/units_test.go`
- **Action**: CREATE
- **Implement**: Table-driven cases — `kg↔g`, `l/dl/rkl/tl→ml`, `шт`/`kpl`→count,
  unknown unit stays opaque, display thresholds (999 g→g, 1000 g→1 kg), Finnish +
  Russian unit spellings.
- **Mirror**: `internal/shopping/normalize_test.go:5-27`
- **Validate**: `go test ./internal/shopping`

### Task 3: Ingredient→category dictionary (pure)

- **File**: `internal/shopping/categories.go`
- **Action**: CREATE
- **Implement**:
  - `LookupCategory(name string) (domain.IngredientCategory, bool)` — returns a
    category + `true` when a built-in keyword matches, else `("", false)`.
  - Curated multilingual keyword → category map (EN/FI/RU), matched with
    `ContainsTerm(name, keyword)` so inflections/plurals are tolerated. Cover the
    common cases that dominate the `recepy-examples/` set:
    - **produce**: onion/sipuli/лук, carrot/porkkana/морковь, potato/peruna/картоф,
      tomato/tomaatti/помидор, garlic/valkosipuli/чеснок, pepper(bell)/paprika/перец,
      cucumber/kurkku/огурец, lettuce/salaatti/салат, apple/omena/яблок, lemon/sitruuna/лимон, herbs…
    - **meat_fish**: chicken/kana/кур, beef/nauta/говядин, pork/sika·porsa/свин,
      salmon/lohi/лосось, fish/kala/рыба, mince/jauheliha/фарш, shrimp/katkarapu/креветк…
    - **dairy**: milk/maito/молоко, cheese/juusto/сыр, butter/voi/масло сливочн,
      cream/kerma/сливк, yogurt/jogurtti/йогурт, egg/muna/яйц…
    - **pantry**: flour/jauho/мука, sugar/sokeri/сахар, rice/riisi/рис,
      pasta/pasta/макарон, oil/öljy/масло раст, salt/suola/соль, spice/mauste/специ, stock/liemi/бульон…
    - **frozen**: frozen/pakaste/заморож, ice cream/jäätelö/мороженое…
  - Document that this is a "good enough" first pass; the LLM fallback (service)
    catches the long tail and the cache makes repeats free.
- **Mirror**: `internal/service/generation.go:589-616` (`inferProtein` multilingual keyword switch — same spirit)
- **Validate**: `gofmt -s -l internal/shopping && go vet ./internal/shopping`

### Task 4: Dictionary coverage test (95% gate)

- **File**: `internal/shopping/categories_test.go`
- **Action**: CREATE
- **Implement**: Embed a fixture of ingredient names drawn from **5 distinct
  weeks** (hand-pick representative names across RU/FI/EN). Assert the dictionary
  alone resolves a high share, and that *every* name resolves to a known category
  once the LLM fallback is simulated — but the 95% AC is measured on the full
  pipeline in Task 8. Here, assert dictionary hits are correct (no
  mis-categorization) and report coverage. Keep the fixture in the test file so it
  is reviewable.
- **Mirror**: `internal/shopping/normalize_test.go:29-57`
- **Validate**: `go test ./internal/shopping`

### Task 5: Consolidation (pure)

- **File**: `internal/shopping/consolidate.go`
- **Action**: CREATE
- **Implement**:
  - `Consolidate(recipes []domain.Recipe, pantryBasics []string) []domain.ShoppingListItem`.
  - For each ingredient across all recipes: skip when any pantry term matches via
    `ContainsTerm(ingredient.Name, term)` (reuse the dislike-style matcher).
  - Group by key = `Normalize(name)` + unit family (from Task 1). Compatible →
    sum canonical quantities; incompatible units of the same name → separate
    groups (separate lines), satisfying the "1 шт + 100 г stay separate" AC.
  - For display, convert summed canonical back via `displayAmount`; keep the
    first-seen original (pretty) name as `Name`.
  - Assign `Category` from `LookupCategory`; leave `""` (empty) when the
    dictionary misses — the service fills these. Do **not** default to `other`
    here (the service distinguishes "unknown, ask LLM" from a real `other`).
  - Deterministic output order (e.g. by category then name) so tests + the
    eventual UI are stable.
- **Mirror**: `internal/service/generation.go:417-438` (`dislikeViolations` ContainsTerm loop)
- **Validate**: `gofmt -s -l internal/shopping && go vet ./internal/shopping`

### Task 6: Consolidation tests

- **File**: `internal/shopping/consolidate_test.go`
- **Action**: CREATE
- **Implement**: AC-driven — `250 g + 100 g carrot = 350 g`; `kg`+`g` of same
  item merge; `1 шт` + `100 g` of same item → two lines; pantry term excludes the
  line; unknown ingredient gets empty category; output order deterministic.
- **Mirror**: `internal/shopping/normalize_test.go:29-57`
- **Validate**: `go test ./internal/shopping`

### Task 7: Category cache migration + repository

- **File**: `migrations/000003_ingredient_category.up.sql` / `.down.sql`, `internal/repository/ingredient_category.go`
- **Action**: CREATE
- **Implement**:
  - Migration (up):
    ```sql
    CREATE TABLE ingredient_category (
        name_normalized TEXT PRIMARY KEY,
        category        TEXT NOT NULL CHECK (category IN
                         ('produce','meat_fish','dairy','pantry','frozen','other')),
        created_at      TIMESTAMP NOT NULL
    );
    ```
    Down: `DROP TABLE ingredient_category;`. **Note (Risk):** this cache is keyed
    by ingredient name and is *household-agnostic* by design — a carrot is produce
    for everyone — so it intentionally omits `household_id` (the CLAUDE.md
    "every table carries household_id" rule targets per-household *data*, not a
    derived global dictionary cache). Flagged for owner confirmation.
  - Repository methods on `*Store`:
    - `CategoriesByNames(ctx, names []string) (map[string]domain.IngredientCategory, error)`
      — single `IN (...)` query keyed by normalized name; missing names simply
      absent from the map.
    - `SaveCategory(ctx, nameNormalized string, c domain.IngredientCategory) error`
      — `INSERT ... ON CONFLICT(name_normalized) DO NOTHING` (idempotent, safe to retry).
  - `context.WithTimeout(ctx, queryTimeout)`, `formatTime(time.Now().UTC())`.
- **Mirror**: `internal/repository/weeklyplan.go:277-304` (list/scan), `store.go:35-68`
- **Validate**: `go build ./... && go test ./internal/repository`

### Task 8: ShoppingBuilder (service orchestration)

- **File**: `internal/service/shopping.go`
- **Action**: CREATE
- **Implement**:
  - Interfaces (defined in service, kept narrow):
    ```go
    type CategoryCache interface {
        CategoriesByNames(ctx context.Context, names []string) (map[string]domain.IngredientCategory, error)
        SaveCategory(ctx context.Context, nameNormalized string, c domain.IngredientCategory) error
    }
    type ShoppingBuilder struct { client llm.Client; cache CategoryCache }
    func NewShoppingBuilder(client llm.Client, cache CategoryCache) *ShoppingBuilder
    func (b *ShoppingBuilder) Build(ctx context.Context, recipes []domain.Recipe, pantry []string) ([]domain.ShoppingListItem, error)
    ```
  - `Build`: call `shopping.Consolidate`. Collect items with empty `Category`.
    For those: (1) look up the DB cache by normalized name; (2) for cache
    misses, call the LLM per name (`prompts.Load("categorize_ingredient.v1.txt")`,
    render `{{ingredient}}`, `llm.Generate[categoryReply]` with
    `Role: llm.RoleCategorize`, small schema hint), coercing via the
    `normalizeCategory` pattern (export it from generation.go or duplicate);
    (3) `SaveCategory` each newly resolved name; (4) anything still unresolved →
    `CategoryOther`.
  - **Fault tolerance**: never fail the whole week because categorization failed —
    on LLM/cache error, log a warn (`slog.Warn("categorize failed", "err", err)`)
    and default the item to `CategoryOther`. The generic `llm.Generate` already
    handles one JSON repair retry; the provider client owns network backoff.
  - Do not log ingredient lists at info level (privacy) — counts/latency only.
- **Mirror**: `internal/service/generation.go:293-301` (Generate call), `:655-663` (coercion), `:368-408` (prompt render)
- **Validate**: `go build ./... && go vet ./internal/service`

### Task 9: ShoppingBuilder tests

- **File**: `internal/service/shopping_test.go`
- **Action**: CREATE
- **Implement**: reuse `fakeLLM` (generation_test) + a `fakeCache`.
  - Dictionary-hit item → no LLM call, no cache write.
  - Cache-hit unknown → category from cache, no LLM call.
  - Cache-miss unknown → LLM called once, result cached, category applied.
  - LLM error → item defaults to `CategoryOther`, week still builds.
  - **95% AC**: feed the 5-week fixture (shared with Task 4) with the LLM stub
    returning correct categories for dictionary-misses; assert ≥95% of items land
    on a correct, non-arbitrary category.
- **Mirror**: `internal/service/generation_test.go:17-29`
- **Validate**: `go test ./internal/service`

### Task 10: Wire builder into GenerateWeek

- **File**: `internal/service/generation.go`
- **Action**: UPDATE
- **Implement**:
  - Add field `builder shoppingBuilder` (interface: `Build(ctx, recipes, pantry) ([]domain.ShoppingListItem, error)`)
    and extend `NewGenerationService(client, repo, builder)`.
  - In `GenerateWeek`, after `recipes, proteins := toDomainRecipes(...)` and before
    `ArchiveAndCreateWeek`: `plan.ShoppingList, err = g.builder.Build(ctx, recipes, h.PantryBasics)`;
    wrap error `fmt.Errorf("generate week: %w", err)`. `insertPlanWithItems`
    already persists `plan.ShoppingList` atomically — no repo change for create.
- **Mirror**: `internal/service/generation.go:180-189`
- **Validate**: `go build ./...`

### Task 11: Rebuild shopping list on swap

- **File**: `internal/service/generation.go`, `internal/repository/weeklyplan.go`
- **Action**: UPDATE
- **Implement**:
  - The service `SwapRecipe` already holds `kept` recipes + the `newRecipe` — i.e.
    the full new set of three. After mapping `newRecipe`, build
    `items := g.builder.Build(ctx, append(kept, newRecipe), h.PantryBasics)`.
  - Change `SwapRecipeInPlan(ctx, planID, oldRecipeID, newRecipe, items)` to
    **replace** the plan's shopping items: keep the existing `DELETE ... WHERE
    weekly_plan_id = ?` then re-insert `items` (reuse the `itemQ` insert from
    `insertPlanWithItems`; extract a shared `insertShoppingItems(ctx, tx, planID, householdID, createdAt, items)` helper). Update the doc comment (currently
    says "cleared as a forward-compatible invalidation hook for CH-12").
  - Update the `generationRepo` interface signature accordingly.
- **Mirror**: `internal/repository/weeklyplan.go:104-147`, `:180-194`
- **Validate**: `go build ./... && go test ./internal/repository`

### Task 12: Update tests + wiring for new signatures

- **File**: `internal/service/generation_test.go`, `internal/repository/weeklyplan_test.go`, `cmd/server/main.go`
- **Action**: UPDATE
- **Implement**:
  - `generation_test.go`: add a `fakeBuilder` returning canned items (and one
    asserting it receives all three recipes + pantry on swap); pass it to
    `NewGenerationService`; update `fakeGenRepo.SwapRecipeInPlan` to the new
    signature and record `items`. Assert `GenerateWeek` attaches the built list to
    the saved plan.
  - `weeklyplan_test.go`: update `SwapRecipeInPlan` call sites; assert items are
    replaced (old gone, new present) after swap.
  - `main.go`: `builder := service.NewShoppingBuilder(llmClient, store)`;
    `genSvc := service.NewGenerationService(llmClient, store, builder)`
    (confirm `*repository.Store` satisfies `CategoryCache`).
- **Mirror**: `internal/service/generation_test.go:34-91`, `cmd/server/main.go:130-137`
- **Validate**: `go build ./... && go test ./...`

---

## Risks

| Risk | Mitigation |
|------|------------|
| `internal/shopping` accidentally gaining infra deps (ctx/SQL/LLM) | Keep consolidation/units/dictionary pure; all ctx+LLM+DB orchestration in `internal/service/shopping.go`. `go vet`/review check. |
| Cache table omits `household_id`, against the CLAUDE.md blanket rule | Deliberate: it is a derived, household-agnostic dictionary cache, not user data. Flagged in Task 7 + here for owner confirmation; trivially reversible (drop migration) if owner wants per-household. |
| Unit-family map misses a real Finnish/Russian unit → over-splitting | Cover the tech-design unit list (g,kg,ml,l,шт,ст.л.,ч.л.,dl,tl,rkl + FI/RU spellings) in Task 1 with tests; unknown units degrade safely to "opaque family" (separate line), never crash. |
| LLM categorize call fails / unreachable in sandbox | Builder defaults to `CategoryOther` on any error and logs a warn; dictionary already covers the common bulk; week generation never blocks on categorization. |
| Per-ingredient LLM calls add latency/cost | Dictionary handles the majority; DB cache makes repeats free; only first-seen unknowns hit the LLM (cheap Haiku `RoleCategorize`). |
| `categorize_ingredient.v1.txt` is single-ingredient (no batch) | Acceptable — few unknowns per week after dictionary+cache; a batch prompt (`.v2`) is a later optimization, not needed for the AC. |
| Swap signature change ripples through fakes/tests | Task 12 updates every call site in the same pass; `go build ./...` catches stragglers. |

---

## Environment & Verification

| Verification | Runs in env? | If blocked: where/when verified |
|--------------|--------------|---------------------------------|
| `gofmt -s -l .`, `go vet ./...`, `golangci-lint run ./...` | yes | — |
| `go test ./...` (shopping, repository SQLite, service with fakes) | yes | — |
| `go build` (`CGO_ENABLED=0`) | yes | — |
| Live LLM categorize E2E (`RoleCategorize`, real key) | maybe (egress varies by session) | Re-probe at run time per CLAUDE.md; if host blocked / key absent, defer to networked host / Mac mini and record. Not required for AC — dictionary + stubbed-LLM pipeline tests cover it. |
| `govulncheck ./...` | no (vuln.go.dev 403 in sandbox) | No new deps added (stdlib + existing `golang.org/x/text`, `google/uuid`); gated at CH-21. |

---

## Validation

```bash
gofmt -s -l .            # formatting (no output = clean)
go vet ./...             # vet
golangci-lint run ./...  # lint
go test ./...            # tests
# govulncheck ./...      # deferred in sandbox (no new deps); CH-21 gate
```

---

## Acceptance Criteria

- [ ] `WeeklyPlan` creation auto-generates a persisted `shopping_list` (AC-1)
- [ ] Identical ingredients with compatible units are summed (250 g + 100 g = 350 g) (AC-2)
- [ ] Incompatible units (1 шт + 100 g) appear as separate lines (AC-3)
- [ ] Every item carries a `category` ∈ {produce, meat_fish, dairy, pantry, frozen, other} (AC-4)
- [ ] Categorization correct on ≥95% across a 5-week fixture (dictionary + LLM fallback) (AC-5)
- [ ] `pantry_basics` ingredients are excluded from the list (AC-6, US-7)
- [ ] Category cached by name in DB; no repeat LLM call for a known ingredient
- [ ] Shopping list rebuilt (not left empty) after a recipe swap
- [ ] `internal/shopping` stays pure; orchestration in service; SQL only in repository
- [ ] `gofmt`/`go vet`/`golangci-lint`/`go test ./...` all pass
- [ ] Live-LLM E2E recorded as run or deferred-to-networked-host (CH-21 gate)
