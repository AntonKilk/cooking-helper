---
description: Create global rules (CLAUDE.md) from codebase analysis
---

# Create Global Rules

Generate a CLAUDE.md file by analyzing the codebase and extracting patterns.

---

## Objective

Create project-specific global rules that give Claude context about:
- What this project is
- Technologies used
- How the code is organized
- Patterns and conventions to follow
- How to build, test, and validate

---

## Phase 1: DISCOVER

### Identify Project Type

First, determine what kind of project this is:

| Type | Indicators |
|------|------------|
| Web App (Full-stack) | Separate client/server dirs, API routes |
| Web App (Frontend) | React/Vue/Svelte, no server code |
| API/Backend | Express/Fastify/etc, no frontend |
| Library/Package | `main`/`exports` in package.json, publishable |
| CLI Tool | `bin` in package.json, command-line interface |
| Monorepo | Multiple packages, workspaces config |
| Script/Automation | Standalone scripts, task-focused |

### Analyze Configuration

Look at root configuration files:

```
package.json       → dependencies, scripts, type
tsconfig.json      → TypeScript settings
vite.config.*      → Build tool
*.config.js/ts     → Various tool configs
```

### Map Directory Structure

Explore the codebase to understand organization:
- Where does source code live?
- Where are tests?
- Any shared code?
- Configuration locations?

---

## Phase 2: ANALYZE

### Extract Tech Stack

From package.json and config files, identify:
- Runtime/Language (Node, Bun, Deno, browser)
- Framework(s)
- Database (if any)
- Testing tools
- Build tools
- Linting/formatting

### Identify Patterns

Study existing code for:
- **Naming**: How are files, functions, classes named?
- **Structure**: How is code organized within files?
- **Errors**: How are errors created and handled?
- **Types**: How are types/interfaces defined?
- **Tests**: How are tests structured?

### Find Key Files

Identify files that are important to understand:
- Entry points
- Configuration
- Core business logic
- Shared utilities
- Type definitions

---

## Phase 3: GENERATE

### Create CLAUDE.md

Use the template at `.claude/CLAUDE-template.md` as a starting point.

**Output path**: `CLAUDE.md` (project root)

**Adapt to the project:**
- Remove sections that don't apply
- Add sections specific to this project type
- Keep it concise - focus on what's useful

**Key sections to include:**

1. **Project Overview** - What is this and what does it do?
2. **Tech Stack** - What technologies are used?
3. **Commands** - How to dev, build, test, lint?
4. **Structure** - How is the code organized?
5. **Patterns** - What conventions should be followed?
6. **Key Files** - What files are important to know?

**Optional sections (add if relevant):**
- Architecture (for complex apps)
- API endpoints (for backends)
- Component patterns (for frontends)
- Database patterns (if using a DB)
- On-demand context references

---

## Phase 4: OUTPUT

```markdown
## Global Rules Created

**File**: `CLAUDE.md`

### Project Type

{Detected project type}

### Tech Stack Summary

{Key technologies detected}

### Structure

{Brief structure overview}

### Next Steps

1. Review the generated `CLAUDE.md`
2. Add any project-specific notes
3. Remove any sections that don't apply
4. Optionally create reference docs for deeper context
```

---

## Mandatory Rules to Always Include

When generating CLAUDE.md, always add the following section regardless of project type:

### Validate Before Implementing

Add this as a top-level section in the generated CLAUDE.md:

```markdown
## Validate Before Implementing

### External integrations and data sources
Never write code for an integration without completing this checklist:
1. **Data is accessible** — get a real response (curl / browser / Postman). Confirm the needed data is present without extra steps.
2. **Authorization** — does it require an API key, registration, B2B agreement, or paid plan? If yes — stop and confirm with the owner before writing any code.
3. **Still works** — verify the endpoint/version is live right now. Unofficial APIs and versioned endpoints disappear without warning.
4. **Fields are parseable** — confirm that the required fields (price, date, ID, etc.) are actually in the response and can be extracted.

### Third-party libraries
Before proposing a library:
- Check it is actively maintained (last commit date, open issues)
- Verify compatibility with the runtime version in use
- Check for conflicts with existing dependencies

### Use agent-browser for web inspection
When inspecting page markup, finding CSS selectors, or checking whether a site
renders data without JavaScript — use the `agent-browser` skill directly.
Do NOT ask the user to save HTML manually and do NOT guess selectors.

Triggers for agent-browser:
- "I need to see the markup of this page"
- Building a scraper for a new site
- Verifying that data exists in static HTML vs JS-rendered
- Finding the correct CSS selector for a parser
```

---

## Tips

- Keep CLAUDE.md focused and scannable
- Don't duplicate information that's in other docs (link instead)
- Focus on patterns and conventions, not exhaustive documentation
- Update it as the project evolves

---

## Owner Preferences

These preferences apply to all projects for this owner and should be reflected
in generated CLAUDE.md files where relevant.

### Language stack

**Default choice: Go** for mini-projects (bots, scrapers, CLIs, small APIs).
**Java** only if the project explicitly requires enterprise ecosystem (Spring, Hibernate, complex DI).

Rationale:
- Go compiles in seconds, deploys as a single binary, has native concurrency — fits bot/scraper/automation projects well
- Java is familiar to the owner but adds boilerplate and JVM overhead that rarely pays off at small scale

