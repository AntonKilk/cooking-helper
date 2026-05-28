# Plan: CH-10 Disliked-Ingredients Post-Validation

## Summary

Harden the disliked-ingredients hard constraint from CH-8 into a real defense-in-depth
gate. Replace the plain-substring check in `internal/service/generation.go` with an
inflection-tolerant matcher implemented in `internal/shopping` (lowercase + Unicode
NFD strip + token prefix match — handles RU падежи like `гриб → грибы/грибной` and
FI cases like `sieni → sienet/sienillä` without an LLM round-trip). Extend the
retry budget from 1 to **2** semantic retries (3 total LLM attempts), escalating
the prompt accent on each retry, and emit a `slog.Warn("dislike violation", ...)`
on every detected violation so the violation rate is observable from the JSON logs.
After 2 exhausted retries the service still fails closed with `ErrDislikeViolation`
— the handler already maps this to the localized `generate.error_dislikes` partial,
so no UI work is needed.

## User Story

As a user
I want my disliked ingredients to never appear in the weekly menu, even if the LLM forgets
So that I can trust the generator without proofreading every recipe.
(US-5 / PRD §14 Risks — "Disliked ingredients игнорируются LLM")

## Metadata

| Field | Value |
|-------|-------|
| Type | ENHANCEMENT (defense-in-depth on top of CH-8) |
| Complexity | LOW–MEDIUM |
| Systems Affected | `internal/shopping`, `internal/service` |
| GitHub Issue | #10 (CH-10) |
| Blocked by | CH-8 (merged at `332449b`) |

---

## Scope boundaries (explicitly NOT in this story)

- **Shopping-list ingredient consolidation/categorization** (F-3 / its own story).
  We introduce normalization primitives in `internal/shopping`; we do not build
  the shopping list yet.
- **LLM-based matching.** The story permits "нормализация *или* LLM-based matching";
  we choose normalization — adding an LLM round-trip to every generation doubles
  cost and latency, and the normalization heuristic is sufficient for the RU/FI
  inflection cases we care about. Revisit if violations persist in the logs.
- **Metrics infra (Prometheus, OTel, etc.).** No metrics pipeline exists yet; the
  "violation frequency" metric is the count of `dislike violation` warn lines in
  the structured JSON log (`log/slog`). This is what the AC asks for — observable,
  not a counter.
- **Other hard constraints** (`ErrPortionsShort`, `ErrProteinVariety`,
  `ErrGenerationInvalid`) — retry behavior unchanged. Only the dislike path gets
  the new retry budget; the others remain fail-closed on first violation as in CH-8.
- **i18n strings.** `generate.error_dislikes` is already wired in
  `internal/handler/generate.go:107-108` and present in all three locale JSONs;
  no change.

---

## Patterns to Follow

### Service: domain-error sentinels + narrow repo interface + fake-backed tests
```go
// SOURCE: internal/service/generation.go:34-42
var (
    ErrGenerationInvalid = errors.New("service: generation did not return three recipes")
    ErrDislikeViolation  = errors.New("service: generated week includes a disliked ingredient")
    ErrPortionsShort     = errors.New("service: generated week does not cover the week's portions")
    ErrProteinVariety    = errors.New("service: generated week lacks protein variety")
)
```

### Current dislike check + 1-retry (to be replaced)
```go
// SOURCE: internal/service/generation.go:105-119
if bad := dislikeViolations(week, h.DislikedIngredients); len(bad) > 0 {
    retryTrigger := trigger + dislikeHint(bad)
    week, err = g.complete(ctx, system, retryTrigger)
    if err != nil {
        return nil, fmt.Errorf("generate week (dislike retry): %w", err)
    }
    if len(week.Recipes) != 3 {
        return nil, fmt.Errorf("generate week: %w", ErrGenerationInvalid)
    }
    if bad := dislikeViolations(week, h.DislikedIngredients); len(bad) > 0 {
        return nil, fmt.Errorf("generate week: %w", ErrDislikeViolation)
    }
}
```

### Current substring matcher (to be replaced by `shopping.ContainsTerm`)
```go
// SOURCE: internal/service/generation.go:198-216
func dislikeViolations(week generatedWeek, disliked []string) []string {
    var hits []string
    for _, term := range disliked {
        t := strings.ToLower(strings.TrimSpace(term))
        if t == "" { continue }
        for _, r := range week.Recipes {
            for _, ing := range r.Ingredients {
                if strings.Contains(strings.ToLower(ing.Name), t) {
                    hits = append(hits, term); goto next
                }
            }
        }
        next:
    }
    return hits
}
```

### Structured logging at boundaries (slog)
```go
// SOURCE: internal/handler/generate.go:97-100
slog.Warn("week generation failed",
    "err", err,
    "request_id", RequestIDFromContext(r.Context()),
)
```

