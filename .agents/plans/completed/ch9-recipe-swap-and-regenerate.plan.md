# Plan: CH-9 Recipe Swap & Full Regenerate (F-2)

## Summary

Extend CH-8's one-tap generation with two new actions on the existing weekly
plan: **swap a single recipe** (keeping the other two) and **regenerate the
whole plan** (archiving the previous one, not deleting it). Swap reuses the
generation pipeline through a new `swap_recipe.v1.txt` prompt that receives
the two kept recipes as context (so the model does not duplicate their dish
profile) and targets the remaining portions needed to still cover
`7 × family_size` for the week. Full regenerate calls the existing
`GenerateWeek` after stamping the current plan's new `archived_at` column.
Both actions invalidate the plan's shopping list (which is still empty until
CH-12, but the invalidation hook is wired now). UI exposes a per-card
"Replace" button and a "Regenerate all" button below the cards; both swap the
`#week` fragment via HTMX.

## User Story

As a household member
I want to replace one recipe without losing the other two, or regenerate the
whole selection if none of the three suit me
So that one weak suggestion doesn't force me to redo the whole week and I
keep the recipes I already like.
(PRD US-2, US-3 / F-2)

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY (extends CH-8) |
| Complexity | MEDIUM |
| Systems Affected | `migrations/`, `internal/domain`, `internal/repository`, `internal/llm/prompts`, `internal/service`, `internal/handler`, `templates/`, `i18n/` |
| GitHub Issue | #9 (CH-9) |

---

## Scope boundaries (explicitly NOT in this story)

- **Shopping-list consolidation/categorization (F-3 / CH-12).** Swap and regen
  *invalidate* the list (delete `shopping_list_item` rows for the affected
  plan) and the new plan is created with an empty list — the actual builder
  is CH-12.
- **Restoring an archived plan ("re-activate this week") / archive UI (F-8
  / CH-18).** We *set* `archived_at` here so archive history is correct from
  day one; the browsing UI is CH-18.
- **Auth / per-recipe ownership checks.** Single household for MVP
  (CLAUDE.md). The swap endpoint trusts the URL path; multi-household
  hardening is the same future work as CH-8.
- **Feedback writes (F-5 / CH-16).** Feedback is read by recent-history (as
  in CH-8) and unaffected by swap/regen.

---

## Patterns to Follow

### Migration: versioned up/down SQL embedded into the binary
```sql
-- SOURCE: migrations/000001_init.up.sql:33-41 — plan table shape we extend
CREATE TABLE weekly_plan (
    id           TEXT PRIMARY KEY,
    household_id TEXT      NOT NULL REFERENCES household_profile(id) ON DELETE CASCADE,
    week_start   TEXT      NOT NULL,
    recipe_ids   TEXT      NOT NULL DEFAULT '[]',
    created_at   TIMESTAMP NOT NULL
);
CREATE INDEX idx_weekly_plan_household ON weekly_plan(household_id);
```
```go
// SOURCE: migrations/embed.go — *.sql is embedded; new files are picked up automatically
```

### Repository: `withTx` for atomic multi-table writes; shared row helpers
```go
// SOURCE: internal/repository/weeklyplan.go:30-45 — recipes + plan in one tx
return s.withTx(ctx, func(tx *sql.Tx) error {
    for i := range recipes {
        if err := insertRecipe(ctx, tx, &recipes[i]); err != nil { return ... }
        ids[i] = recipes[i].ID
    }
    p.RecipeIDs = ids
    return insertPlanWithItems(ctx, tx, p)
})
// SOURCE: internal/repository/recipe.go:29-55 — insertRecipe takes an execer (DB or Tx)
// SOURCE: internal/repository/store.go:35-58 — withTx commits/rolls back/handles panics
```

### Service: narrow repo interface + sentinel errors + fakes
```go
// SOURCE: internal/service/generation.go:47-71 — generationRepo interface keeps service unit-testable
type generationRepo interface {
    RecentRecipes(ctx context.Context, householdID string, limit int) ([]domain.Recipe, error)
    CreateWeekWithRecipes(ctx context.Context, p *domain.WeeklyPlan, recipes []domain.Recipe) error
}
// SOURCE: internal/service/generation.go:32-42 — sentinel errors handlers translate to i18n keys
var ErrDislikeViolation = errors.New("service: generated week includes a disliked ingredient")
```

### LLM: provider-agnostic typed call with one-shot JSON repair
```go
// SOURCE: internal/llm/client.go:72-98 — Generate[T] runs Complete + decodes + repairs once
out, err := llm.Generate[generatedSwap](ctx, g.client, llm.Request{
    Role: llm.RoleGenerate, System: system, Prompt: trigger, Schema: schemaHint, MaxTokens: 2048,
})
// SOURCE: internal/service/generation.go:150-190 — system/trigger split on "---TRIGGER---"
//                                                  + text/template-rendered trigger data
```

### Handler: depend on a narrow interface, render fragments, log-not-leak
```go
// SOURCE: internal/handler/generate.go:16-25 — handler depends on `weekGenerator` (stubbable)
type weekGenerator interface {
    GenerateWeek(ctx context.Context, h *domain.HouseholdProfile) (*service.GeneratedWeek, error)
}
// SOURCE: internal/handler/generate.go:96-116 — renderError logs detail, renders localized fragment
// SOURCE: internal/handler/render.go:64-87 — renderFragment buffers first, sets t() for the language
```

