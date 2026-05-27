# Plan: CH-7 LLM Service Abstraction with Retry & JSON Parsing

## Summary

Build a provider-agnostic LLM layer in `internal/llm`. A thin `Client` interface exposes a
single `Complete(ctx, Request) (Completion, error)` call that returns raw model text plus
token usage. The Anthropic implementation (`internal/llm/anthropic`) wraps the official Go
SDK, sets an explicit per-call timeout, retries transient (network/5xx) failures with
exponential backoff (2s→4s→8s, max 3), marks the stable system block for prompt caching, and
logs token count + latency (never prompt contents). A package-level generic helper
`Generate[T]` decodes the model's JSON into a typed Go value and, on invalid JSON, performs a
single repair retry with a clarifying hint before failing. Prompts live as versioned files
under `internal/llm/prompts/` loaded via an `embed.FS`. All higher features (CH-8 week
generation, swap, categorization) depend only on this interface — no SDK calls leak into
handlers/services.

## User Story

As a developer
I want a provider-agnostic LLM service with retry logic and strict JSON parsing
So that downstream features (week generation, swap, categorization) build on one stable interface

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY |
| Complexity | MEDIUM |
| Systems Affected | `internal/llm`, `go.mod`/`go.sum` |
| GitHub Issue | #7 (CH-7) |

---

## Pre-flight: Validate the external integration (CLAUDE.md "Validate Before Implementing")

Before writing the Anthropic implementation, confirm in this order:

1. **Authorization / accessible** — `ANTHROPIC_API_KEY` is provisioned as a server-side env
   var (per CLAUDE.md). Do **not** hardcode it; read via `os.Getenv` at wiring time only.
2. **Dependency installs** — run `go get github.com/anthropics/anthropic-sdk-go@latest` and
   confirm the module resolves under the environment network policy. **If `go get` is blocked
   by the network policy, stop and confirm with the owner** rather than vendoring or guessing.
3. **API surface still valid** — confirm in the resolved SDK version: the message-create call,
   the streaming/non-streaming response shape, the `Usage` fields (input/output tokens), the
   error type used for HTTP status (to classify 4xx vs 5xx), and the `cache_control: ephemeral`
   field on a system content block (prompt caching). Pin these to the actual installed version,
   not memory.
4. **govulncheck** — run `govulncheck ./...` after the dep is added (CLAUDE.md requires it
   before adding/bumping a library).

> The SDK call itself is not unit-tested against the live API. Unit tests target the interface,
> the retry helper, and the JSON decoder using fakes (see Tasks 5–6). Confirming the live call
> is a manual smoke step (Task 7).

---

## Patterns to Follow

### Narrow interface + constructor injection (service depends on a small interface, fakeable)
```go
// SOURCE: internal/service/household.go:27-46
type householdRepo interface {
	FirstHousehold(ctx context.Context) (*domain.HouseholdProfile, error)
	// ...
}
type HouseholdService struct { repo householdRepo }
func NewHouseholdService(repo householdRepo) *HouseholdService { return &HouseholdService{repo: repo} }
```
→ `llm.Client` is the narrow interface; `anthropic.Client` is the concrete impl; downstream
services accept the `llm.Client` interface.

### Sentinel errors that shield callers from the underlying library
```go
// SOURCE: internal/repository/errors.go:5-7
// ErrNotFound ... shields callers from database/sql sentinels (sql.ErrNoRows never escapes this package).
var ErrNotFound = errors.New("repository: not found")
```
→ `llm` defines `ErrInvalidJSON`, `ErrTransient`, `ErrTimeout`; the anthropic impl maps SDK
error types to these so `sql`-style leakage never happens (no SDK error types escape `internal/llm`).

### Error wrapping with context
```go
// SOURCE: internal/service/household.go:58-64
return nil, fmt.Errorf("current household: %w", err)
```
→ Use `fmt.Errorf("llm generate: %w", err)` etc.

### Explicit per-call timeout via context
```go
// SOURCE: internal/handler/health.go:14-22
const healthPingTimeout = 2 * time.Second
ctx, cancel := context.WithTimeout(r.Context(), healthPingTimeout)
defer cancel()
```
→ `anthropic.Client.Complete` wraps the inbound ctx with `context.WithTimeout(ctx, c.timeout)`.

