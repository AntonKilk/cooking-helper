# Plan: CH-2 Project Skeleton & Dev Environment

## Summary

Bootstrap the Go project so an empty-but-runnable app exists and future features can be
layered on. Create the `go.mod`, the `cmd/server` entry point with a `log/slog`-instrumented
HTTP server (timeouts + graceful shutdown), a `GET /healthz` endpoint that returns `200` (DB
check is a stub until CH-3), the full `internal/` package layout from tech-design §4.4,
linter config that passes clean, a multi-stage `Dockerfile` + `docker-compose.yml`, and a
README with local + Docker run instructions. No business logic, no DB, no LLM — just the
skeleton that CH-3/CH-4/CH-6 build on.

## User Story

As a developer
I want a basic Go project skeleton with a working dev server
So that I can run an empty app locally and in Docker, then grow functionality on top.

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY |
| Complexity | LOW |
| Systems Affected | build, HTTP server, Docker, CI/validation |
| GitHub Issue | #2 |

---

## Locked-in constraints (from CLAUDE.md + tech-design §4.4)

These are non-negotiable. The plan builds on them; it does not re-open them.

- **Stack**: Go + `html/template` + HTMX + SQLite. No SPA, no npm/node in prod, no ORM.
- **Module path**: `github.com/AntonKilk/cooking-helper` (matches the git remote owner/repo).
- **Go version**: 1.24 (toolchain present: go1.24.7).
- **Layering**: handlers → services → repositories → domain. Never reverse. SQL only in
  `internal/repository`. Domain has no infra deps. Group by feature, not tech layer.
- **Logging**: `log/slog`, JSON to stdout, every log line carries a `request_id` (UUID).
- **Fault tolerance**: explicit timeouts on the HTTP server; graceful shutdown.
- **Security**: `ANTHROPIC_API_KEY` is a server-side env var only — never hardcoded, never
  on the client. Not used yet in CH-2, but compose must pass it through, not bake it in.
- **Validation commands** (must all pass clean): `gofmt -s -l .`, `go vet ./...`,
  `golangci-lint run ./...`, `go test ./...`.

---

## Patterns to Follow

> The repo is **pre-code** — there are no existing Go files to mirror. Patterns below are the
> conventions mandated by CLAUDE.md; follow them exactly so later stories stay consistent.

### Naming & package layout (CLAUDE.md "Architecture", tech-design §4.4)
```
cmd/server/main.go        # entry point only — wiring, no business logic
internal/domain/          # models, no framework/DB/HTTP deps
internal/handler/         # HTTP handlers, grouped by feature, validate input
internal/service/         # business logic / orchestration
internal/repository/      # SQL access only
internal/llm/             # provider-agnostic interface + anthropic/ impl + prompts/
internal/i18n/            # ru/fi/en dictionaries + t() func
internal/shopping/        # ingredient consolidation + categorization
```
Standard Go conventions: `MixedCaps`, short receiver names, exported decls documented
starting with the name.

### Structured logging (CLAUDE.md "Observability")
```go
// slog JSON to stdout; request_id propagated across lines.
slog.Info("request received", "method", r.Method, "path", r.URL.Path, "request_id", id)
slog.Error("server error", "err", err)
```

### Error wrapping (CLAUDE.md "Naming & errors")
```go
return fmt.Errorf("start server: %w", err)
```

### Tests (CLAUDE.md "Validation" — `go test ./...`)
Standard library `testing` + `net/http/httptest`. Table-free is fine for a single case.
```go
func TestHealth_OK(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
    rec := httptest.NewRecorder()
    handler.Health(rec, req)
    if rec.Code != http.StatusOK {
        t.Fatalf("got %d, want 200", rec.Code)
    }
}
```

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `go.mod` | CREATE | Module definition, Go 1.24, no deps yet. |
| `.gitignore` | CREATE | Ignore `bin/`, `data/`, `*.db`, local env. |
| `cmd/server/main.go` | CREATE | Entry point: slog setup, mux, HTTP server w/ timeouts, graceful shutdown. |
| `internal/handler/health.go` | CREATE | `GET /healthz` handler returning 200 (DB stub). |
| `internal/handler/health_test.go` | CREATE | Test that `/healthz` returns 200 — gives `go test` something to run. |
| `internal/handler/router.go` | CREATE | Build `*http.ServeMux`, register routes, request_id + logging middleware. |
| `internal/domain/doc.go` | CREATE | Package placeholder so the layout exists and compiles. |
| `internal/service/doc.go` | CREATE | Package placeholder. |
| `internal/repository/doc.go` | CREATE | Package placeholder. |
| `internal/llm/doc.go` | CREATE | Package placeholder. |
| `internal/i18n/doc.go` | CREATE | Package placeholder. |
| `internal/shopping/doc.go` | CREATE | Package placeholder. |
| `.golangci.yml` | CREATE | golangci-lint v2 config; must run clean. |
| `Dockerfile` | CREATE | Multi-stage build → small runtime image. |
| `docker-compose.yml` | CREATE | Single service, data volume, port + env passthrough, healthcheck. |
| `.dockerignore` | CREATE | Keep build context small. |
| `README.md` | CREATE | Run instructions: local (`go run`) and Docker (`docker compose`). |

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Initialize Go module