### Templates: i18n via `t()`, HTMX attributes, fragment per outcome
```gohtml
{{/* SOURCE: templates/generate.gohtml:1-15 — generate/cards fragment we extend */}}
<button hx-post="/generate" hx-target="#week" hx-indicator="#generate-indicator">
  {{ t "home.generate" }}
</button>
```

### Tests: fake LLM scripted by reply sequence + fake repo recording writes
```go
// SOURCE: internal/service/generation_test.go:15-52 — fakeLLM (canned replies) + fakeGenRepo
// SOURCE: internal/handler/generate_test.go:25-44 — stubGenerator + HX-Request header on httptest
```

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `migrations/000002_weekly_plan_archive.up.sql` | CREATE | `ALTER TABLE weekly_plan ADD COLUMN archived_at TIMESTAMP NULL;` + `CREATE INDEX idx_weekly_plan_active ON weekly_plan(household_id) WHERE archived_at IS NULL;` |
| `migrations/000002_weekly_plan_archive.down.sql` | CREATE | Drop the index + (SQLite-safe) recreate-without-column rollback |
| `internal/domain/plan.go` | UPDATE | Add `ArchivedAt *time.Time` to `WeeklyPlan` |
| `internal/repository/weeklyplan.go` | UPDATE | (a) Persist + scan `archived_at`; (b) add `CurrentWeeklyPlan(ctx, householdID)`; (c) add `ArchiveAndCreateWeek(ctx, prevID, p, recipes)` (one tx); (d) add `SwapRecipeInPlan(ctx, planID, oldRecipeID, newRecipe)` (one tx: insert recipe, mutate recipe_ids, clear shopping_list_item) |
| `internal/repository/recipe.go` | UPDATE | Add `RecipesByIDs(ctx, ids []string) ([]Recipe, error)` — loads kept recipes for the swap prompt; preserves the requested ID order |
| `internal/repository/weeklyplan_test.go` | UPDATE | Add tests: archive sets timestamp, `CurrentWeeklyPlan` ignores archived, `ArchiveAndCreateWeek` atomicity, `SwapRecipeInPlan` rotates IDs + clears items |
| `internal/repository/recipe_test.go` | UPDATE | Add `RecipesByIDs` test (preserves order, returns `ErrNotFound` if any id missing) |
| `internal/llm/prompts/swap_recipe.v1.txt` | CREATE | System + trigger for single-recipe replacement; receives kept recipes + remaining-portions target |
| `internal/service/generation.go` | UPDATE | (a) Make `GenerateWeek` archive the previous active plan in the same atomic write (load current → call `ArchiveAndCreateWeek`); (b) add `SwapRecipe(ctx, h, plan, oldRecipeID)`; (c) extend `generationRepo` interface; (d) extract a shared `mapRecipe(generatedRecipe, h)` helper |
| `internal/service/generation_dto.go` | UPDATE | Add `generatedSwap struct { Recipe generatedRecipe }` decoded by `llm.Generate[generatedSwap]` |
| `internal/service/generation_test.go` | UPDATE | Add tests for `SwapRecipe` (happy path, dislike retry, variety/portions, kept-recipe context appears in prompt) and for archive-previous behavior of `GenerateWeek` |
| `internal/handler/generate.go` | UPDATE | (a) Add `Swap(w, r)` handler reading `{recipeID}` from path; (b) extend `weekGenerator` interface; (c) include `PlanID` in `cardsData` so the template can render per-card swap buttons; (d) when rendering after generation, also load/expose the active plan id (returned from service) |
| `internal/handler/generate_test.go` | UPDATE | Add `TestSwapRendersCards` (success), `TestSwapRendersErrors` (dislike / variety / portions / generic), `TestSwapMissingRecipeID` (404 fragment) |
| `internal/handler/router.go` | UPDATE | Register `POST /generate/swap/{recipeID}` alongside `POST /generate`; keep both gated by `canGenerate` |
| `templates/generate.gohtml` | UPDATE | (a) Render each card with a `<form hx-post="/generate/swap/{id}" hx-target="#week">` "Replace" button; (b) add a "Regenerate all" button below cards (`hx-post="/generate" hx-target="#week"`); (c) wire `#generate-indicator` (already in home) for both |
| `i18n/en.json`, `i18n/ru.json`, `i18n/fi.json` | UPDATE | New keys: `recipe.replace`, `home.regenerate_all`, `generate.swapping`, `generate.error_swap` (and reuse existing `generate.error_*` for dislike/portions/variety) |

---

## Design detail

### Why a single `archived_at TIMESTAMP NULL` column (vs. a `status` enum)

- One household has at most one *current* plan at any time; the partial index
  `WHERE archived_at IS NULL` keeps the "current plan" lookup O(1) without
  enforcing uniqueness (we don't want UNIQUE because regen races would error
  instead of just superseding).
- "Archived" is a one-way transition for MVP (no un-archive UI until CH-18);
  an enum buys us no flexibility we'll use this phase.
- Trade-off accepted: when CH-18 adds richer states (e.g. "favourited"),
  it can layer them as their own columns; archived_at remains stable.

### Migration strategy (SQLite-safe)

