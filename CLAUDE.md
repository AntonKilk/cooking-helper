# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> **Status:** Pre-code. The repo currently holds planning artifacts only
> (`.agents/PRDs/PRD.md`, `.agents/tech-design.md`, `.agents/stories/stories.md`).
> The stack below is **locked-in** by the tech design — build against it, do not
> re-litigate it. When code lands, update the Commands/Key Files sections to match reality.

## Project Overview

Cooking Helper — a home meal-planning assistant for a family in Finland. One tap
generates a weekly menu of 3 recipes (portioned for 7 days of eating) plus a
consolidated, store-categorized shopping list. Personalization comes from feedback
(like / dislike / cook-again), a disliked-ingredients list, and a pantry-basics list —
not from manual configuration. UI is RU/FI/EN from the first commit, optimized for an
iPad on the kitchen counter.

Architecture is **home-network-first**: data lives centrally on a home server (Mac mini),
the iPad is a thin client on the home network. The iPad does not work away from home —
this is an accepted trade-off (shopping happens via Apple Reminders export, Phase 2).

Full context: [`.agents/tech-design.md`](.agents/tech-design.md) and
[`.agents/PRDs/PRD.md`](.agents/PRDs/PRD.md).

---

## Tech Stack

| Technology | Purpose |
|------------|---------|
| **Go** | Backend, single binary, native concurrency. Default language for this owner. |
| **`html/template` + HTMX** | Server-side rendering. No SPA, no client-side model duplication. |
| **SQLite** (`database/sql`, no ORM) | Single-file DB in a Docker volume. Single-writer is fine for one household. |
| **`golang-migrate`** (or `goose`) | Schema migrations. Never edit schema by hand. |
| **LLM SDKs (Anthropic + OpenAI)** | LLM calls behind a provider-agnostic `internal/llm` interface. Either provider plugs in; call sites stay provider-neutral. |
| **`log/slog`** | Structured JSON logging (built-in, no external lib). |
| **Docker + docker-compose** | Single container on Mac mini Intel i7. |
| **Tailscale Serve** | HTTPS inside the tailnet only (HTTPS is required for Service Worker). No Funnel. |
| **PWA** (manifest + Service Worker) | Install-to-home-screen on iPad, offline cache of generated recipes. |
| **frontend-design skill** (dev-time only) | Generates HTML/CSS markup in the Nordic Kitchen design system. Not a runtime dependency. |

**LLM models — selected by *role*, not by name.** Call sites request a role
(`RoleGenerate` for week generation / swap; `RoleCategorize` for ingredient
categorization and shopping-list normalization); each provider maps the role to a
concrete model:

| Role | Anthropic | OpenAI |
|------|-----------|--------|
| `RoleGenerate` | `claude-sonnet-4-6` | `gpt-5.4-mini` |
| `RoleCategorize` | `claude-haiku-4-5-20251001` | `gpt-5.4-nano` |

Switching provider never touches call sites — only the wired implementation
(`internal/llm/anthropic` or `internal/llm/openai`) changes.

---

## Commands

> Placeholders until the Go module exists. Update once `go.mod` and the Dockerfile land.

```bash
# Development (local)
go run ./cmd/server

# Build
go build -o bin/server ./cmd/server

# Test
go test ./...

# Migrations (golang-migrate)
migrate -path migrations -database "sqlite3://data/cooking.db" up

# Docker (production on Mac mini)
docker compose up -d --build
```

---

## Architecture

Layout (see [`.agents/tech-design.md`](.agents/tech-design.md) §4.4):

```
cooking-helper/
├── cmd/server/main.go     # entry point
├── internal/
│   ├── domain/            # models: Recipe, WeeklyPlan, HouseholdProfile — no infra deps
│   ├── handler/           # HTTP handlers, grouped by feature
│   ├── service/           # business logic, orchestration
│   ├── repository/        # SQL access only
│   ├── llm/               # provider-agnostic interface + anthropic/ + openai/ impls + prompts/
│   ├── i18n/              # ru/fi/en dictionaries + t() func
│   └── shopping/          # ingredient consolidation + categorization
├── migrations/            # golang-migrate files
├── templates/             # *.gohtml (html/template)
├── static/                # CSS, HTMX, Service Worker, self-hosted fonts
└── i18n/                  # ru.json, fi.json, en.json
```