### Service test with scripted LLM replies
```go
// SOURCE: internal/service/generation_test.go:16-28, 112-131
type fakeLLM struct{ replies []string; calls int }
func (f *fakeLLM) Complete(_ context.Context, _ llm.Request) (llm.Completion, error) {
    i := f.calls; f.calls++
    if i >= len(f.replies) { i = len(f.replies) - 1 }
    return llm.Completion{Text: f.replies[i]}, nil
}
// ...drive the service with replies = [bad, validWeek()] and assert retry path
```

### Package-level doc comment (Go convention used in this repo)
```go
// SOURCE: internal/shopping/doc.go:1-3
// Package shopping consolidates recipe ingredients into a single shopping list
// and categorizes items by store section.
package shopping
```

---

## Design notes

### Normalization algorithm (`internal/shopping`)

Goal: detect that `гриб` (disliked) matches `грибы`/`грибной`/`Fresh Mushrooms` (ingredient).

1. **Normalize**: lower-case → Unicode NFD → drop combining marks (so `é`→`e`,
   `й`→`и` is *not* done — Cyrillic short-i is a distinct letter; only combining
   diacritics drop) → replace anything that is not a letter/digit with a single
   space → collapse runs of whitespace.
2. **Tokenize** on whitespace.
3. **Match a needle against a haystack**: for each needle token, look for a
   haystack token that **starts with** that needle token (covers `гриб`→`грибы`,
   `mushroom`→`mushrooms`, `sieni`→`sienet`). Needle tokens shorter than 3
   characters fall back to *exact* token match (so `or` does not eat `orange`).
   All needle tokens must match (per-token AND) — so `green bean` does not
   match a haystack of just `green peppers`.
4. **Empty / whitespace-only needle** → never matches anything.

This is intentionally simple — no stemmer, no ICU. The 3-char prefix rule is the
key trade-off: long enough that `гриб` (4) and `sieni` (5) work but `or`/`на`
don't blow up. Tested with RU/FI/EN fixtures.

### Retry budget

`maxDislikeRetries = 2` → up to 3 LLM calls total in the dislike path. Each retry
calls `dislikeHint(bad, attempt)` which escalates accent on the final retry
(`THIS IS THE FINAL ATTEMPT — ABSOLUTELY DO NOT use these ingredients: ...`).
Transport-level retries inside `internal/llm` are unchanged and orthogonal.

### Logging shape

```go
slog.Warn("dislike violation",
    "attempt", attempt,        // 1, 2, 3 (final)
    "terms", bad,              // []string of disliked terms that survived
    "household_id", h.ID,
)
```