### Structured logging without secrets/payloads
```go
// SOURCE: cmd/server/main.go:32-33, CLAUDE.md Observability
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
// log boundaries with request_id; never log prompt contents — token count + latency only
```

### Package doc.go (every package has one)
```go
// SOURCE: internal/llm/doc.go:1-4
// Package llm defines a provider-agnostic Client interface ...
```

### Test patterns: table tests, in-memory fake implementing the interface, `errors.Is`
```go
// SOURCE: internal/service/household_test.go:13-50, 117-145
type fakeRepo struct { ... }            // fake implements the interface, no infra
cases := []struct{ name string; ... }{} // table-driven
if !errors.Is(err, ErrInvalidFamilySize) { t.Fatalf(...) }
```
→ Use a `fakeClient` implementing `llm.Client` (returns scripted text/errors) to test
`Generate[T]` and the repair retry; use a counting closure to test the backoff helper.

---

## Design Decisions

- **Two-layer retry, by concern:**
  - *Transport retry* (network/5xx, max 3, 2s→4s→8s) lives in the anthropic impl, around the
    SDK call, using a shared `retry` helper in `internal/llm`. 4xx is non-retryable.
  - *JSON-validity retry* (max 1, with clarifying hint) lives in the package-level generic
    `Generate[T]`, which calls `Client.Complete` a second time with an appended repair hint.
  - These are independent and separately testable, matching the two distinct acceptance bullets.
- **Typed output via generics:** `Client.Complete` stays non-generic (interfaces can't have
  generic methods) and returns raw text + usage. `func Generate[T any](ctx, c Client, req Request) (T, error)`
  unmarshals into `T`. This keeps the interface mockable and the decode logic in one place.
- **Prompt caching:** `Request.System` is the stable, cacheable block (household profile +
  disliked + pantry + recent feedback, assembled by future callers). The anthropic impl sets
  `cache_control: ephemeral` on this block; `Request.Prompt` is the variable trigger and is
  not cached.
- **Backoff is injectable** (a `sleep func(time.Duration)` field or unexported var) so tests
  don't actually wait 2+4 seconds.

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `internal/llm/client.go` | CREATE | `Client` interface, `Request`/`Completion`/`Usage`/`Model` types, sentinel errors, generic `Generate[T]` decoder + JSON-repair retry |
| `internal/llm/retry.go` | CREATE | Transport `retry` helper (exp backoff, max 3, classifies retryable vs permanent), injectable sleep |
| `internal/llm/client_test.go` | CREATE | Tests: `Generate[T]` success, JSON-repair retry (1x then success / then fail → `ErrInvalidJSON`), fail-fast on transport error |
| `internal/llm/retry_test.go` | CREATE | Tests: retry on transient succeeds, no retry on permanent (4xx), max-3-attempts cap, backoff sequence |
| `internal/llm/prompts/embed.go` | CREATE | `//go:embed *.txt` `embed.FS` + `LoadPrompt(name string) (string, error)` |
| `internal/llm/prompts/categorize_ingredient.v1.txt` | CREATE | First versioned prompt (placeholder, establishes the `*.v1.txt` pattern) |
| `internal/llm/anthropic/client.go` | CREATE | SDK-backed `Client`: per-call timeout, transport retry, prompt-cache on system block, slog token/latency, SDK-error→sentinel mapping |
| `internal/llm/anthropic/doc.go` | CREATE | Package doc |
| `internal/llm/doc.go` | KEEP | Already accurate; no change |
| `go.mod` / `go.sum` | UPDATE | Add `github.com/anthropics/anthropic-sdk-go` |

