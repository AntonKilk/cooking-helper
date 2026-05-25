---
description: Prime agent with codebase understanding
argument-hint: [github-issue-numbers]
---

# Prime: Load Project Context

**Input**: $ARGUMENTS

## Objective

Build comprehensive understanding of this codebase by analyzing structure and key files.

## Process

### Step 0: Load External Context (if provided)

The argument is an optional GitHub issue number or comma-separated list of numbers (e.g., `5` or `5,6,7`).

If GitHub issue numbers are provided:
1. For each issue number, call `mcp__github__issue_read` with the owner/repo from the project's GitHub config to fetch the issue title, description, acceptance criteria, and relevant context
2. Use this context to inform your understanding of what work is expected

### Step 1: Analyze the Codebase

1. Read `CLAUDE.md` for project conventions, stack, and directory structure
2. Identify the main source directories from `CLAUDE.md` and explore them
3. Check recent commits with `git log --oneline -5`

## Output

Produce a scannable summary of what you learned:

- **Project Purpose**: One sentence
- **Tech Stack**
  - Language/runtime (e.g. Go, Python, Java, Node)
  - Frameworks and key libraries
  - Database (if any)
  - Validation and error handling approach
- **Data Model**: Core entities and their relationships
- **Key Patterns**: How data flows through the system, naming conventions, error handling
- **Current State**: Recent commits, current branch

Use bullet points. Keep it concise.
