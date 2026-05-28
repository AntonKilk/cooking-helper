# Implementation Report

**Plan**: `.agents/plans/completed/ch7-llm-service-abstraction.plan.md`
**Branch**: `claude/prime-7-wON2t`
**GitHub Issue**: #7 (CH-7)
**Status**: COMPLETE

## Update (provider switch to OpenAI)

After the initial Anthropic-only implementation, the owner requested switching to the
OpenAI API. Because `llm.Client` is provider-agnostic, this was an additive change — both
providers now coexist and a caller selects one at wiring time (CH-8).

- Added `internal/llm/openai` (official OpenAI Go SDK v1.12.0, Chat Completions API),
  mirroring the anthropic client: per-attempt timeout, transient retry (network/5xx/429,
  2s→4s→8s, max 3), SDK errors mapped to `llm.ErrTransient`/`ErrTimeout`, token+latency
  logging without prompt contents.
- **Models changed** from Anthropic (`claude-sonnet-4-6` / `claude-haiku-4-5-20251001`)
  to cheap OpenAI analogs: `gpt-5.4-mini` for generation/swap (`openai.ModelGenerate`) and
  `gpt-5.4-nano` for categorization (`openai.ModelCategorize`). GPT-5.4 Mini is cheaper than
  Claude Haiku on input; GPT-5.4 Nano is the cheapest production tier. Exact model IDs to be
  confirmed against `GET /v1/models` during the live test.
