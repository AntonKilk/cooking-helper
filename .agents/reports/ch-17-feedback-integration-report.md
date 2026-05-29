# Implementation Report

**Plan**: `.agents/plans/completed/ch-17-feedback-integration.plan.md`
**Branch**: `claude/confident-volta-5vyds`
**Status**: COMPLETE
**GitHub Issue**: #17 (CH-17)

## Summary

Closed the remaining gaps for CH-17 (feedback integration in the next-week
generation prompt). The feedback→prompt-text serialization is now a dedicated,
unit-tested utility (`internal/service/feedback.go`); the recent-history depth
**N** is configurable via the `FEEDBACK_HISTORY_LIMIT` env var (default 20, never
exposed in the UI); previously-disliked dishes are flagged with an explicit
do-not-repeat instruction in the prompt; and each history line's length is bounded
so the prompt stays within a reasonable token budget. Recent feedback is kept in
the variable trigger (the large stable few-shot block is already the cached
`System` block) — see Deviations.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Create feedback serialization utility (`serializeRecent`, `feedbackTag`, rune-aware truncation, disliked override) | `internal/service/feedback.go` | ✅ |
| 2 | Unit-test the utility (tags, disliked override, order, truncation, multibyte) | `internal/service/feedback_test.go` | ✅ |
| 3 | Make N configurable: `defaultRecentLimit=20`, `recentLimit` field, `WithRecentLimit` option; swap `formatRecent`→`serializeRecent`; delete moved helpers | `internal/service/generation.go` | ✅ |
| 4 | Test configured/default limit reaches repo + disliked history rendered in trigger | `internal/service/generation_test.go` | ✅ |
| 5 | Plumb limit through router via `RouterOption` / `WithFeedbackHistoryLimit` | `internal/handler/router.go` | ✅ |
| 6 | Read `FEEDBACK_HISTORY_LIMIT` at the edge (default/invalid → 20) | `cmd/server/main.go` | ✅ |
| 7 | Tighten prompt wording: disliked = do-not-repeat; liked/cook-again = style influence | `internal/llm/prompts/generate_week.v1.txt` | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean (no output) |
| `go vet ./...` | ✅ clean |
| `golangci-lint run ./...` (v2.12.2, built with go1.26.3) | ✅ 0 issues |
| `go test ./...` | ✅ all packages pass |
| Static build (`CGO_ENABLED=0 go build ./cmd/server`) | ✅ |
| Runtime smoke (env→config wiring, `/healthz`) | ✅ log shows `"feedback history limit","n":7` for `FEEDBACK_HISTORY_LIMIT=7`; healthz 200 |
| **Live E2E** — POST `/generate` against OpenAI | ✅ 3 valid cards; `gpt-5.4-mini` 5068 in / 1211 out tokens; `gpt-5.4-nano` categorization; constraints satisfied |

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `internal/service/feedback.go` | CREATE | +69 |
| `internal/service/feedback_test.go` | CREATE | +121 |
| `internal/service/generation.go` | UPDATE | +50 / −48 |
| `internal/service/generation_test.go` | UPDATE | +70 / −14 |
| `internal/handler/router.go` | UPDATE | +27 / −3 |
| `cmd/server/main.go` | UPDATE | +28 / −3 |
| `internal/llm/prompts/generate_week.v1.txt` | UPDATE | +4 / −2 |

## Deviations from Plan

- **Feedback kept in the variable trigger, not moved behind the cache breakpoint.**
  As recorded in the plan's Design Decision #1, the large stable block
  (instructions + few-shot examples) is already the cached `System` block
  (`anthropic/client.go:137`). Moving per-household feedback behind the breakpoint
  is a structural change with marginal benefit (feedback changes between
  generations) and out of scope for this Small story. Recommend a follow-up if/when
  per-household cache reuse across retries is measured to matter.
- **golangci-lint** had to be reinstalled at v2 built with go1.26.3 — the
  pre-provisioned binary (go1.25/v1 config) could not load the repo's v2 config.
  Not a code change; noted for environment reproducibility.

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/service/feedback_test.go` | `TestSerializeRecentEmpty`; `TestSerializeRecentTags` (no feedback, liked+cook-again, liked-only, cook-again-only, disliked-override, empty-signal); `TestSerializeRecentDislikedNotNeutralTag`; `TestSerializeRecentPreservesOrder`; `TestSerializeRecentTruncatesLongTitle`; `TestSerializeRecentTruncationRuneAware` |
| `internal/service/generation_test.go` | `TestGenerateWeekRendersDislikedHistory`; `TestGenerateWeekDefaultRecentLimit`; `TestGenerateWeekUsesConfiguredRecentLimit`; `TestWithRecentLimitIgnoresNonPositive` (+ `fakeGenRepo.recentLimitSeen` recording) |

## Acceptance Criteria

- [x] Prompt receives the latest N recipes with titles + feedback (default 20).
- [x] Disliked past recipes flagged "do not make this again" — won't reappear.
- [x] Liked/cook-again influence style via prompt text, not a fixed list.
- [x] N configurable via `FEEDBACK_HISTORY_LIMIT` env (default 20); never via UI.
- [x] Prompt context bounded (line count = N, per-title length capped at 80 runes).
- [x] Feedback serialization is a dedicated, unit-tested utility.
- [x] `gofmt`, `go vet`, `golangci-lint`, `go test ./...` all pass.
- [x] Caching-scope deviation recorded above (follow-up suggested).