`request_id` is not threaded into `GenerationService` today (it's handler-local).
That's fine for this story — the warn line will correlate by household+timestamp.
Threading request_id is a cross-cutting change for a separate story.

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `internal/shopping/normalize.go` | CREATE | `Normalize(string) string` + `ContainsTerm(haystack, needle string) bool` — inflection-tolerant ingredient name matching |
| `internal/shopping/normalize_test.go` | CREATE | Table tests for EN/RU/FI cases, plurals, empty input, short-token guard |
| `internal/service/generation.go` | UPDATE | Bump `dislikeViolations` to use `shopping.ContainsTerm`; loop the retry up to `maxDislikeRetries = 2`; escalate hint on final attempt; emit `slog.Warn("dislike violation", ...)` for each detected round |
| `internal/service/generation_test.go` | UPDATE | Update existing dislike-retry test to confirm 1st-retry success path; add: dislike survives once then passes on 2nd retry; dislike survives all 3 attempts → `ErrDislikeViolation`; case+inflection variants caught (RU `гриб`→`грибы`, FI `sieni`→`sienet`, EN `mushroom`→`Fresh Mushrooms`) |

No migrations, no template changes, no i18n changes, no handler changes.

---

## Risks

| Risk | Mitigation |
|------|------------|
| Over-eager matching blocks legitimate ingredients (e.g. disliked `or` matches `orange`) | Per-token match with 3-char minimum for prefix mode; ≤2-char needles require exact token match. Covered by `normalize_test.go` table cases. |
| RU/FI inflection misses (e.g. `яйцо` vs `яйца` — different stems) | Token-prefix on `яйц` matches both. We test the common inflection families and document in `shopping/doc.go` that the matcher is best-effort — the LLM remains the primary defense; this is the safety net. |
| Doubling LLM cost on dislike misses | Capped at 3 total attempts; each emits a token-count log line via the existing provider client. If misses become routine, the log trace gives us the signal to add prompt strengthening or LLM-based matching in a follow-up. |
| Test using `goto next` style (kept from CH-8) is awkward to extend | Rewrite the loop with a labeled early-return helper (`firstMatch(...)`) or split into nested funcs; behavior identical. |
| Transport timeout exhaustion across 3 attempts | The outer `generationTimeout = 45s` (`generation.go:21`) already bounds the total wall-clock; provider client has its own per-call timeout. No change needed. |

---

## Environment & Verification

| Verification | Runs in env? | If blocked: where/when verified |
|--------------|--------------|---------------------------------|
| `gofmt -s -l .` | yes | — |
| `go vet ./...` | yes | — |
| `golangci-lint run ./...` | yes | — |
| `go test ./...` (unit tests for `shopping` + `service`) | yes | — |
| `govulncheck ./...` | **no** (sandbox 403) | CH-21 deploy gate on networked host |
| Live LLM smoke test with a real RU/FI disliked term | **no** (no API key + egress allowlist) | Mac mini at deploy / CH-21 |
| Service Worker / HTTPS behavior | n/a (no SW change) | — |

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Create the normalization utility

- **File**: `internal/shopping/normalize.go`
- **Action**: CREATE
- **Implement**:
  - Package doc already in `doc.go`; this file is `package shopping` with no extra doc.
  - `func Normalize(s string) string` — lowercase, `norm.NFD` strip combining marks (`golang.org/x/text/unicode/norm` + `golang.org/x/text/runes`), replace non-letter/non-digit runes (per `unicode.IsLetter`/`IsDigit`) with space, collapse whitespace, trim.
  - `func ContainsTerm(haystack, needle string) bool`:
    - Normalize both. If either ends up empty → return `false`.
    - Tokenize on whitespace.
    - For each needle token: find at least one haystack token where, if `len(needle token) >= 3`, the haystack token has the needle as a prefix; if `len < 3`, the haystack token equals the needle token.
    - All needle tokens must match → return `true`. Otherwise `false`.
  - Constant: `const minPrefixLen = 3` with a short comment on the trade-off (don't repeat the explanation already in the plan; one line on why 3).
- **Mirror**: `internal/i18n/bundle.go` for package layout and `internal/llm/retry.go` for small-helper style.
- **Dependency note**: adds `golang.org/x/text` to `go.mod`. This is a Go-team-maintained module, already standard in Go projects — fits the CLAUDE.md "actively maintained / standard library first" rule. Run `go mod tidy` after.
- **Validate**: `gofmt -s -l internal/shopping/ && go vet ./internal/shopping/...`

### Task 2: Unit-test the normalizer

- **File**: `internal/shopping/normalize_test.go`
- **Action**: CREATE
- **Implement**: Table-driven `TestNormalize` and `TestContainsTerm` with at minimum:
  - Empty / whitespace needle → false (both functions).
  - EN: `mushroom` ↔ `Fresh Mushrooms` → true; `mushroom` ↔ `mushy pasta` → false (prefix is `mush`, not full token).
  - RU: `гриб` ↔ `грибы`/`грибной соус` → true; `яйцо` ↔ `яйца` → true (shared 3+ prefix `яйц`).
  - FI: `sieni` ↔ `sienet`/`sienillä` → true.
  - Short needle: `or` ↔ `orange` → false (short-token guard); `or` ↔ `or` → true.
  - Multi-token needle: `green bean` ↔ `green beans, chopped` → true; `green bean` ↔ `green peppers` → false.
  - Punctuation/diacritic-insensitive: `crème fraîche` ↔ `Creme Fraiche` → true.
- **Mirror**: `internal/i18n/bundle_test.go` for table-test style.
- **Validate**: `go test ./internal/shopping/...`

### Task 3: Wire the matcher into `dislikeViolations` and extend the retry loop

- **File**: `internal/service/generation.go`
- **Action**: UPDATE
- **Implement**:
  - Add `import "github.com/AntonKilk/cooking-helper/internal/shopping"`.
  - Add `import "log/slog"`.
  - Add a constant near the existing `recentLimit` block:
    ```go
    const maxDislikeRetries = 2 // CH-10: total LLM attempts in dislike path = 1 + maxDislikeRetries
    ```
  - Rewrite `dislikeViolations` to use `shopping.ContainsTerm(ing.Name, term)` instead of `strings.Contains`. The empty/whitespace term guard moves into `ContainsTerm`, so the wrapper just iterates.
  - Replace lines 105–119 in `GenerateWeek` with a loop:
    ```go
    attempt := 1
    for {
        bad := dislikeViolations(week, h.DislikedIngredients)
        if len(bad) == 0 { break }
        slog.Warn("dislike violation",
            "attempt", attempt,
            "terms", bad,
            "household_id", h.ID,
        )
        if attempt > maxDislikeRetries {
            return nil, fmt.Errorf("generate week: %w", ErrDislikeViolation)
        }
        retryTrigger := trigger + dislikeHint(bad, attempt == maxDislikeRetries+1)
        // wait — see below; the "final" hint should fire on the LAST retry, not after
        week, err = g.complete(ctx, system, retryTrigger)
        if err != nil {
            return nil, fmt.Errorf("generate week (dislike retry): %w", err)
        }
        if len(week.Recipes) != 3 {
            return nil, fmt.Errorf("generate week: %w", ErrGenerationInvalid)
        }
        attempt++
    }
    ```
    Clarification on the final-hint flag: pass `final := attempt == maxDislikeRetries` (the retry we're about to issue is the last one). Compute it before the call.
  - Update `dislikeHint(bad []string, final bool) string` to escalate when `final` is true:
    - Default: existing wording.
    - Final: prefix with `THIS IS THE FINAL ATTEMPT. ` and add `If you cannot exclude them, return three recipes that use entirely different proteins and produce.`
- **Mirror**:
  - Retry-loop shape: keep the early-return-on-each-failure style used in CH-8 (no defer-magic); see `internal/llm/retry.go` for cadence reference (though that's transport, not semantic).
  - Logging: shape per `internal/handler/generate.go:97-100`.
- **Validate**: `gofmt -s -l internal/service/ && go vet ./internal/service/... && go build ./...`

### Task 4: Update service tests

- **File**: `internal/service/generation_test.go`
- **Action**: UPDATE
- **Implement**:
  - Keep `TestGenerateWeekDislikeRetrySucceeds` as the "succeeds on 1st retry" case — already covers `replies = [bad, validWeek]`.
  - **Add** `TestGenerateWeekDislikeRetrySucceedsOnSecondRetry`: `replies = [bad, bad2, validWeek]`, expect success and that `repo.saved != nil`.
  - **Update** `TestGenerateWeekDislikePersistsFails` to use 3 bad replies (1 initial + 2 retries) — assert `ErrDislikeViolation` and `repo.saved == nil`. Update the comment to "All three attempts violate".
  - **Add** `TestGenerateWeekDislikeInflection` (table test):
    - RU: disliked `"гриб"`, bad reply ingredient `"грибы"` on attempt 1, valid on 2 → success.
    - FI: disliked `"sieni"`, bad reply ingredient `"sienet"` on attempt 1, valid on 2 → success.
    - EN punctuation: disliked `"creme fraiche"`, bad reply `"Crème Fraîche"` on attempt 1, valid on 2 → success.
  - Optional bonus: capture `slog` output by installing a discard handler in tests; not required by AC — skip unless trivial.
- **Mirror**: existing `TestGenerateWeekDislike*` block, `generation_test.go:112-153`.
- **Validate**: `go test ./internal/service/...`

### Task 5: Full validation sweep

- **File**: n/a (project-wide)
- **Action**: validate
- **Implement**: Run the full local validation suite from CLAUDE.md.
- **Validate**:
  ```bash
  gofmt -s -l .
  go vet ./...
  golangci-lint run ./...
  go test ./...
  go mod tidy
  ```
  Confirm `go.mod` / `go.sum` diffs are limited to the `golang.org/x/text` addition.
  `govulncheck` is environment-blocked — record in the implementation report; gate at CH-21.

---

## Validation

```bash
gofmt -s -l .
go vet ./...
golangci-lint run ./...
go test ./...
# govulncheck ./...        # sandbox-blocked (vuln.go.dev 403) — run on networked host / CH-21
```

---

## Acceptance Criteria

Mapping CH-10 issue AC → tasks:

- [ ] **After each LLM reply, compare all ingredients against the disliked list** — Task 3 (loop body) + `dislikeViolations` over `shopping.ContainsTerm` (Task 1).
- [ ] **Regenerate on violation (max 2 retries) with explicit accent in the prompt** — Task 3 (`maxDislikeRetries = 2`, `dislikeHint(..., final)` escalation).
- [ ] **If 2 retries still violate → show an error, no silent ignoring** — already wired: returns `ErrDislikeViolation` → handler renders `generate.error_dislikes` partial (`internal/handler/generate.go:107-108`). Test covers it (Task 4, `TestGenerateWeekDislikePersistsFails`).
- [ ] **Match accounts for cases and spelling variants (normalization / LLM-based matching)** — Task 1 `Normalize` + `ContainsTerm`; Task 2 RU/FI/EN inflection tests; Task 4 integration test through `GenerateWeek`.
- [ ] **Violation frequency is logged for prompt-quality monitoring** — Task 3 `slog.Warn("dislike violation", attempt, terms, household_id, ...)`.

Plus repo-wide gates:

- [ ] All tasks completed in order
- [ ] `gofmt`, `go vet`, `golangci-lint`, `go test` all green
- [ ] Follows existing patterns (narrow interfaces, sentinel errors, table tests, slog at boundaries)
- [ ] `govulncheck` deferred to CH-21 / networked host and recorded in the implementation report
