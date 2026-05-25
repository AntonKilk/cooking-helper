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

Style checks run as part of every pre-commit validation alongside tests.