- **Up**: `ALTER TABLE weekly_plan ADD COLUMN archived_at TIMESTAMP NULL;`
  is supported by SQLite as long as the new column is NULLable or has a
  literal default. Add `CREATE INDEX idx_weekly_plan_active ON
  weekly_plan(household_id) WHERE archived_at IS NULL;` for the current-plan
  lookup.
- **Down**: SQLite doesn't support `DROP COLUMN` before 3.35; use the
  rename-and-copy pattern in `.down.sql` (`CREATE TABLE weekly_plan_old AS
  SELECT ... FROM weekly_plan;` etc.). Modern SQLite (3.35+) does support
  `DROP COLUMN`; emit `ALTER TABLE weekly_plan DROP COLUMN archived_at;`
  (the project pins recent SQLite via the Go driver — confirm the bundled
  version at implementation time; fall back to the rename pattern if older).

### Domain change (additive only)
```go
type WeeklyPlan struct {
    // ... existing fields
    ArchivedAt *time.Time // nil = currently active plan
}
```
`*time.Time` (not zero-valued) makes "is this plan archived?" unambiguous
without sentinel times. The existing `WeekStart` stays as `time.Time` because
it's always set.

### Repository additions

```go
// CurrentWeeklyPlan returns the household's single active plan, or ErrNotFound.
// "Active" = most recent row with archived_at IS NULL. Multiple actives are
// not expected (we archive before creating); ORDER BY created_at DESC is a
// safety net if a race ever produced two.
func (s *Store) CurrentWeeklyPlan(ctx context.Context, householdID string) (*domain.WeeklyPlan, error)

// ArchiveAndCreateWeek stamps archived_at on the previous plan (if non-empty)
// AND inserts the new plan + its recipes in a single transaction, so a partial
// switch never lands. Mirrors CreateWeekWithRecipes but with the archive step.
func (s *Store) ArchiveAndCreateWeek(ctx context.Context, previousPlanID string, p *domain.WeeklyPlan, recipes []domain.Recipe) error

// SwapRecipeInPlan replaces one recipe in an existing plan atomically:
//   1) INSERT the new recipe (UUID + timestamps assigned)
//   2) UPDATE weekly_plan.recipe_ids by replacing oldRecipeID with newRecipe.ID
//      (preserving order)
//   3) DELETE FROM shopping_list_item WHERE weekly_plan_id = planID
//      (invalidation hook for CH-12)
// The old recipe row is kept (archive history). Returns ErrNotFound if the
// plan is missing or oldRecipeID is not in its recipe_ids.
func (s *Store) SwapRecipeInPlan(ctx context.Context, planID, oldRecipeID string, newRecipe *domain.Recipe) error
```

`recipe_ids` is JSON in a TEXT column: read → decode → splice → encode →
write inside the tx. The 3-element slice keeps this trivial and avoids
SQLite JSON1 dependency.

### LLM prompt — `swap_recipe.v1.txt`

Two halves separated by `---TRIGGER---` (matching the existing convention so
the service can reuse the splitter).

**System block (cacheable):**
- Role: "You replace ONE dinner recipe in an existing weekly plan."
- Hard rules:
  1. Return exactly ONE recipe.
  2. Disliked ingredients are forbidden (verbatim from generate_week).
  3. `servings ≥ target_servings_for_this_recipe` (passed in trigger).
  4. The new recipe MUST NOT duplicate the dish profile of the two kept
     recipes (different main protein OR clearly different style/cuisine).
  5. Output language matches household language.
  6. Same units/category enums as generate_week.
- Output contract (single recipe wrapped in `{"recipe": {...}}`).

**Trigger template variables:**
- `Language`
- `Adults`, `Kids`
- `TargetServings` (integer = total target − sum of kept servings, clamped ≥ 1)
- `Disliked` (list)
- `Pantry` (list)
- `Recent` (list of titles + feedback tags — exactly the same formatting as
  generate_week, so the service helper is shared)
- `Kept` (list of two compact summaries: `Title | protein | top-3 ingredient
  names | one-line description`) — enough for the model to recognize the
  dish profiles without bloating the trigger.

Few-shot examples (`recipe_examples.v1.txt`) are **NOT appended** to swap
calls: those examples are tuned for the multi-recipe `generate_week`
contract, and the single-recipe contract is tight enough that the system
block + the in-trigger kept-recipe summaries suffice. (Reassess if quality
slips on real provider runs at CH-21.)

### Service additions

```go
// SwappedRecipe is what SwapRecipe returns to the handler.
type SwappedRecipe struct {
    Plan    *domain.WeeklyPlan // current plan with updated RecipeIDs
    Recipe  domain.Recipe     // the inserted replacement
    Protein string            // for the card emoji
}

// SwapRecipe replaces oldRecipeID in plan with a freshly generated recipe.
// Validation: exactly 1 recipe, dislikes excluded (1 semantic retry, then
// ErrDislikeViolation), new.Servings ≥ remaining target, combined protein
// variety across the 3 recipes ≥ 2 (ErrProteinVariety). On success the
// repository call rotates the plan's recipe_ids and invalidates the
// shopping list in one tx.
func (g *GenerationService) SwapRecipe(ctx context.Context, h *domain.HouseholdProfile, plan *domain.WeeklyPlan, oldRecipeID string) (*SwappedRecipe, error)
```

