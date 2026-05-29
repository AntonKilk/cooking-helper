# Plan: CH-17 — Feedback integration in next-generation prompt

## Summary

The next-week generation prompt already receives recent recipes with their
feedback tags (`formatRecent`/`feedbackTag` in `internal/service/generation.go`),
and the prompt's SOFT GUIDANCE already tells the model to lean toward
liked/cook-again styles and away from disliked ones. CH-17 closes the remaining
gaps: (1) extract the feedback→text serialization into a dedicated, tested
utility (per the issue's technical note); (2) make the history depth **N**
configurable via env (not UI), bumping the default from 10 → 20; (3) strengthen
the signal so previously **disliked** dishes are explicitly flagged "do not
repeat" rather than only soft-steered; (4) bound the prompt context so it cannot
grow past a reasonable token budget. No data-layer changes — `RecentRecipes`
already loads feedback newest-first.

## User Story

As a household member
I want my likes and dislikes to shape the next week's menu automatically
So that the suggestions improve over time without any manual configuration

## Metadata

| Field | Value |
|-------|-------|
| Type | ENHANCEMENT |
| Complexity | LOW (issue: Small) |
| Systems Affected | `internal/service` (generation, new feedback util), `internal/handler` (router wiring), `cmd/server` (env read), `internal/llm/prompts` (template wording) |
| GitHub Issue | #17 (CH-17) |

---

## Current State (what already exists — do NOT rebuild)

- `internal/repository/recipe.go:128` — `RecentRecipes(ctx, householdID, limit)` loads
  up to `limit` recipes newest-first **with feedback columns** decoded. No change needed.
- `internal/service/generation.go:23` — `const recentLimit = 10` (the N to make configurable).
- `internal/service/generation.go:503-530` — `formatRecent` + `feedbackTag` produce lines
  like `"Creamy Pasta [liked, cook again]"`. To be **moved** into a dedicated utility and enriched.
- `internal/service/generation.go:408,343` — both `loadPrompt` and `loadSwapPrompt` call
  `RecentRecipes(..., recentLimit)`. Both must use the new configurable field.
- `internal/llm/prompts/generate_week.v1.txt` — SOFT GUIDANCE already covers liked/disliked
  styling and "avoid repeating recent". Trigger renders `{{.Recent}}` as a bullet list.
- `internal/llm/anthropic/client.go:137-142` — cache breakpoint is on `req.System`
  (instructions + few-shot examples — the large stable block, already cached). Recent
  feedback lives in `req.Prompt` (the variable trigger). See Risk note on the caching scope.

---

## Patterns to Follow

### Functional options (mirror for the service-level config)
```go
// SOURCE: internal/llm/anthropic/client.go:51-63
// Option configures a Client.
type Option func(*Client)

func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) { c.logger = l }
}
```

### Env read at the edge, with default (mirror for FEEDBACK_HISTORY_LIMIT)
```go
// SOURCE: cmd/server/main.go:45-53
port := os.Getenv("PORT")
if port == "" {
	port = defaultPort
}
```

### Tuning constants block (where the default N lives)
```go
// SOURCE: internal/service/generation.go:23-32
const (
	recentLimit       = 10
	generationTimeout = 45 * time.Second
	maxGenTokens      = 4096
	...
)
```

### Prompt-assembly test asserting trigger content (mirror for new util tests)
```go
// SOURCE: internal/service/generation_test.go:353-387 (TestGenerateWeekIncludesHistoryInPrompt)
repo := &fakeGenRepo{recent: []domain.Recipe{
	{Title: "Old Stew", Feedback: &domain.Feedback{Liked: true, CookAgain: true}},
}}
llmClient := &capturingLLM{reply: validWeek()}
svc := newTestGenService(llmClient, repo)
...
if !strings.Contains(llmClient.lastPrompt, "liked, cook again") { ... }
```

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `internal/service/feedback.go` | CREATE | Dedicated feedback→prompt-text serialization utility + token-budget bound. Houses moved `feedbackTag` and new `serializeRecent`. |
| `internal/service/feedback_test.go` | CREATE | Unit tests for the utility: tag formatting, disliked "do not repeat" emphasis, N truncation, per-line length bound, nil feedback. |
| `internal/service/generation.go` | UPDATE | Remove `const recentLimit`; add `recentLimit` field + `WithRecentLimit` option + `defaultRecentLimit = 20`; call the new util; delete moved `formatRecent`/`feedbackTag`. |
| `internal/service/generation_test.go` | UPDATE | Add a test that the configured limit is passed to `RecentRecipes`; keep existing assertions green (still rendered in trigger). |
| `internal/handler/router.go` | UPDATE | Plumb the configured limit into `NewGenerationService(...)` via the new option; accept it through a variadic `RouterOption`. |
| `cmd/server/main.go` | UPDATE | Read `FEEDBACK_HISTORY_LIMIT` (default 20, ignore non-positive), pass to `NewRouter`. |
| `internal/llm/prompts/generate_week.v1.txt` | UPDATE | Tighten wording so disliked past dishes are an explicit "do not regenerate", not only soft guidance. |

---

## Design Decisions

1. **Keep recent feedback in the trigger (`req.Prompt`), not the cached `req.System`.**
   The issue's technical note mentions "кэшируемая часть промпта (prompt caching)".
   The large stable block (instructions + few-shot examples) is *already* the cached
   System block (`anthropic/client.go:137`). Moving the per-household context behind the
   cache breakpoint is a structural change to the prompt split that would alter several
   prompt-assembly tests and yields marginal benefit (recent feedback changes between
   generations, so it would only cache-hit across a single generation's retries). It is
   out of scope for this Small story. Recorded as a follow-up in Risks.

2. **N configurable via `FEEDBACK_HISTORY_LIMIT` env var; default 20** (issue example).
   Read once at the edge (`main.go`), threaded through `NewRouter` → `NewGenerationService`
   via a functional option, mirroring the LLM-client option pattern. Non-positive / unparseable
   values fall back to the default. Never surfaced in the UI.

3. **Disliked-recipe avoidance via explicit prompt flag, not a data filter.** The acceptance
   criterion is "do not reappear (or with very low probability)" — a soft/LLM-side constraint,
   not a hard exclusion. The serializer renders a disliked past dish as
   `"Title [DISLIKED — do not make this again]"`, and the template wording is tightened to
   instruct the model accordingly. (Hard de-duplication against history would be over-engineering
   for "very low probability" and risks starving variety.)

4. **Token budget = bounded line count (N) × bounded line length.** N already caps the number
   of lines. Add a defensive per-title truncation (`maxFeedbackTitleChars = 80`) so a pathological
   long title cannot blow the budget. Document the reasoning in the util.

5. **Single utility serves both generate and swap.** `swapTriggerData.Recent` uses the same
   serializer, so both code paths benefit and stay consistent.

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Create the feedback serialization utility

- **File**: `internal/service/feedback.go`
- **Action**: CREATE
- **Implement**:
  - Package `service`. Add `const maxFeedbackTitleChars = 80`.
  - `func serializeRecent(recipes []domain.Recipe) []string` — for each recipe, build a line:
    `title` (truncated to `maxFeedbackTitleChars`, appending `…` when cut) plus a bracketed tag
    from `feedbackTag`. When the recipe was **disliked**, the bracket reads
    `"[DISLIKED — do not make this again]"` (overrides the neutral tag); otherwise it appends the
    neutral `feedbackTag` (`liked`, `cook again`, combinations) when non-empty.
  - Move `feedbackTag(f *domain.Feedback) string` here verbatim from `generation.go`.
  - Doc comments in the repo's style (start with the symbol name).
- **Mirror**: `internal/service/generation.go:501-530` (existing `formatRecent`/`feedbackTag`).
- **Validate**: `go build ./... && gofmt -s -l internal/service`

### Task 2: Unit-test the utility

- **File**: `internal/service/feedback_test.go`
- **Action**: CREATE
- **Implement**: table tests for —
  - nil feedback → bare title, no brackets;
  - liked + cook-again → `[liked, cook again]`;
  - disliked → `[DISLIKED — do not make this again]` (and not the neutral tag);
  - title longer than `maxFeedbackTitleChars` → truncated with `…`;
  - empty input → empty slice; order preserved (newest-first as received).
- **Mirror**: `internal/service/generation_test.go:252-304` (table-test style) and the
  `strings.Contains` assertion style at `:369-377`.
- **Validate**: `go test ./internal/service/ -run TestSerializeRecent`

### Task 3: Make N configurable on the generation service

- **File**: `internal/service/generation.go`
- **Action**: UPDATE
- **Implement**:
  - Remove `recentLimit = 10` from the const block; add `const defaultRecentLimit = 20`.
  - Add field `recentLimit int` to `GenerationService`.
  - Add `type GenOption func(*GenerationService)` and
    `func WithRecentLimit(n int) GenOption` (ignore `n <= 0`, keeping the default).
  - Change `NewGenerationService(client, repo, builder, opts ...GenOption)`; initialize
    `recentLimit: defaultRecentLimit`, then apply `opts`.
  - In `loadPrompt` and `loadSwapPrompt`, replace `recentLimit` with `g.recentLimit`.
  - Replace the `formatRecent(recent)` calls with `serializeRecent(recent)`; delete the now-moved
    `formatRecent` and `feedbackTag` from this file.
- **Mirror**: option pattern at `internal/llm/anthropic/client.go:51-63`.
- **Validate**: `go build ./... && go vet ./internal/service/`

### Task 4: Test the configurable limit reaches the repository

- **File**: `internal/service/generation_test.go`
- **Action**: UPDATE
- **Implement**:
  - Extend `fakeGenRepo.RecentRecipes` to record the `limit` it was called with (add a
    `recentLimitSeen int` field).
  - Add `TestGenerateWeekUsesConfiguredRecentLimit`: build the service with
    `WithRecentLimit(20)`, run `GenerateWeek`, assert `repo.recentLimitSeen == 20`; and a
    default case (no option) asserting `== defaultRecentLimit`.
  - Confirm existing `TestGenerateWeekIncludesHistoryInPrompt` still passes (serializer keeps
    the `liked, cook again` substring). Add an assertion that a disliked recent recipe renders
    `do not make this again` in the trigger.
- **Mirror**: `internal/service/generation_test.go:47-49` (fake), `:353-387` (prompt assertions).
- **Validate**: `go test ./internal/service/`

### Task 5: Plumb the limit through the router

- **File**: `internal/handler/router.go`
- **Action**: UPDATE
- **Implement**:
  - Add `type RouterOption func(*routerConfig)` with a small `routerConfig{ feedbackHistoryLimit int }`
    (default 0 → let the service default apply).
  - Add `func WithFeedbackHistoryLimit(n int) RouterOption`.
  - Change `NewRouter(..., llmClient llm.Client, opts ...RouterOption)`; resolve config from opts.
  - In the `if canGenerate` block, build the gen service with the option when configured:
    `service.NewGenerationService(llmClient, store, builder, service.WithRecentLimit(cfg.feedbackHistoryLimit))`
    (only pass the option when `cfg.feedbackHistoryLimit > 0`; otherwise call without it).
  - The 3 existing test callers (`*_integration_test.go`, `health_test.go`) pass `nil` llmClient and
    no opts → **compile unchanged** thanks to the variadic.
- **Mirror**: option pattern at `internal/llm/anthropic/client.go:51-63`.
- **Validate**: `go build ./... && go test ./internal/handler/`

### Task 6: Read the env var at the edge

- **File**: `cmd/server/main.go`
- **Action**: UPDATE
- **Implement**:
  - Add `const defaultFeedbackHistoryLimit = 20`.
  - In `run`, read `FEEDBACK_HISTORY_LIMIT`; parse with `strconv.Atoi`; on error or `n <= 0`
    use the default. Log the resolved value (`slog.Info("feedback history limit", "n", n)`).
  - Pass `handler.WithFeedbackHistoryLimit(n)` to `NewRouter`.
- **Mirror**: `cmd/server/main.go:45-53` (PORT/DB_PATH env-with-default).
- **Validate**: `go build ./cmd/server`

### Task 7: Tighten the prompt wording for disliked dishes

- **File**: `internal/llm/prompts/generate_week.v1.txt`
- **Action**: UPDATE
- **Implement**:
  - In SOFT GUIDANCE (or as a new HARD-adjacent line), state that any recent dish tagged
    `DISLIKED` MUST NOT be regenerated or closely re-created, and liked/cook-again dishes are a
    *style* signal (lean toward their flavor/technique, not a copy). Keep it concise; do not
    restructure the OUTPUT CONTRACT or the `---TRIGGER---` split. Phrasing must match the bracket
    text the serializer emits.
  - Do not bump the prompt version filename (behavioral nudge within v1; no contract change).
- **Mirror**: existing SOFT GUIDANCE block in the same file.
- **Validate**: `go test ./internal/service/` (prompt is embedded; tests load it).

---

## Risks

| Risk | Mitigation |
|------|------------|
| Changing `NewGenerationService` signature breaks 4 call sites | Use a **variadic** `opts ...GenOption` — existing positional calls (test helpers at `generation_test.go:119,610,639,656`) compile unchanged. |
| Changing `NewRouter` signature breaks 3 test callers | Use a **variadic** `opts ...RouterOption`; the `nil`-llm test callers pass no opts and compile unchanged. |
| Issue note asks feedback in the *cached* prompt part; we keep it in the trigger | Documented Design Decision #1: the large stable block is already cached; per-household feedback caching is marginal and structurally out of scope. Record as a follow-up note in the PR/issue. |
| Disliked emphasis could over-suppress variety | It's a soft LLM-side nudge ("very low probability"), not a hard filter; the existing dislike-*ingredient* hard constraint and protein-variety checks remain authoritative. |
| Prompt wording drift vs. serializer bracket text | Tests assert the exact substring (`do not make this again`) in both the util test and the trigger test, locking wording and prompt in sync. |

---

## Environment & Verification

| Verification | Runs in env? | If blocked: where/when verified |
|--------------|--------------|---------------------------------|
| `gofmt -s -l .` | yes | — |
| `go vet ./...` | yes | — |
| `golangci-lint run ./...` | yes | — |
| `go test ./...` (unit, fake LLM) | yes | — |
| Static `CGO_ENABLED=0` build | yes | — |
| `govulncheck ./...` (no new deps added) | no | CH-21 (deploy gate); no dependency changes in this story |
| Live LLM behavior (does the model actually avoid disliked dishes) | maybe | Re-probe provider reachability at run time (CLAUDE.md); if blocked, defer to networked host / Mac mini and record under CH-21 |

---

## Validation

```bash
gofmt -s -l .            # formatting (no output = clean)
go vet ./...             # vet
golangci-lint run ./...  # lint
go test ./...            # tests
# govulncheck ./...      # only if deps change (they don't); else CH-21 deploy gate
```

---

## Acceptance Criteria

- [ ] Generation prompt receives the latest N recipes with titles + feedback (default N=20).
- [ ] Disliked past recipes are explicitly flagged "do not make again" and won't reappear (or very rarely).
- [ ] Liked/cook-again recipes influence style via prompt text — not a fixed/forced list.
- [ ] N is configurable via `FEEDBACK_HISTORY_LIMIT` env (default 20); never via UI.
- [ ] Prompt context is bounded (line count = N, per-title length capped).
- [ ] Feedback→text serialization lives in a dedicated, unit-tested utility (`internal/service/feedback.go`).
- [ ] `gofmt`, `go vet`, `golangci-lint`, `go test ./...` all pass.
- [ ] Caching-scope deviation (feedback kept in trigger) recorded as a follow-up.
```