- **File**: `go.mod`
- **Action**: CREATE
- **Implement**: `module github.com/AntonKilk/cooking-helper` with `go 1.24`. No
  dependencies yet (CH-2 uses only the standard library). Create via
  `go mod init github.com/AntonKilk/cooking-helper`.
- **Validate**: `go build ./...` (succeeds with no packages yet is fine).

### Task 2: Add .gitignore

- **File**: `.gitignore`
- **Action**: CREATE
- **Implement**: Ignore build + local artifacts: `/bin/`, `/data/`, `*.db`, `*.db-*`
  (SQLite WAL/SHM), `*.local`, `.env`. Do NOT ignore `recepy-examples/`.
- **Validate**: `git status` shows the file, nothing important ignored.

### Task 3: Health handler

- **File**: `internal/handler/health.go`
- **Action**: CREATE
- **Implement**: `package handler`. Export `func Health(w http.ResponseWriter, r *http.Request)`.
  Write `Content-Type: application/json`, status `200`, body `{"status":"ok"}`. Add a one-line
  comment: DB readiness check is added in CH-3; this is a 200 stub. No service/repo dependency
  yet (none exists). Keep it a plain function — no premature interface.
- **Mirror**: Tests pattern above; CLAUDE.md "Observability → Healthcheck".
- **Validate**: `go vet ./internal/handler/`

### Task 4: Router + middleware

- **File**: `internal/handler/router.go`
- **Action**: CREATE
- **Implement**: `func NewRouter(logger *slog.Logger) http.Handler`. Use Go 1.22+ `http.ServeMux`
  with method patterns: `mux.HandleFunc("GET /healthz", Health)`. Wrap the mux in a middleware
  that (1) generates a `request_id` (use `crypto/rand` hex or a tiny UUID helper — stdlib only,
  no new dep), (2) logs `slog.Info("request received", "method", ..., "path", ..., "request_id", id)`,
  (3) stores the id in the request context for downstream use. Return the wrapped handler.
- **Mirror**: CLAUDE.md "Observability" logging snippet; layering (handler owns routing).
- **Validate**: `go vet ./internal/handler/`

### Task 5: Server entry point

- **File**: `cmd/server/main.go`
- **Action**: CREATE
- **Implement**: `package main`.
  - Set the default slog logger to a JSON handler writing to stdout.
  - Read `PORT` from env, default `"8080"`.
  - Build the router via `handler.NewRouter(logger)`.
  - Construct `*http.Server` with `Addr: ":"+port`, `Handler`, and explicit timeouts
    (`ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`) — never block forever.
  - Start `ListenAndServe` in a goroutine; log fatal on non-`ErrServerClosed` error.
  - Block on `signal.NotifyContext` for `SIGINT`/`SIGTERM`; on signal, `server.Shutdown` with a
    bounded `context.WithTimeout` (e.g. 10s). Log start and shutdown.
  - Wrap returned errors with `fmt.Errorf("...: %w", err)`.
- **Mirror**: CLAUDE.md "Fault Tolerance" (timeouts), "Observability" (slog).
- **Validate**: `go run ./cmd/server` starts; `curl -s localhost:8080/healthz` → `200`
  `{"status":"ok"}`; Ctrl-C shuts down cleanly.

### Task 6: Health handler test

- **File**: `internal/handler/health_test.go`
- **Action**: CREATE
- **Implement**: `package handler`. `TestHealth_OK` per the Tests pattern above — assert status
  200 and that the body contains `"ok"`. This guarantees `go test ./...` has a real assertion.
- **Mirror**: Tests pattern above.
- **Validate**: `go test ./internal/handler/`

### Task 7: Package placeholders for the layout

- **Files**: `internal/domain/doc.go`, `internal/service/doc.go`, `internal/repository/doc.go`,
  `internal/llm/doc.go`, `internal/i18n/doc.go`, `internal/shopping/doc.go`
- **Action**: CREATE
- **Implement**: Each is a single `// Package x ...` doc comment + `package x` declaration —
  just enough for the directory to exist as a real, compiling package per tech-design §4.4.
  One short line describing the package's future responsibility (e.g. domain = core models).
  Do not add types/functions yet — those land in CH-3+.
- **Mirror**: tech-design §4.4 layout.
- **Validate**: `go build ./...`

### Task 8: golangci-lint config

- **File**: `.golangci.yml`
- **Action**: CREATE
- **Implement**: golangci-lint **v2** format (`version: "2"`). Enable a sane default set
  (the v2 `standard`/default linters; explicitly include `govet`, `staticcheck`, `errcheck`,
  `ineffassign`, `unused`). Keep it minimal so the skeleton passes clean. Ensure the run targets
  `./...`.