`GenerateWeek` changes minimally: it loads the household's current plan
via `repo.CurrentWeeklyPlan`, then writes through
`repo.ArchiveAndCreateWeek(ctx, prevID, plan, recipes)` instead of the
current `CreateWeekWithRecipes`. When there is no prior plan, `prevID == ""`
and the call behaves exactly like CH-8. (`CreateWeekWithRecipes` stays
on the Store for direct use in tests that pre-existed CH-9; the service
just calls the new method.)

### Handler additions

- `POST /generate/swap/{recipeID}` → `gh.Swap`:
  1. Load household + current plan (`gen.CurrentPlan(...)` via narrow
     interface). If no current plan or `{recipeID}` not in
     `plan.RecipeIDs` → render `generate/error` with `generate.error` (HTTP
     200; HTMX swaps the fragment; log the real reason).
  2. Call `gen.SwapRecipe(ctx, h, plan, recipeID)`. Map sentinels exactly
     like `Generate` does (`ErrDislikeViolation` → `generate.error_dislikes`,
     etc.); fall through to `generate.error` for transient/LLM errors.
  3. On success, reload the **3 cards** (the unchanged 2 + the new one) and
     render `generate/cards`. The two kept cards are reconstructed from
     `plan.RecipeIDs` minus the old id, plus the new one, in their original
     positions; protein emojis for the kept ones come from a small helper
     `proteinFromRecipe(Recipe) string` that infers the bucket from
     ingredient names (good enough for emoji-only; if it returns "", we
     fall through to `other 🍽`). The new recipe's protein is taken from
     the swap result.
  4. Card view model gains a `PlanID` so each "Replace" button can target
     the right URL.

- `POST /generate` (existing): unchanged URL, now also archives any active
  plan via the service path.

### Templates

`generate/cards` becomes:

```gohtml
{{- define "generate/cards" -}}
  <h2 class="week__heading">{{ t "home.week_heading" }}</h2>
  <ul class="week__cards">
    {{- range .Cards }}
    <li class="recipe-card">
      <a class="recipe-card__link" href="/recipe/{{ .ID }}"
         hx-get="/recipe/{{ .ID }}" hx-target="#content" hx-push-url="true">
        <span class="recipe-card__emoji" aria-hidden="true">{{ .Emoji }}</span>
        <h3 class="recipe-card__title">{{ .Title }}</h3>
        <p class="recipe-card__time">{{ t "recipe.cook_time" .CookTime }}</p>
        <p class="recipe-card__desc">{{ .Description }}</p>
      </a>
      <button type="button" class="recipe-card__swap"
              hx-post="/generate/swap/{{ .ID }}"
              hx-target="#week"
              hx-indicator="#generate-indicator">
        {{ t "recipe.replace" }}
      </button>
    </li>
    {{- end }}
  </ul>
  <button type="button" class="week__regenerate"
          hx-post="/generate" hx-target="#week"
          hx-indicator="#generate-indicator">
    {{ t "home.regenerate_all" }}
  </button>
{{- end -}}
```

The existing `#generate-indicator` in `home.gohtml` already shows
"home.generating" — we add `generate.swapping` as a second visible state by
toggling `hx-indicator` text via a small CSS-only swap, or (simpler for
MVP) keep the single localized indicator: "Updating the week…" wording
shared by all three actions (key: `home.working`). I'll add **one new key
`home.working`** and reuse it for both generate and swap indicators; the
existing `home.generating` stays so we don't break CH-8 strings. (No-op
alternative: keep `home.generating` for everything. Decide at impl time;
default to the no-op alternative if anyone hesitates.)

### i18n keys

Per locale:

- `recipe.replace`: "Replace" / "Заменить" / "Vaihda"
- `home.regenerate_all`: "Regenerate all" / "Перегенерировать всё" / "Luo kaikki uudelleen"
- `generate.error_swap` (optional, unused if we reuse `generate.error`): kept
  out of the default plan to avoid string churn — reuse `generate.error`.

### Risks