- Prompt caching: OpenAI caches long stable prefixes automatically, so there is no explicit
  cache breakpoint (unlike Anthropic's `cache_control`).
- Added a key-gated live integration test (`internal/llm/openai/integration_test.go`) that
  skips unless `OPENAI_API_KEY` is set.
- The Anthropic implementation was kept (owner's choice), so the prior report content below
  still applies to `internal/llm/anthropic`.

## Summary

Implemented the provider-agnostic LLM layer in `internal/llm`. A thin `Client`
interface exposes a single `Complete(ctx, Request) (Completion, error)` call returning
raw model text plus token usage. The Anthropic implementation
(`internal/llm/anthropic`) wraps the official Go SDK (v1.45.0) with an explicit
per-attempt timeout, transient-failure retry (2s→4s→8s, max 3, no 4xx), a prompt-cache
breakpoint on the stable system block, and token/latency logging that never includes
prompt or reply contents. A package-level generic `Generate[T]` decodes the reply JSON
into a typed Go value and performs a single repair retry (with a clarifying hint) before
returning `ErrInvalidJSON`. Prompts are version-controlled `*.v1.txt` files loaded via an
`embed.FS`.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Add Anthropic Go SDK dependency | `go.mod`, `go.sum` | ✅ (v1.45.0) |
| 2 | `Client` interface, types, sentinels, generic `Generate[T]` decoder + repair retry | `internal/llm/client.go` | ✅ |
| 3 | Transport retry helper (exp backoff, transient-only, injectable sleep) | `internal/llm/retry.go` | ✅ |
| 4 | Prompt loader + first versioned prompt | `internal/llm/prompts/embed.go`, `categorize_ingredient.v1.txt` | ✅ |
| 5 | Decoder + repair-retry unit tests | `internal/llm/client_test.go` | ✅ |
| 6 | Retry-helper unit tests | `internal/llm/retry_test.go` | ✅ |
| 7 | Anthropic SDK implementation | `internal/llm/anthropic/client.go`, `doc.go`, `client_test.go` | ✅ |
| 8 | Full validation sweep | — | ✅ (see below) |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ clean |
| `golangci-lint run ./...` | ✅ 0 issues |
| `go test ./...` | ✅ all packages pass (12 new test cases) |
| `govulncheck ./...` | ⚠️ could not run — vuln DB (`vuln.go.dev`) blocked by env network policy (403). Re-run locally. |
| Live API smoke test | ⚠️ not run — `ANTHROPIC_API_KEY` absent in this environment. API egress is reachable; run locally with a key. |

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `internal/llm/client.go` | CREATE | +110 |
| `internal/llm/retry.go` | CREATE | +61 |
| `internal/llm/client_test.go` | CREATE | +96 |
| `internal/llm/retry_test.go` | CREATE | +92 |
| `internal/llm/prompts/embed.go` | CREATE | +22 |
| `internal/llm/prompts/categorize_ingredient.v1.txt` | CREATE | +9 |
| `internal/llm/anthropic/client.go` | CREATE | +159 |
| `internal/llm/anthropic/doc.go` | CREATE | +5 |
| `internal/llm/anthropic/client_test.go` | CREATE | +87 |
| `go.mod` / `go.sum` | UPDATE | SDK + transitive deps |

## Deviations from Plan

1. **Cache control construction.** The plan sketched `CacheControlEphemeralParam{}` (zero
   value). In SDK v1.45.0 that field is tagged `omitzero`, so a zero value is dropped on
   marshal and prompt caching would silently NOT apply. Fixed to
   `sdk.NewCacheControlEphemeralParam()` (sets `Type: "ephemeral"`), and added a test that
   marshals the system block and asserts `"cache_control"` + `"ephemeral"` are present.
2. **`Retry` exported.** The plan kept `retry` unexported. Because `internal/llm/anthropic`
   is a sibling package it can't call an unexported helper, so a thin exported `Retry(ctx, fn)`
   wraps the testable unexported `retry(ctx, attempts, sleep, fn)`. Tests target the
   unexported core with an injected no-op sleep.
3. **`messageAPI` seam.** The anthropic client holds the SDK's `Messages` service behind a
   one-method `messageAPI` interface (documenting the exact surface used). Note the SDK's
   `New` has a pointer receiver, so the client stores `&api.Messages`.
4. **Transient classification includes 429.** Rate-limit (429) is treated as transient
   alongside 5xx/network, which matches the retry intent; other 4xx are permanent.

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/llm/client_test.go` | success (1 call); repair on invalid JSON then success (2 calls); invalid JSON twice → `ErrInvalidJSON` (2 calls); transport error → no repair (1 call) |
| `internal/llm/retry_test.go` | transient then success (delay = 2s); permanent → no retry; exhausts 3 attempts (delays 2s,4s); ctx cancelled mid-backoff → `context.Canceled` |
| `internal/llm/anthropic/client_test.go` | `classify` table (500/429 transient, 400/401 permanent, deadline → timeout, network → transient); `classify(nil)`; `buildParams` sets defaults + ephemeral cache_control (marshaled); `buildParams` with no system block |

## Acceptance Criteria

- [x] `Client` interface takes prompt+schema (`Request`), returns typed object (`Generate[T]`) or error
- [x] `internal/llm/anthropic` implements it via the Anthropic Go SDK
- [x] Explicit `context.WithTimeout` per attempt
- [x] Exp-backoff retry 2s→4s→8s, max 3, on network/5xx/429; 4xx not retried
- [x] Invalid JSON → one repair retry with hint, then `ErrInvalidJSON`
- [x] Versioned prompts in `internal/llm/prompts/` (`*.v1.txt`)
- [x] Prompt caching: stable `System` block marked ephemeral (verified via marshal test)
- [x] Token count + latency logged; prompt/reply contents never logged
- [x] Unit tests: success, retry, fallback
- [~] `govulncheck` — blocked by env network policy; run locally
- [~] Live API smoke — no key in env; run locally

## Follow-ups for the owner (local)

```bash
govulncheck ./...                 # vuln DB unreachable in the web sandbox
ANTHROPIC_API_KEY=... go run ...  # one cheap Haiku call to smoke the live SDK path
```

No wiring into `cmd/server` or any handler/service in this issue — the first consumer is
CH-8 (week generation).