### Layer rules
- **Domain** (models, business logic): no dependencies on frameworks, DB, or HTTP.
- **Service**: orchestrates domain logic, calls repository, returns domain errors.
- **Repository**: only DB access, no business logic. No SQL anywhere else.
- **Handler**: validates input, calls service, maps errors to HTTP responses / template data.

Dependency direction: **handlers → services → repositories → domain. Never reverse.**

**Wiring site**: dependencies (services, repositories, LLM client) are constructed and
wired together in `internal/handler/router.go`, **not** in `cmd/server/main.go`. Add new
routes and dependency wiring there.

### Domain-Driven Design
- Group code by domain feature (`internal/shopping/`), not by technical layer.
- Name types, functions, and packages after the domain concept, not the technology.
- Keep `sql.Row` and HTTP types out of domain structs.

---

## Validate Before Implementing

### External integrations and data sources
Never write code for an integration without completing this checklist:
1. **Data is accessible** — get a real response (curl / browser). Confirm the needed data is present.
2. **Authorization** — does it need an API key, registration, or paid plan? If yes — stop and confirm with the owner first.
3. **Still works** — verify the endpoint/version is live right now.
4. **Fields are parseable** — confirm required fields are actually in the response.

This applies directly to the **LLM provider API** — Anthropic (`ANTHROPIC_API_KEY`)
or OpenAI (`OPENAI_API_KEY`), key required, provisioned via env — and to the
**Apple Reminders / Shortcuts** export in Phase 2. Note: the web sandbox's egress
allowlist **varies by session** — a provider host may be blocked
(`x-deny-reason: host_not_allowed`) in one run and reachable in another (CH-8 ran a
live OpenAI E2E in-sandbox with `OPENAI_API_KEY` set). **Re-check reachability at run
time** (a quick probe, or the live attempt itself) before deciding to defer — do NOT
skip a verifiable live LLM test on the strength of this note alone. Only defer to a
networked host (Mac mini / dev machine) once you've confirmed the host is actually
blocked or the key is absent in *this* run.

### Third-party libraries
Before proposing a library: check it's actively maintained, compatible with the Go
version in use, and free of conflicts with existing dependencies. Default to the standard
library — this project deliberately avoids an ORM and a JS framework.

### Use agent-browser for web inspection
When inspecting page markup, finding CSS selectors, or checking whether a site renders
data without JavaScript, use the `agent-browser` skill directly. Do NOT ask the user to
save HTML manually and do NOT guess selectors. (Relevant when parsing the `recepy-examples/`
HTML or any future recipe source.)

---

## Code Patterns

### Typing
Strictly typed by default — the compiler is the first reviewer. Use explicit Go types;
avoid `interface{}` / `any` unless genuinely necessary. Model the PRD §15 data schema as
Go structs.

### Naming & errors
- Standard Go conventions: `MixedCaps`, short receiver names, exported docs start with the name.
- Wrap errors with context: `fmt.Errorf("generate week: %w", err)`. Return domain errors
  from services; never leak `sql`/HTTP details into the domain.

### LLM calls (`internal/llm`)
- All calls go through the provider-agnostic `Client` interface — no direct SDK calls in handlers/services, no provider-specific model names at call sites.
- **Select the model by `Role`** (`RoleGenerate` / `RoleCategorize`); the wired provider maps the role to a concrete model ID. The provider is chosen once, at the wiring site.
- Prompts live in version-controlled files under `internal/llm/prompts/` (e.g. `generate_week.v1.txt`).
- Use **prompt caching**: cache the stable part (household profile + disliked + pantry + recent feedback), vary only the generation trigger. Anthropic needs an explicit cache breakpoint on the System block; OpenAI caches long stable prefixes automatically.
- **Validate LLM output against the hard constraints**: disliked ingredients must be 100% excluded — post-process and regenerate (max 1 retry) on violation.

---

## Security