> No wiring into `cmd/server/main.go` or any handler/service in this issue — CH-7 delivers the
> abstraction only. Wiring happens in CH-8 when a feature first needs it. (Keeps the package
> from being dead-code flagged is acceptable: it's a library package with tests.)

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Add the Anthropic Go SDK dependency

- **File**: `go.mod`, `go.sum`
- **Action**: UPDATE
- **Implement**: `go get github.com/anthropics/anthropic-sdk-go@latest`. Record the resolved
  version. If the network policy blocks the download, STOP and confirm with the owner.
- **Validate**: `go mod tidy && govulncheck ./...`

### Task 2: Define the provider-agnostic interface and typed decoder

- **File**: `internal/llm/client.go`
- **Action**: CREATE
- **Implement**:
  - `Model` string type + consts `ModelSonnet = "claude-sonnet-4-6"`, `ModelHaiku = "claude-haiku-4-5-20251001"`.
  - `Request{ Model Model; System string; Prompt string; Schema string; MaxTokens int; RequestID string }`
    (`System` = cacheable stable block; `Schema` = optional schema text reused in the repair hint).
  - `Usage{ InputTokens, OutputTokens int }`, `Completion{ Text string; Usage Usage }`.
  - `Client interface { Complete(ctx context.Context, req Request) (Completion, error) }`.
  - Sentinel errors: `ErrInvalidJSON`, `ErrTransient`, `ErrTimeout` (via `errors.New`).
  - `func Generate[T any](ctx context.Context, c Client, req Request) (T, error)`: call
    `Complete`; on transport error wrap+return immediately; `json.Unmarshal` into `T`; on
    unmarshal error, retry **once** with `req.Prompt + repair hint` (hint references the JSON
    error and `req.Schema`); on second failure return zero `T` + `ErrInvalidJSON`.
- **Mirror**: interface/constructor shape from `internal/service/household.go:27-46`; sentinel
  style from `internal/repository/errors.go:5-7`; error wrapping from `internal/service/household.go:58`.
- **Validate**: `gofmt -s -l internal/llm && go vet ./internal/llm/... && go build ./...`

### Task 3: Implement the transport retry helper

- **File**: `internal/llm/retry.go`
- **Action**: CREATE
- **Implement**: `func retry(ctx context.Context, attempts int, sleep func(time.Duration), fn func() error) error`.
  Run `fn`; if it returns an error that `errors.Is(err, ErrTransient)` and attempts remain,
  sleep `2s, 4s, 8s` (respecting `ctx.Done()`), retry; otherwise return the error. Non-transient
  errors return immediately (no retry — covers 4xx). Cap at `attempts` (3). `sleep` is a param
  so tests inject a no-op.
- **Mirror**: timeout/ctx idiom from `internal/handler/health.go:14-22`.
- **Validate**: `go vet ./internal/llm/... && go build ./...`

### Task 4: Prompt loader + first versioned prompt

- **File**: `internal/llm/prompts/embed.go`, `internal/llm/prompts/categorize_ingredient.v1.txt`
- **Action**: CREATE
- **Implement**: `embed.go` with `//go:embed *.txt` `var FS embed.FS` and
  `func Load(name string) (string, error)` (wraps `fs.ReadFile`, `fmt.Errorf("load prompt %q: %w", ...)`).
  `categorize_ingredient.v1.txt` = a short placeholder prompt establishing the `*.v1.txt`
  naming pattern (the real generate_week/swap prompts arrive with CH-8).
- **Mirror**: embed pattern from `templates/embed.go` / `static` / `i18n` embeds.
- **Validate**: `go build ./... && go test ./internal/llm/prompts/...` (build-only is fine)

### Task 5: Unit tests for the decoder and JSON-repair retry

- **File**: `internal/llm/client_test.go`
- **Action**: CREATE
- **Implement**: `fakeClient` implementing `Client` with a scripted slice of
  `(Completion, error)` responses and a call counter. Tests:
  - success: valid JSON on first call → typed value, 1 call.
  - repair-success: invalid JSON then valid JSON → typed value, exactly 2 calls.
  - repair-exhausted: invalid JSON twice → `errors.Is(err, ErrInvalidJSON)`, exactly 2 calls.
  - transport-error: `Complete` returns `ErrTransient` → `Generate` returns it, no decode, 1 call.
- **Mirror**: fake + table-test + `errors.Is` style from `internal/service/household_test.go:13-50, 117-145`.
- **Validate**: `go test ./internal/llm/...`

### Task 6: Unit tests for the retry helper

- **File**: `internal/llm/retry_test.go`
- **Action**: CREATE
- **Implement**: counting closure + no-op `sleep`. Tests:
  - transient then success → succeeds, attempt count correct.
  - permanent (non-`ErrTransient`) error → returns immediately, 1 attempt.
  - always transient → exactly 3 attempts then returns last error.
  - ctx cancelled mid-backoff → returns ctx error.
- **Mirror**: table-test style from `internal/service/household_test.go:127-145`.
- **Validate**: `go test ./internal/llm/...`

### Task 7: Anthropic SDK implementation

- **File**: `internal/llm/anthropic/client.go`, `internal/llm/anthropic/doc.go`
- **Action**: CREATE
- **Implement**:
  - `Client` struct holding the SDK client, `timeout time.Duration`, `logger *slog.Logger`.
  - `func New(apiKey string, opts ...Option) *Client` (functional options for timeout/logger;
    apiKey read from env by the *caller*, passed in — never read inside via hardcode).
  - `Complete(ctx, req)`: wrap ctx with `context.WithTimeout(ctx, c.timeout)`; build the SDK
    message request — system block from `req.System` with `cache_control: ephemeral` (prompt
    caching), user block from `req.Prompt`, `MaxTokens`, model from `req.Model`; run the call
    inside `llm.retry(...)`; classify SDK errors → wrap network/5xx as `llm.ErrTransient`, 4xx
    as a permanent error, `context.DeadlineExceeded` → `llm.ErrTimeout`; on success map SDK
    usage → `llm.Usage` and return `llm.Completion`.
  - Logging: `slog.Info("llm complete", "model", req.Model, "input_tokens", u.In, "output_tokens", u.Out, "latency_ms", ms, "request_id", req.RequestID)` — **no prompt/response text**.
  - `doc.go`: package doc.
- **Mirror**: timeout idiom `internal/handler/health.go:14-22`; slog usage `cmd/server/main.go:32-33` + CLAUDE.md Observability; sentinel-mapping intent from `internal/repository/errors.go`.
- **Validate**: `gofmt -s -l internal/llm && go vet ./... && go build ./...`. Then a **manual
  smoke test** (real key, one cheap Haiku call) to confirm the live SDK call, usage fields, and
  cache_control field are correct against the installed SDK version. Do not commit any key.

### Task 8: Full validation sweep

- **File**: —
- **Action**: validate
- **Implement**: run the full CLAUDE.md gate; fix any lint/vet/test issues.
- **Validate**: see Validation section.

---

## Validation

```bash
gofmt -s -l .            # formatting — no output = clean
go vet ./...             # vet
golangci-lint run ./...  # lint
go test ./...            # tests
govulncheck ./...        # dependency vulnerabilities (after adding the SDK)
```

---

## Risks

| Risk | Mitigation |
|------|------------|
| `go get` blocked by environment network policy | Pre-flight step 2 — if blocked, STOP and confirm with owner before proceeding (per CLAUDE.md external-integration checklist) |
| SDK API shape differs from memory (usage fields, cache_control, error types) | Pin to the actually-installed version (pre-flight step 3); confirm via SDK source/godoc, not assumption |
| Interfaces can't have generic methods | `Complete` returns raw text; typed decode is the free generic func `Generate[T]` — keeps `Client` mockable |
| Tests sleeping real backoff seconds | `retry` takes an injectable `sleep func(time.Duration)`; tests pass a no-op |
| Accidental secret/payload logging | Log only token counts + latency + request_id; never `req.System`/`req.Prompt`/response text (CLAUDE.md Security/Observability) |
| Package looks like dead code (no caller yet) | Acceptable — library package with full unit tests; first consumer is CH-8 |

---

## Acceptance Criteria

- [ ] `Client` interface in `internal/llm` takes prompt+schema (via `Request`) and returns a typed Go object (via `Generate[T]`) or error
- [ ] `internal/llm/anthropic` implements it using the Anthropic Go SDK
- [ ] Explicit `context.WithTimeout` on every call
- [ ] Exponential-backoff retry (2s→4s→8s, max 3) on network/5xx; 4xx not retried
- [ ] Invalid JSON → one repair retry with clarifying hint, then `ErrInvalidJSON`
- [ ] Prompts are versioned files in `internal/llm/prompts/` (`*.v1.txt`)
- [ ] Prompt caching: stable `System` block marked cacheable
- [ ] Token count + latency logged; prompt contents never logged in prod
- [ ] Unit tests cover success, retry, and fallback-on-error
- [ ] Full validation gate passes (gofmt, vet, golangci-lint, test, govulncheck)
- [ ] Follows existing patterns (narrow interface, sentinel errors, error wrapping, table tests)