### Typing

Always prefer **strictly typed languages**. In vibe coding, the compiler is the
first reviewer — type errors catch AI-generated mistakes before runtime.
- Go: use explicit types, avoid `interface{}` / `any` unless necessary
- Java: use generics, avoid raw types
- Python (if used): always add type hints and run `mypy`

### Style checking

Always include a style/lint check in the project's `CLAUDE.md` validation section:
- Go: `golangci-lint run ./...` (install: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`)
- Java: `mvn checkstyle:check` (Maven) or `gradle checkstyleMain` (Gradle)
- Python: `ruff check .`
- JS/TS: project-specific lint script

---

## Architecture and Quality Standards

When generating CLAUDE.md, include the relevant sections below based on project type.
For any project with external I/O, HTTP calls, or a database — all sections apply.

### Clean Architecture

Add to generated CLAUDE.md:

```markdown
## Architecture

### Layer rules
- **Domain layer** (models, business logic): no dependencies on frameworks, DB, or HTTP
- **Service layer**: orchestrates domain logic, calls repository, throws domain errors
- **Repository/data layer**: only DB access, no business logic
- **Handler/entry point**: validates input, calls service, maps errors to responses

Dependency direction: handlers → services → repositories → domain. Never reverse.

### Domain-Driven Design
- Name types, functions, and packages after the domain concept, not the technology
- Group code by domain feature (e.g. `internal/booking/`), not by layer (e.g. `internal/handlers/`)
- Keep domain logic free of infrastructure concerns (no `sql.Row` in a domain struct)
```

---

### Security

Add to generated CLAUDE.md:

```markdown
## Security

- **Secrets**: never hardcode tokens, passwords, or API keys. Use environment variables.
- **Input validation**: validate and sanitize all external input at the boundary (handler/entry point). Trust nothing from outside.
- **Authentication**: every non-public endpoint must verify identity before processing.
- **Authorization**: verify the caller has permission for the specific resource, not just that they are authenticated.
- **Errors**: never expose internal error details, stack traces, or DB messages to the caller. Log internally, return a generic message.
- **Dependencies**: check for known vulnerabilities before adding a library (`go install golang.org/x/vuln/cmd/govulncheck@latest` for Go, `mvn dependency-check:check` for Java).
```

---

### Fault Tolerance

Add to generated CLAUDE.md:

```markdown
## Fault Tolerance

### External calls (HTTP, DB, message brokers)
- **Timeouts**: always set an explicit timeout on every external call. No call should block indefinitely.
- **Retry with exponential backoff**: retry on transient errors (network, 5xx). Do NOT retry on 4xx — those are caller errors.
  - Delays: 2s → 4s → 8s (3 attempts max as a default)
- **Circuit Breaker**: if a dependency fails repeatedly, stop calling it for a cooldown period rather than hammering it.
  - Use when: calling external APIs, third-party services, or slow downstream systems.
- **Graceful degradation**: if a non-critical dependency fails, continue with reduced functionality rather than crashing.

### Idempotency
- **Mutating operations** (POST, PUT, payment, send message): design to be safe to retry.
  - Use an idempotency key (client-supplied or derived from content) to detect and deduplicate duplicate requests.
- **Message consumers**: an event may be delivered more than once. Consumers must handle duplicates safely (check if already processed before acting).

### Rate limiting
- Protect endpoints that call expensive resources or external APIs with a rate limit.
- Return `429 Too Many Requests` when limit exceeded, include `Retry-After` header.
```

---

### Observability

Add to generated CLAUDE.md:

```markdown
## Observability

### Structured logging
- Log in a machine-readable format (JSON preferred in production).
- Every log entry must include: timestamp, level, message, and a **correlation/request ID** to trace a request across log lines.
- Log at boundaries: incoming request, outgoing external call, error.
- Do NOT log sensitive data (tokens, passwords, personal data).
- **Go**: use `log/slog` (built-in since Go 1.21) — no external library needed.
  ```go
  slog.Info("request received", "method", r.Method, "path", r.URL.Path, "request_id", id)
  slog.Error("db query failed", "err", err, "request_id", id)
  ```
- **Java**: use SLF4J + Logback or Log4j2 with JSON encoder.
- **Python**: use `structlog` or `logging` with a JSON formatter.

### Healthcheck
- Expose a `GET /health` (or `/healthz`) endpoint.
- Returns `200 OK` when the service is ready to handle traffic.
- Checks critical dependencies (DB reachable, etc.) and returns `503` if not.

### Key metrics to track
- Request latency (p50, p95, p99)
- Error rate
- External dependency latency and error rate
```

---

### Database

Add to generated CLAUDE.md:

```markdown
## Database

### Migrations
- **Never modify the schema manually**. All schema changes go through migration files.
- Use a migration tool appropriate for the stack:
  - Go: `golang-migrate` or `goose`
  - Java: Liquibase or Flyway
  - Python: Alembic
- Migration files are version-controlled and run automatically on startup or deploy.

### Access patterns
- All DB access goes through the repository layer. No SQL in service or handler code.
- Use transactions for operations that modify multiple tables atomically.
- Set query timeouts to avoid long-running queries blocking the DB.
```

Style checks run as part of every pre-commit validation alongside tests.