- **Secrets**: never hardcode tokens or keys. The LLM key (`ANTHROPIC_API_KEY` / `OPENAI_API_KEY`) is a server-side env var only — never in code, never on the client.
- **Pin the LLM base URL**: both SDKs silently honor a `*_BASE_URL` env var, which could redirect the key to an arbitrary host. Set the base URL explicitly at construction and do not read it from untrusted env.
- **Input validation**: validate and sanitize all external input at the handler boundary. Trust nothing from outside, including LLM output (it's external too).
- **Errors**: never expose internal error details, stack traces, or DB messages to the client. Log internally, return a generic message / friendly template.
- **Privacy**: do not send personal data to the LLM beyond what the feature needs (preferences, feedback, generation history). Do not log prompt contents in production — log token count and latency only.
- **Dependencies**: run `govulncheck ./...` before adding or bumping a library.
- **Network**: access is tailnet-only via Tailscale Serve. Do not enable Funnel (external exposure) without explicit owner approval.

> MVP has no auth (single household). When multi-user lands, add identity + per-resource
> authorization — the schema already carries `household_id` for this.

---

## Fault Tolerance

### External calls (LLM provider API, DB)
- **Timeouts**: set an explicit timeout (`context.WithTimeout`) on every external call. Nothing blocks indefinitely.
- **Retry with exponential backoff**: retry transient errors (network, 5xx) — 2s → 4s → 8s, max 3 attempts. Do NOT retry 4xx.
- **Invalid LLM JSON**: retry once with a clarifying hint, then fail gracefully.
- **Graceful degradation**: if the archive or a non-critical read fails, render what you can rather than 500-ing the whole page.

### Idempotency
- Feedback writes and generation triggers should be safe to retry. Service Worker queues
  feedback writes while offline and replays them — consumers must dedupe.

---

## Observability

### Structured logging (`log/slog`, JSON to stdout)
- Every entry includes timestamp, level, message, and a **`request_id`** (UUID) propagated
  across log lines and into LLM calls.
- **Propagate `request_id` via `context.Context`, not via handler-package internals.**
  The handler generates the UUID at the request boundary and stores it on the context
  through a *neutral* shared package (e.g. `internal/reqid` with `WithID(ctx, id)` /
  `FromContext(ctx)`). Services and the `internal/llm` client read it back from `ctx`.
  Do NOT leave `Request.RequestID` empty to dodge a service→handler import — that
  reverse dependency is the thing the neutral package exists to avoid; reading the
  request_id from `ctx` keeps the dependency direction correct *and* satisfies this rule.
- Log at boundaries: incoming request, outgoing LLM/DB call, error.
- Do NOT log secrets, prompt contents, or personal data.

```go
slog.Info("request received", "method", r.Method, "path", r.URL.Path, "request_id", id)
slog.Error("llm generate failed", "err", err, "request_id", id)
```

### Healthcheck
- `GET /healthz` checks the DB connection, returns `200` ready / `503` not ready.
  Tailscale Serve points its healthcheck here.

### Cost
- Log LLM token counts per call — the budget is personal use, monitor it from day one.

---

## Database

### Migrations
- **Never modify the schema manually.** All changes go through `golang-migrate` files in `migrations/`, version-controlled, applied on startup/deploy.
- Schema = the data model in PRD §15 Appendix. Every table carries a `household_id` UUID for future multi-user.

### Access
- All DB access goes through `internal/repository`. No SQL in services or handlers.
- Use transactions for multi-table atomic writes (e.g. WeeklyPlan + ShoppingList).
- Set query timeouts via context. SQLite is single-writer — keep write transactions short.

### Backup
- Daily `launchd` job on the Mac mini does `sqlite3 .backup`, retains 14 days. Backups are
  critical here because all data lives on one box.

---

## Frontend & UI Generation

UI markup is generated with the Anthropic **`frontend-design`** skill, then moved into
`templates/*.gohtml` and wired with HTMX. **Every invocation must specify:**

1. Output: **HTML + CSS only** — no React, no Vue, no JSX.
2. Design system: **Nordic Kitchen** (see [`.agents/tech-design.md`](.agents/tech-design.md) §4.5).
3. Target: iPad Safari, kitchen context, 50cm reading distance.
4. Constraints: **≥18pt body, ≥24pt headings, 44×44pt touch targets**, no hover-only interactions.

Nordic Kitchen essentials: warm cream background `#F5EFE6`, deep oak text `#2B2118`,
terracotta accent `#C2603A`; Fraunces headings + Public Sans body (self-hosted in
`static/fonts/`); respect `prefers-color-scheme` for dark mode.

### HTMX write idiom
The established pattern for state-changing controls (shopping checkboxes, recipe feedback)
is a native `<input type="checkbox" name=… value="true">` with `hx-trigger="change"` +
`hx-target="closest …"` + `hx-swap="outerHTML"`. Endpoints take **absolute** state, never
a toggle — this keeps a Service-Worker offline replay an idempotent no-op (see
Fault Tolerance § Idempotency). The codebase uses **no `hx-vals`**; use `hx-include` to
post the full control state. Match this idiom rather than introducing `<button>`+`hx-vals`.

### i18n
All UI strings go through `t(key, args...)` registered in the template `FuncMap` — no
hardcoded strings. Generated recipes keep the language they were created in; switching the
UI language does not re-translate existing recipes.

---

## Validation

Run before every commit (style checks run alongside tests):

```bash
gofmt -s -l .          # formatting (no output = clean)
go vet ./...           # vet
golangci-lint run ./...# lint  (install: GOTOOLCHAIN=go1.26.3 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)
                       #   NB: the repo pins go1.26.3 and ships a v2 .golangci.yml. A stock
                       #   `@latest` install of the v1 module path is built with an older Go and
                       #   refuses the v2 config ("Go language version ... is lower than 1.26.3").
                       #   Use the v2 module path under the pinned toolchain, as shown.
go test ./...          # tests
govulncheck ./...      # dependency vulnerabilities (before adding/bumping deps)
```

> **Web / sandbox environment constraints.** Outbound network is restricted and there is
> no Docker daemon, so some checks **cannot run here** and must not be treated as failures:
> - `govulncheck ./...` — `vuln.go.dev` returns `403`.
> - `docker build` / image pulls — no daemon; Docker Hub + CDNs (unpkg, jsDelivr) return `403`.
> - Service Worker activation — needs HTTPS (`localhost` or tailnet), unavailable in-sandbox.
>
> Run everything that *does* work (`gofmt`, `go vet`, `golangci-lint`, `go test`, static
> `CGO_ENABLED=0` build, HTTP-level E2E). For the blocked ones: vendor assets from a
> reachable pinned source (e.g. GitHub raw), then **defer-and-record** — list what was not
> verified and where it must run (networked host / Mac mini / tailnet HTTPS). These deferred
> checks are gated at deploy time by **CH-21**, not silently skipped.

---

## Key Files

| File | Purpose |
|------|---------|
| `.agents/PRDs/PRD.md` | Product requirements (v3). Source of truth for scope. |
| `.agents/tech-design.md` | Locked-in architecture + Nordic Kitchen design system. |
| `.agents/stories/stories.md` | User stories / work items (CH-*). |
| `recepy-examples/` | 39 reference Finnish recipes (HTML) — seed data for parsing/LLM tests, not runtime. |
| `cmd/server/main.go` | Entry point (once code lands). |
| `internal/llm/` | LLM abstraction — the heart of generation. |

---

## On-Demand Context

| Topic | File |
|-------|------|
| Why this stack (alternatives + trade-offs) | `.agents/tech-design.md` |
| Scope, user stories, data model | `.agents/PRDs/PRD.md` |
| Work items | `.agents/stories/stories.md` |
| Design system details | `.agents/tech-design.md` §4.5 |

---

## Notes

- **HTTPS is mandatory** for the Service Worker — that's why Tailscale Serve exists. Local `go run` over plain HTTP won't register the SW; test PWA behavior over the tailnet HTTPS URL.
- **SQLite is single-writer.** Keep write transactions short; if concurrent family edits ever become a real bottleneck, revisit Postgres (tech-design §3.3).
- **No images in MVP** — recipe cards use emoji + typography. No `<img>`, no image pipeline.
- **No npm/node in production.** The frontend-design skill is a dev-time tool only.
- The iPad **only works at home** (tailnet). Away-from-home shopping is solved by Apple Reminders export (Phase 2), not online access.
