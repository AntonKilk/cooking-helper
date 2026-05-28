# Implementation Report

**Plan**: `.agents/plans/completed/ch-10-disliked-ingredients-post-validation.plan.md`
**Branch**: `claude/prime-10-USbL8`
**Status**: COMPLETE

## Summary

CH-10 hardens the disliked-ingredients hard constraint introduced in CH-8 with
defense in depth:

- New `internal/shopping` package exposes `Normalize` (lowercase + Unicode NFD +
  drop combining marks + collapse non-alphanumerics) and `ContainsTerm` — a
  three-strategy matcher (forward prefix, reverse prefix, single-rune stem drop)
  that catches the common RU/FI/EN inflection patterns (`гриб→грибы/грибной`,
  `mushroom→mushrooms`, `mushrooms→mushroom`, `яйцо→яйца`, `sieni→sienet`)
  without an LLM round-trip.
- `internal/service/generation.go` now retries up to **2** times on dislike
  violations (3 total LLM attempts), escalates the prompt accent on the final
  attempt, and emits `slog.Warn("dislike violation", attempt, terms,
  household_id)` per violation so frequency is observable from the JSON log
  stream.
- After the retry budget is exhausted the service fails closed with the
  pre-existing `ErrDislikeViolation`, which the handler already maps to the
  localized `generate.error_dislikes` partial — no UI changes needed.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Create normalization utility (`Normalize` + `ContainsTerm`) | `internal/shopping/normalize.go` | ✅ |
| 2 | Unit-test the normalizer (EN/RU/FI + edge cases) | `internal/shopping/normalize_test.go` | ✅ |
| 3 | Wire matcher into `dislikeViolations`; loop retry up to `maxDislikeRetries = 2`; escalate hint on final; add `slog.Warn` | `internal/service/generation.go` | ✅ |
| 4 | Add second-retry-success, three-attempt-fail, and inflection-table tests | `internal/service/generation_test.go` | ✅ |
| 5 | Full validation sweep (`gofmt`, `vet`, `golangci-lint`, `go test`, `go mod tidy`) | repo-wide | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ no output |
| `go vet ./...` | ✅ no output |
| `golangci-lint run ./...` | ✅ 0 issues |
| `go test ./...` | ✅ all packages pass (`internal/shopping` 21 cases, `internal/service` updated suite green) |
| `go mod tidy` | ✅ no diff after first run |
| `go build ./...` | ✅ clean |
| `govulncheck ./...` | ⏸ sandbox-blocked (`vuln.go.dev` returns 403) — defer to CH-21 / networked host |

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `internal/shopping/normalize.go` | CREATE | +120 |
| `internal/shopping/normalize_test.go` | CREATE | +64 |
| `internal/service/generation.go` | UPDATE | +47 / −18 |
| `internal/service/generation_test.go` | UPDATE | +86 / −1 |
| `go.mod` | UPDATE | +1 (`golang.org/x/text v0.31.0` promoted from indirect → direct) |
| `go.sum` | UPDATE | +2 |

## Deviations from Plan

1. **Algorithm refinement during Task 2.** The plan specified pure forward
   prefix matching (`HasPrefix(haystack, needle)`). Initial test runs revealed
   two real cases the heuristic missed:
   - RU `яйцо ↔ яйца` (both 4 runes, neither a prefix of the other)
   - FI `sieni ↔ sienet` (`sieni` not a prefix of `sienet` — diverges at the 5th rune)

   Refined to a three-strategy matcher (forward prefix, reverse prefix,
   single-rune stem drop) documented inline in `internal/shopping/normalize.go`.
   The new strategies were verified to *not* regress the false-positive guard
   (`mushroom` vs `mushy`, `berry` vs `blackberry`). One known limit remains
   and is documented in the doc comment: a long inflected needle whose stem
   mutates 2+ runes from the recipe form (e.g. needle `sienet` against hay
   `sieni`) still slips through the matcher — the retry + fail-closed layers
   catch it.

2. **`golang.org/x/text` pinned to v0.31.0 (not `@latest`).** The version was
   already in the transitive graph at 0.31.0; promoting it to a direct dep at
   the same version keeps `go.sum` churn minimal. The plan said "adds the
   dep" without specifying a version.

3. **Plan task 3 had a small mid-block clarification (`final` flag semantics)
   that the implementation resolved as written:** `final := attempt ==
   maxDislikeRetries` (computed before issuing the retry — the upcoming call
   is the last one the budget permits). The merged code matches that resolution.

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/shopping/normalize_test.go` | `TestNormalize` (8 cases: empty, whitespace, lowercase, NFD diacritics, punctuation, digits, Cyrillic, Finnish letters); `TestContainsTerm` (20 cases: empty/whitespace inputs, EN plurals both directions, EN false-positive guards, RU inflection nom/gen/adj, RU eggs stem-mutation, FI plural + adessive, short-needle exact-only, multi-token AND, diacritic insensitivity, substring-inside-token guard) |
| `internal/service/generation_test.go` | Updated `TestGenerateWeekDislikePersistsFails` to use 3 bad replies; added `TestGenerateWeekDislikeRetrySucceedsOnSecondRetry`; added `TestGenerateWeekDislikeInflection` (table: RU `гриб↔грибы`, FI `sieni↔sienet`, EN `creme fraiche↔Crème Fraîche`) |

## Acceptance-Criteria Mapping

| CH-10 AC | Where satisfied |
|----------|-----------------|
| Compare every LLM reply's ingredients against the disliked list | `service/generation.go` retry loop body + `dislikeViolations` over `shopping.ContainsTerm` |
| Retry up to 2 times with explicit prompt accent | `maxDislikeRetries = 2`; `dislikeHint(bad, final)` escalates on the last retry |
| User-facing error after exhaustion, no silent ignoring | Pre-existing `ErrDislikeViolation` → `generate.error_dislikes` i18n partial (handler at `internal/handler/generate.go:107-108`); covered by `TestGenerateWeekDislikePersistsFails` |
| Inflection / spelling tolerance | `shopping.ContainsTerm` (three strategies, RU/FI/EN unit tests + service-layer table test) |
| Violation frequency observable | `slog.Warn("dislike violation", attempt, terms, household_id)` per detected round |

## E2E Verification

The plan's only true E2E hop — a live LLM call with a real RU/FI disliked term —
is sandbox-blocked (no API key + egress allowlist denies provider hosts). Per
the plan's "Environment & Verification" table this is deferred to CH-21 / Mac
mini.

In-sandbox substitutes that *did* run:

- Handler-side integration: existing `internal/handler/generate_test.go`
  (stub generator) passes against the updated error wiring — no regression
  to the friendly-error partial.
- Service-side integration: `TestGenerateWeekDislikeInflection` and
  `TestGenerateWeekDislikeRetrySucceedsOnSecondRetry` drive the full
  `GenerationService.GenerateWeek` flow including LLM retry, post-validation,
  prompt-hint escalation, and `slog.Warn` emission.
