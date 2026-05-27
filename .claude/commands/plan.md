---
description: Create implementation plan with codebase analysis
argument-hint: <feature description | path/to/prd.md>
---

# Implementation Plan Generator

**Input**: $ARGUMENTS

## Objective

Transform the input into a battle-tested implementation plan through codebase exploration and pattern extraction.

**Core Principle**: PLAN ONLY - no code written. Create a context-rich document that enables one-pass implementation.

**Order**: CODEBASE FIRST. Solutions must fit existing patterns.

---

## Phase 1: PARSE

### Determine Input Type

| Input | Action |
|-------|--------|
| `#N` (GitHub issue number) | Fetch issue with `mcp__github__issue_read` (owner/repo from `git remote get-url origin`). Use title + body as feature description. Store issue number in Metadata. |
| `.prd.md` file | Read PRD, extract next pending phase |
| Other `.md` file | Read and extract feature description |
| Free-form text | Use directly as feature input |
| Blank | Use conversation context |

### Extract Feature Understanding

- **Problem**: What are we solving?
- **User Story**: As a [user], I want to [action], so that [benefit]
- **Type**: NEW_CAPABILITY / ENHANCEMENT / REFACTOR / BUG_FIX
- **Complexity**: LOW / MEDIUM / HIGH
- **GitHub Issue**: If a GitHub issue number (e.g., `#5`) is available in the conversation context — from a prior `/prime` command, user mention, or PRD — capture it. This is optional but should be included in the plan metadata when available so that `/implement` can update the issue after completion.

---

## Phase 2: EXPLORE

### Read CLAUDE.md First

Before touching the codebase, read `CLAUDE.md` and extract:

- **Validation commands** — exact commands for lint, type check, tests (use these in every Task's Validate step and in the Validation section)
- **Architecture rules** — layer structure, dependency direction, DDD grouping
- **Security requirements** — input validation boundaries, auth rules, secret handling
- **Fault tolerance requirements** — timeouts, retry policy, idempotency expectations
- **Database rules** — migration tool, repository pattern constraints

These rules are non-negotiable constraints for the plan. Every task must respect them.

If `CLAUDE.md` links to a tech-design or architecture doc (e.g. `.agents/tech-design.md`),
read it too: it holds the locked-in stack decisions and their rationale. **Do not propose a
different language, framework, datastore, or hosting approach than what these docs lock in** —
the plan builds on those decisions, it does not re-open them.

### Study the Codebase

Use the Explore agent to find:

1. **Similar implementations** - analogous features with file:line references
2. **Naming conventions** - actual examples from the codebase
3. **Error handling patterns** - how errors are created and handled
4. **Type definitions** - relevant interfaces and types
5. **Test patterns** - test file structure and assertion styles

### Document Patterns

| Category | File:Lines | Pattern |
|----------|------------|---------|
| NAMING | `path/to/file.ts:10-15` | {pattern description} |
| ERRORS | `path/to/file.ts:20-30` | {pattern description} |
| TYPES | `path/to/file.ts:1-10` | {pattern description} |
| TESTS | `path/to/test.ts:1-25` | {pattern description} |

---

## Phase 3: DESIGN

### Map the Changes

- What files need to be created?
- What files need to be modified?
- What's the dependency order?

### Identify Risks

| Risk | Mitigation |
|------|------------|
| {potential issue} | {how to handle} |

### Environment & Verification

Decide **up front** how each validation / E2E step will be verified — don't let the
implementer discover mid-task that a check can't run. For every check the feature needs,
mark whether it runs in the execution environment (see `CLAUDE.md` › Validation › *Web /
sandbox environment constraints*). For any that can't (`govulncheck`, `docker build`,
Service-Worker-over-HTTPS, or any networked step), name the fallback environment and the
gate where it gets verified (typically **CH-21** at deploy).

| Verification | Runs in env? | If blocked: where/when verified |
|--------------|--------------|---------------------------------|
| {check / E2E step} | yes / no | {networked host / Mac mini / tailnet HTTPS / CH-21} |

---

## Phase 4: GENERATE

### Create Plan File

**Output path**: `.agents/plans/{kebab-case-name}.plan.md`

```bash
mkdir -p .agents/plans
```

```markdown
# Plan: {Feature Name}

## Summary

{One paragraph: What we're building and approach}

## User Story

As a {user type}
I want to {action}
So that {benefit}

## Metadata

| Field | Value |
|-------|-------|
| Type | {type} |
| Complexity | {LOW/MEDIUM/HIGH} |
| Systems Affected | {list} |
| GitHub Issue | {issue number if available, e.g. #5, or "N/A"} |

---

## Patterns to Follow

### Naming
```
// SOURCE: {file:lines}
{actual code snippet}
```

### Error Handling
```
// SOURCE: {file:lines}
{actual code snippet}
```

### Tests
```
// SOURCE: {file:lines}
{actual code snippet}
```

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `path/to/file.ts` | CREATE | {why} |
| `path/to/other.ts` | UPDATE | {why} |

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: {Description}

- **File**: `path/to/file.ts`
- **Action**: CREATE / UPDATE
- **Implement**: {what to do}
- **Mirror**: `path/to/example.ts:lines` - follow this pattern
- **Validate**: `pnpm run build`

### Task 2: {Description}

- **File**: `path/to/file.ts`
- **Action**: CREATE / UPDATE
- **Implement**: {what to do}
- **Mirror**: `path/to/example.ts:lines`
- **Validate**: `pnpm run build`

{Continue for each task...}

---

## Validation

```bash
# Use the exact commands from CLAUDE.md — do not assume pnpm/npm/go/mvn
{lint command from CLAUDE.md}
{type check / build command from CLAUDE.md}
{test command from CLAUDE.md}
```

---

## Acceptance Criteria

- [ ] All tasks completed
- [ ] Type check passes
- [ ] Tests pass
- [ ] Follows existing patterns
- [ ] Environment-blocked verifications (if any) recorded with their CH-21 / networked-host gate
```

---

## Phase 5: OUTPUT

```markdown
## Plan Created

**File**: `.agents/plans/{name}.plan.md`

**Summary**: {2-3 sentence overview}

**Scope**:
- {N} files to CREATE
- {M} files to UPDATE
- {K} total tasks

**Key Patterns**:
- {Pattern 1 with file:line}
- {Pattern 2 with file:line}

**Next Step**: Review the plan, then implement tasks in order.
```
