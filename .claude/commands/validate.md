---
description: Run linter, type checker, and tests - report any failures
---

# Validate

Run all validation checks for the current project and report results.

---

## Step 1: Detect Project Stack

Read `CLAUDE.md` to identify the language and tools. Then run the checks for that stack.

---

## Checks by Stack

### Go

```bash
# Format check
gofmt -l .

# Vet (built-in static analysis)
go vet ./...

# Style and lint (golangci-lint — Go equivalent of Checkstyle)
golangci-lint run ./...

# Tests
go test ./...
```

If `golangci-lint` is not installed:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### Java (Maven)

```bash
# Checkstyle
mvn checkstyle:check

# Compile
mvn compile

# Tests
mvn test
```

### Java (Gradle)

```bash
# Checkstyle
gradle checkstyleMain checkstyleTest

# Compile + tests
gradle build
```

### Python

```bash
# Lint and style
ruff check .

# Type check (if mypy configured)
mypy .

# Tests
python3 -m pytest
```

### JavaScript / TypeScript (fallback)

```bash
# Lint
pnpm run lint   # or: bun run lint / npm run lint

# Type check
pnpm run type-check   # or: bunx tsc --noEmit

# Tests
pnpm test
```

---

## Process

1. Identify stack from `CLAUDE.md`
2. Run all checks for that stack, capture output
3. Collect all failures
4. Report results

---

## Output

```
## Validation Results

| Check | Result | Details |
|-------|--------|---------|
| Style (Checkstyle / golangci-lint / ruff) | ✅/❌ | {N errors or "passed"} |
| Type / Compile check | ✅/❌ | {N errors or "passed"} |
| Tests | ✅/❌ | {N passed, M failed} |

### Summary
- **Status**: ✅ ALL PASSING / ❌ {N} FAILURES
- **Action needed**: {None / list of things to fix}
```

---

## If Failures Found

List each failure with:
1. File and line number
2. Error message
3. Suggested fix (if obvious)

Example:
```
### Failures

1. **internal/scraper/ryanair.go:42**
   - Error: `exported function Fetch should have comment`
   - Fix: Add godoc comment above the function

2. **src/service/UserService.java:78**
   - Error: `Line is longer than 120 characters`
   - Fix: Break the line or extract a variable
```