- **Mirror**: CLAUDE.md "Validation" — `golangci-lint run ./...` must pass clean.
- **Validate**: `golangci-lint run ./...` (no findings).

### Task 9: Dockerfile

- **File**: `Dockerfile`
- **Action**: CREATE
- **Implement**: Multi-stage.
  - **Build stage**: `golang:1.24` (or `-alpine`). Copy `go.mod` (+ `go.sum` when it exists),
    `go mod download`, copy source, `CGO_ENABLED=0 go build -o /out/server ./cmd/server`.
    (CGO off keeps the binary static — see Risks re: SQLite driver in CH-3.)
  - **Run stage**: minimal base (`gcr.io/distroless/static` or `alpine`). Copy the binary,
    create/declare `/data` for the future SQLite file, `EXPOSE 8080`, non-root user,
    `ENTRYPOINT ["/server"]`.
- **Validate**: `docker build -t cooking-helper .` succeeds.

### Task 10: .dockerignore

- **File**: `.dockerignore`
- **Action**: CREATE
- **Implement**: Exclude `.git`, `.agents`, `.claude`, `recepy-examples`, `bin`, `data`,
  `*.db`, `README.md` from the build context.
- **Validate**: `docker build` context is small / build still succeeds.

### Task 11: docker-compose.yml

- **File**: `docker-compose.yml`
- **Action**: CREATE
- **Implement**: One service `cooking-helper`, `build: .`, map `8080:8080`, named volume
  mounted at `/data` for SQLite persistence, `environment:` passes `ANTHROPIC_API_KEY`
  (from host env, value NOT hardcoded) and `PORT`. Add a `healthcheck` running
  `wget`/`curl` against `http://localhost:8080/healthz` (note: distroless has no shell —
  if distroless, use Go-based healthcheck or switch run stage to alpine; decide in Task 9).
- **Validate**: `docker compose up -d --build` then `curl localhost:8080/healthz` → 200;
  `docker compose down`.

### Task 12: README

- **File**: `README.md`
- **Action**: CREATE
- **Implement**: Short. Project one-liner + link to `.agents/tech-design.md`. Prereqs (Go 1.24,
  Docker). **Run locally**: `go run ./cmd/server`, then `curl localhost:8080/healthz`.
  **Run via Docker**: `docker compose up -d --build`. Note `ANTHROPIC_API_KEY` env (not needed
  for CH-2 but documented). Validation commands block (gofmt/vet/lint/test). Note HTTPS/PWA is
  via Tailscale Serve on the host (out of scope for this story — CH-21).
- **Validate**: Renders; commands are copy-pasteable.

### Task 13: Full validation pass

- **Action**: VALIDATE
- **Implement**: Run the full validation suite below; fix anything that isn't clean.

---

## Validation

```bash
gofmt -s -l .            # formatting — no output = clean
go vet ./...             # vet
golangci-lint run ./...  # lint — no findings
go test ./...            # tests pass
go run ./cmd/server &    # starts; then:
curl -fsS localhost:8080/healthz   # → 200 {"status":"ok"}
docker compose up -d --build && curl -fsS localhost:8080/healthz && docker compose down
```

---

## Risks

| Risk | Mitigation |
|------|------------|
| SQLite driver choice affects Docker build (mattn/go-sqlite3 needs CGO → bigger image / no distroless-static). | CH-2 imports no DB driver; build with `CGO_ENABLED=0`. Recommend the pure-Go `modernc.org/sqlite` in CH-3 to keep CGO off and the static Dockerfile unchanged. Flag this in CH-3. |
| Distroless run image has no shell → compose `healthcheck` using curl/wget fails. | Either use an `alpine` run stage (has wget) or a tiny Go healthcheck binary. Decide in Task 9; keep compose healthcheck consistent with that choice. |
| Empty `internal/*` packages flagged by linter as unused. | `doc.go` with only a package clause + doc comment is valid and not flagged. Avoid declaring unused symbols. |
| golangci-lint v2 config format differs from v1 examples online. | Installed version is v2.5.0 — use `version: "2"` schema. Verify with `golangci-lint run` locally. |
| Module path mismatch breaks imports. | Use `github.com/AntonKilk/cooking-helper` exactly (matches remote); all internal imports derive from it. |

---

## Acceptance Criteria (from issue #2 / stories CH-2)

- [ ] `go run ./cmd/server` starts an HTTP server locally
- [ ] Package layout matches tech-design §4.4 (`cmd/server`, `internal/{domain,handler,service,repository,llm,i18n,shopping}`)
- [ ] `GET /healthz` returns `200` (DB-check stub)
- [ ] `gofmt -s`, `go vet`, `golangci-lint` all run clean
- [ ] `Dockerfile` + `docker-compose.yml` build and run the container
- [ ] README documents local + Docker run
- [ ] `go test ./...` passes
- [ ] Follows CLAUDE.md layering, logging, and fault-tolerance rules