| Risk | Mitigation |
|------|------------|
| `swap_recipe.v1.txt` cannot be live-tested in the web sandbox (no LLM key; OpenAI host blocked) | All swap logic unit-tested with `fakeLLM` scripted replies (kept-recipe context appears in trigger; dislike retry path; variety/portion edges). Live end-to-end deferred to a networked dev host / Mac mini, gated by **CH-21**. |
| Protein bucket for the two kept recipes is lost (CH-8 didn't store it) | Card emoji for kept recipes is rebuilt by a lightweight ingredient-name heuristic (`proteinFromRecipe`); when unsure it returns `other` → `🍽`. The variety check, by contrast, uses the *new* recipe's reported protein plus the kept recipes' inferred protein — if both inferences land in the same bucket, the model could pass variety check with only 1 protein. **Mitigation:** the swap prompt explicitly forbids duplicating the kept recipes' dish profile, and the post-validation falls back to comparing the new protein against the most-frequent inferred bucket; if equal across all 3 → `ErrProteinVariety`. Documented limitation; revisit if CH-16/CH-17 introduce a per-recipe `protein` column. |
| Re-generation race: archive-then-create runs in two SQL statements, so a crash between them could leave a household with no active plan | Both run inside a single `withTx` (`ArchiveAndCreateWeek`). |
| SQLite version variation around `DROP COLUMN` in the down migration | Use the rename-recreate pattern in `.down.sql` so it works on any SQLite ≥ 3.25; document the choice in the migration comment. |
| `recipe_ids` mutation conflicts with a concurrent swap on the same plan | SQLite is single-writer (CLAUDE.md) and a household has one user at a time — accepted. The swap repo method re-reads `recipe_ids` inside the tx and aborts with `ErrNotFound` if `oldRecipeID` is gone (someone else already swapped). |
| LLM returns a recipe whose servings barely meet `TargetServings` and a kept recipe's servings change later | Not a real risk here: kept recipes are immutable post-creation in MVP; if CH-12+ introduces scaling, swap will need to re-target. Noted, not blocking. |
| Old recipe rows accumulate forever | Acceptable: archive is a feature (CH-18 search). No cleanup needed in MVP; `recipe.created_at` already supports time-based pruning later. |
| Changing the `generationRepo` interface breaks CH-8 tests | Extend the interface additively; the existing fake (`fakeGenRepo`) gains the new methods (`CurrentWeeklyPlan`, `ArchiveAndCreateWeek`, `SwapRecipeInPlan`, `RecipesByIDs`) so unit tests stay in lock-step with the production `*Store`. |

---

## Environment & Verification

| Verification | Runs in env? | If blocked: where/when verified |
|--------------|--------------|---------------------------------|
| `gofmt -s -l .` | yes | — |
| `go vet ./...` | yes | — |
| `golangci-lint run ./...` | yes | — |
| `go test ./...` (service/handler/repo with fakes + httptest, in-memory SQLite for repo) | yes | — |
| Static `CGO_ENABLED=0 go build ./...` | yes | — |
| **Live LLM swap call** (real provider) | **no** (no key; OpenAI host blocked per CLAUDE.md sandbox note) | networked dev host / Mac mini; deploy-gated by **CH-21** |
| **Live LLM full regenerate** | **no** | same — CH-21 |
| `govulncheck ./...` (no new deps planned) | **no** | vuln.go.dev 403 in sandbox; run on networked host / CH-21 |
| Service-Worker / PWA behavior of the new fragments | **no** | tailnet HTTPS on Mac mini (CH-21) |
| `migrate up` against a real SQLite file | yes | — (the in-memory test harness runs migrations as part of `newTestStore`) |

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Migration — `archived_at` on `weekly_plan`

- **File**: `migrations/000002_weekly_plan_archive.up.sql` (CREATE), `migrations/000002_weekly_plan_archive.down.sql` (CREATE)
- **Action**: CREATE
- **Implement**:
  - `.up.sql`: `ALTER TABLE weekly_plan ADD COLUMN archived_at TIMESTAMP NULL;` then `CREATE INDEX idx_weekly_plan_active ON weekly_plan(household_id) WHERE archived_at IS NULL;`
  - `.down.sql`: drop the partial index, then drop the column via the SQLite rename-recreate pattern (or `ALTER TABLE ... DROP COLUMN` if the bundled SQLite is ≥ 3.35 — note the chosen pattern in a SQL comment).
- **Mirror**: `migrations/000001_init.up.sql` (table shape + index naming convention); `migrations/embed.go` already globs `*.sql` so no Go change needed.
- **Validate**: `go test ./internal/repository/...` (the test harness applies all migrations on startup, so the new file must parse and apply cleanly).

### Task 2: Domain — `ArchivedAt` on `WeeklyPlan`

- **File**: `internal/domain/plan.go`
- **Action**: UPDATE
- **Implement**: Add `ArchivedAt *time.Time` to `WeeklyPlan`. Update the doc comment to note that `nil = currently active`. No method changes.
- **Mirror**: `internal/domain/recipe.go:43-58` (struct doc style, `*Feedback` for "optional" — same nullable pointer idiom).
- **Validate**: `go build ./...`

### Task 3: Repository — read/write `archived_at`

- **File**: `internal/repository/weeklyplan.go`
- **Action**: UPDATE
- **Implement**:
  - Extend the `weekly_plan` `INSERT` (`insertPlanWithItems`) to include `archived_at` (nullable, written as `sql.NullString` when `ArchivedAt != nil`).
  - Extend `GetWeeklyPlan` SELECT + scan to load `archived_at` into `WeeklyPlan.ArchivedAt`.
- **Mirror**: `internal/repository/recipe.go:204-233` (`feedbackColumns` / `scanFeedback` — same nullable-pointer pattern, including `parseTime` round-trip).
- **Validate**: `go test ./internal/repository/...`

### Task 4: Repository — `CurrentWeeklyPlan`

- **File**: `internal/repository/weeklyplan.go`
- **Action**: UPDATE
- **Implement**: `func (s *Store) CurrentWeeklyPlan(ctx context.Context, householdID string) (*domain.WeeklyPlan, error)` — `SELECT ... FROM weekly_plan WHERE household_id = ? AND archived_at IS NULL ORDER BY created_at DESC LIMIT 1`, reusing the same field decoding as `GetWeeklyPlan` (extract a shared scanPlan helper if it keeps the code clean). Returns `ErrNotFound` when no active plan.
- **Mirror**: `internal/repository/recipe.go:127-152` (`RecentRecipes` — ordered list query); `internal/repository/household.go:104-112` (`FirstHousehold` ordering pattern).
- **Validate**: extend `internal/repository/weeklyplan_test.go` — `TestCurrentWeeklyPlanIgnoresArchived` (seed two plans, archive the older, expect the newer back; archive both, expect `ErrNotFound`). `go test ./internal/repository/...`

### Task 5: Repository — `ArchiveAndCreateWeek`

- **File**: `internal/repository/weeklyplan.go`
- **Action**: UPDATE
- **Implement**: `func (s *Store) ArchiveAndCreateWeek(ctx context.Context, previousPlanID string, p *domain.WeeklyPlan, recipes []domain.Recipe) error` — in one `withTx`: if `previousPlanID != ""`, `UPDATE weekly_plan SET archived_at = ? WHERE id = ? AND archived_at IS NULL` (rows-affected 0 is OK — could be a race); then run the same INSERT logic as `CreateWeekWithRecipes`. Factor the recipe-insert and plan-insert helpers so both methods share them.
- **Mirror**: `internal/repository/weeklyplan.go:30-45` (`CreateWeekWithRecipes` — same tx shape); `internal/repository/store.go:35-58` (`withTx`).
- **Validate**: extend `weeklyplan_test.go` — `TestArchiveAndCreateWeekAtomicity` (seeds an active plan, runs Archive+Create, asserts the old plan is archived AND the new one is active AND, with an injected duplicate-ID failure on the new plan, the old plan stays unarchived after rollback). `go test ./internal/repository/...`

### Task 6: Repository — `SwapRecipeInPlan`

- **File**: `internal/repository/weeklyplan.go`
- **Action**: UPDATE
- **Implement**: `func (s *Store) SwapRecipeInPlan(ctx context.Context, planID, oldRecipeID string, newRecipe *domain.Recipe) error` — in one `withTx`:
  1. Lock-load (`SELECT ... FROM weekly_plan WHERE id = ?`) and decode `recipe_ids`. Return `ErrNotFound` if missing or if `oldRecipeID` not present.
  2. `insertRecipe(ctx, tx, newRecipe)` (assigns UUID/timestamps).
  3. Splice: replace `oldRecipeID` with `newRecipe.ID` at the same index; re-encode and `UPDATE weekly_plan SET recipe_ids = ? WHERE id = ?`.
  4. `DELETE FROM shopping_list_item WHERE weekly_plan_id = ?` (invalidation).
- **Mirror**: `internal/repository/weeklyplan.go:49-82` (`insertPlanWithItems` for encode/exec); `internal/repository/recipe.go:29-55` (`insertRecipe(ex execer, ...)`).
- **Validate**: extend `weeklyplan_test.go` — `TestSwapRecipeInPlanRotatesIDs` (3-recipe plan; swap middle id; assert order preserved, new recipe persisted, shopping items cleared); `TestSwapRecipeInPlanNotFound` (unknown old id → `ErrNotFound`). `go test ./internal/repository/...`

### Task 7: Repository — `RecipesByIDs`

- **File**: `internal/repository/recipe.go`
- **Action**: UPDATE
- **Implement**: `func (s *Store) RecipesByIDs(ctx context.Context, ids []string) ([]domain.Recipe, error)` — `SELECT recipeColumns FROM recipe WHERE id IN (?, ?, ...)` (placeholder count = `len(ids)`). Scan into a map keyed by id, then return `[]domain.Recipe` in the requested order. Return `ErrNotFound` if any id is missing (kept recipes for swap must all exist).
- **Mirror**: `internal/repository/recipe.go:127-152` (`RecentRecipes` — query+scan loop); `internal/repository/recipe.go:74-122` (`recipeColumns` + `scanRecipe`).
- **Validate**: extend `internal/repository/recipe_test.go` — `TestRecipesByIDsPreservesOrder`, `TestRecipesByIDsMissingReturnsNotFound`. `go test ./internal/repository/...`

### Task 8: Prompt — `swap_recipe.v1.txt`

- **File**: `internal/llm/prompts/swap_recipe.v1.txt`
- **Action**: CREATE
- **Implement**: System block (role, hard rules including 100% dislike exclusion, the single-recipe JSON contract `{"recipe": {...}}`, store-category/protein-tag enums, "do not duplicate the dish profile of the kept recipes") + `---TRIGGER---` delimiter + trigger template referencing `{{.Language}}`, `{{.Adults}}`, `{{.Kids}}`, `{{.TargetServings}}`, `{{.Disliked}}`, `{{.Pantry}}`, `{{.Recent}}` (same format as `generate_week.v1.txt`), and a new `{{.Kept}}` range — each kept entry rendered as `Title | protein | top-3-ingredients | one-line-desc`. Picked up automatically by the existing `*.txt` embed glob.
- **Mirror**: `internal/llm/prompts/generate_week.v1.txt` (whole structure + `---TRIGGER---` delimiter); `internal/llm/prompts/embed.go` (load contract).
- **Validate**: `go test ./internal/llm/...`

### Task 9: Service DTO — `generatedSwap`

- **File**: `internal/service/generation_dto.go`
- **Action**: UPDATE
- **Implement**: Add `type generatedSwap struct { Recipe generatedRecipe \`json:"recipe"\` }`. No other DTO changes.
- **Mirror**: `internal/service/generation_dto.go:1-27` (current DTO style with json tags).
- **Validate**: `go build ./...`

### Task 10: Service — archive-previous in `GenerateWeek`

- **File**: `internal/service/generation.go`
- **Action**: UPDATE
- **Implement**:
  - Extend the `generationRepo` interface with `CurrentWeeklyPlan(ctx, householdID) (*domain.WeeklyPlan, error)` (must return repo's `ErrNotFound` when none) and `ArchiveAndCreateWeek(ctx, previousPlanID string, p *WeeklyPlan, recipes []Recipe) error`.
  - In `GenerateWeek`, after validation passes: `prev, err := g.repo.CurrentWeeklyPlan(ctx, h.ID)`; on `errors.Is(err, repository.ErrNotFound)` use `prevID = ""`. Call `g.repo.ArchiveAndCreateWeek(ctx, prevID, plan, recipes)` instead of `CreateWeekWithRecipes`. (The interface does not need to retain `CreateWeekWithRecipes`; remove it from the interface — the production `Store` still has it for repo tests.)
- **Mirror**: `internal/service/generation.go:88-135` (current `GenerateWeek` flow); CLAUDE.md fault-tolerance rule about idempotency.
- **Validate**: update the existing `fakeGenRepo` in `generation_test.go` to satisfy the new interface (return `ErrNotFound` from `CurrentWeeklyPlan` by default; record the `previousPlanID` for assertion). Add `TestGenerateWeekArchivesPreviousPlan` (seed a prev plan id; assert it's passed through to `ArchiveAndCreateWeek`). `go test ./internal/service/...`

### Task 11: Service — `SwapRecipe`

- **File**: `internal/service/generation.go`
- **Action**: UPDATE
- **Implement**:
  - Extend `generationRepo` with `RecipesByIDs(ctx, ids []string) ([]Recipe, error)` and `SwapRecipeInPlan(ctx, planID, oldRecipeID string, newRecipe *Recipe) error`.
  - Add `func (g *GenerationService) SwapRecipe(ctx, h, plan, oldRecipeID) (*SwappedRecipe, error)`:
    1. Validate `oldRecipeID ∈ plan.RecipeIDs`; else `ErrGenerationInvalid`.
    2. Load kept recipes via `RecipesByIDs` (the other two ids).
    3. Compute `targetServings = max(1, targetPortions(h.FamilySize) - sum(kept.Servings))`.
    4. Render trigger from `swap_recipe.v1.txt` (split on `---TRIGGER---`, same as `loadPrompt`; share a helper). Call `llm.Generate[generatedSwap]` with `Role: RoleGenerate`, `MaxTokens: 2048`.
    5. Validate exactly 1 recipe; dislikes check (same `dislikeViolations` helper, one semantic retry with `dislikeHint`); `recipe.Servings ≥ targetServings` (else `ErrPortionsShort`); combined protein variety with the kept recipes' inferred protein (else `ErrProteinVariety`).
    6. Map to `domain.Recipe` (shared `mapRecipe` helper extracted from `toDomainRecipes`).
    7. `repo.SwapRecipeInPlan(ctx, plan.ID, oldRecipeID, &newRecipe)`. Update `plan.RecipeIDs` locally to reflect the swap. Return `SwappedRecipe{Plan: plan, Recipe: newRecipe, Protein: protein}`.
  - Wrap the whole thing in `context.WithTimeout(ctx, generationTimeout)` like `GenerateWeek` does.
- **Mirror**: `internal/service/generation.go:88-135` (flow); `internal/service/generation.go:280-307` (`toDomainRecipes` — extract `mapRecipe` for reuse); `internal/service/generation.go:198-222` (`dislikeViolations` / `dislikeHint` — reused as-is).
- **Validate**: extend `internal/service/generation_test.go`:
  - `TestSwapHappyPath` (1 recipe replies, validates persistence, recipe_ids rotated locally).
  - `TestSwapDislikeRetrySucceeds` (1 bad reply then 1 clean reply; retry trigger contains the offending term).
  - `TestSwapDislikeRetryFails` (`ErrDislikeViolation`).
  - `TestSwapPortionsShort` (kept = 5+5, target = 14, swap returns 3 → `ErrPortionsShort`).
  - `TestSwapProteinVariety` (kept inferred = poultry+poultry, swap returns poultry → `ErrProteinVariety`).
  - `TestSwapKeptContextInPrompt` (capturingLLM: assert kept titles appear in the trigger).
  Run `go test ./internal/service/...`

### Task 12: Handler — `Swap` + view-model updates

- **File**: `internal/handler/generate.go`
- **Action**: UPDATE
- **Implement**:
  - Extend `weekGenerator` with `CurrentPlan(ctx, householdID string) (*domain.WeeklyPlan, error)` (thin pass-through to `repo.CurrentWeeklyPlan`, added as a method on `*GenerationService` in the previous task or directly on the service) and `SwapRecipe(...)`.
  - Add `PlanID string` to `cardsData` so the template can build per-card URLs; populate it in `Generate` (`week.Plan.ID`).
  - Add `Swap(w, r)`: read `r.PathValue("recipeID")`; if empty → render the generic error fragment (HTTP 200). Load household + current plan; if no plan or recipe not in plan → same. Call `gen.SwapRecipe`. Map sentinel errors via the existing `errorMessageKey`. On success, reconstruct the full 3-card view: load the two kept recipes by their ids (via a new narrow `recipeLoader` interface — `RecipesByIDs`), build cards in order (`plan.RecipeIDs` is already rotated), infer emojis for kept ones via a new `proteinFromRecipe(Recipe) string` helper (simple keyword match against ingredient names: `chicken|kana → poultry`, `beef|nauda → red_meat`, `salmon|lohi → fish`, `pork|sika → pork`, `shrimp|katkarapu → seafood`; else `""` → `other`), and use the returned protein for the new card.
- **Mirror**: `internal/handler/generate.go:67-92` (`Generate` flow); `internal/handler/generate.go:47-62` (`proteinEmoji` map + `emojiFor`).
- **Validate**: extend `internal/handler/generate_test.go` with the four tests listed in **Files to Change**. `go test ./internal/handler/...`

### Task 13: Router — register the swap route

- **File**: `internal/handler/router.go`
- **Action**: UPDATE
- **Implement**: Inside the existing `if canGenerate { ... }` block, add `mux.HandleFunc("POST /generate/swap/{recipeID}", gh.Swap)`. The `gh` struct already carries everything `Swap` needs once `weekGenerator` is extended (Task 12).
- **Mirror**: `internal/handler/router.go:50-53` (existing generate-only block).
- **Validate**: `go test ./internal/handler/...`

### Task 14: Templates — per-card swap + regenerate-all button

- **File**: `templates/generate.gohtml`
- **Action**: UPDATE
- **Implement**: Render the per-card "Replace" button (`hx-post="/generate/swap/{{.ID}}"`, `hx-target="#week"`, `hx-indicator="#generate-indicator"`) and the "Regenerate all" button (`hx-post="/generate"`) as shown in the **Templates** section above. Keep the existing card anchor structure; the swap button sits inside the `<li>` but **outside** the anchor (so clicking it doesn't navigate). Wrap the buttons in a `<form>` only if HTMX needs it — `<button>` with `hx-post` is fine and is what the existing generate button uses.
- **Mirror**: `templates/home.gohtml:22-28` (existing HTMX button pattern); `templates/generate.gohtml:1-19` (current fragment shape).
- **Validate**: rerun `go test ./internal/handler/...` — the existing card test now also has to match the new buttons; update assertions to include "Replace" and "Regenerate all" strings (the test was English-only with EN bundle).

### Task 15: i18n — new strings

- **File**: `i18n/en.json`, `i18n/ru.json`, `i18n/fi.json`
- **Action**: UPDATE
- **Implement**: Add the two keys `recipe.replace` and `home.regenerate_all` to each file. EN: "Replace" / "Regenerate all". RU: "Заменить" / "Перегенерировать всё". FI: "Vaihda" / "Luo kaikki uudelleen". (No `generate.error_swap` — the existing `generate.error` is reused.)
- **Mirror**: `i18n/en.json:5-12` (existing key style), `i18n/fi.json` / `i18n/ru.json` (matching keys).
- **Validate**: `go test ./internal/i18n/...` (if a test enforces all-locales-have-same-keys; otherwise covered by handler tests that render the cards with each language bundle).

### Task 16: Final validation pass

- **Action**: VALIDATE
- **Implement**: Run the full local validation suite from CLAUDE.md (skip the sandbox-blocked ones per the Environment table above).
- **Validate**:
  ```bash
  gofmt -s -l .
  go vet ./...
  golangci-lint run ./...
  go test ./...
  CGO_ENABLED=0 go build ./cmd/server
  ```

---

## Validation

```bash
# In the web sandbox, these are the checks that DO run (CLAUDE.md › Validation › sandbox constraints):
gofmt -s -l .
go vet ./...
golangci-lint run ./...
go test ./...
CGO_ENABLED=0 go build ./cmd/server

# Deferred to a networked host / Mac mini, gated by CH-21:
#   - live LLM swap + regenerate calls
#   - govulncheck ./...
#   - Service-Worker / PWA behavior over tailnet HTTPS
```

---

## Acceptance Criteria

- [ ] Migration `000002_weekly_plan_archive` applies cleanly (up + down) against in-memory SQLite in tests
- [ ] `domain.WeeklyPlan.ArchivedAt` round-trips through the repository
- [ ] `CurrentWeeklyPlan` returns the most recent non-archived plan and `ErrNotFound` otherwise
- [ ] `ArchiveAndCreateWeek` archives the previous plan AND creates the new plan in one transaction (rollback on failure leaves the previous plan unarchived)
- [ ] `SwapRecipeInPlan` rotates `recipe_ids` in place, persists the new recipe, and clears `shopping_list_item` rows — all in one transaction
- [ ] `GenerateWeek` archives the previous active plan (when one exists) and is a no-op-archive otherwise
- [ ] `SwapRecipe` validates: exactly 1 recipe, dislike exclusion with one semantic retry, remaining-portions coverage, combined protein variety
- [ ] `POST /generate/swap/{recipeID}` replaces one card and re-renders the 3-card fragment; sentinel errors map to localized i18n keys; internal error detail never reaches the client (`generate_test.go` asserts the leak guard, as in CH-8)
- [ ] `POST /generate` (existing) still works for first-time generation AND for "regenerate all"; the previous plan is archived on regen
- [ ] Per-card "Replace" button + "Regenerate all" button render in all three locales (EN/RU/FI)
- [ ] `gofmt`, `go vet`, `golangci-lint`, `go test` all pass; static `CGO_ENABLED=0` build succeeds
- [ ] Environment-blocked verifications recorded in the Environment table with their CH-21 / networked-host gate
