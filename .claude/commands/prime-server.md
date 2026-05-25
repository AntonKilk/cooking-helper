---
description: Prime agent with server/backend codebase understanding
argument-hint: [jira-issues] [confluence-pages]
---

# Prime Server: Load Backend Context

**Input**: $ARGUMENTS

## Objective

Build comprehensive understanding of the server/backend codebase by analyzing structure and key files.

## Process

### Step 0: Load External Context (if provided)

The first argument is an optional Jira issue key or comma-separated list (e.g., `RH-5` or `RH-5,RH-6`). The second argument is an optional Confluence page ID or comma-separated list.

If Jira issues are provided:
1. Call `mcp__atlassian__getAccessibleAtlassianResources` to get the `cloudId`
2. For each issue key, call `mcp__atlassian__getJiraIssue` with `responseContentFormat: "markdown"`
3. Use this context to inform your understanding of what work is expected

If Confluence page IDs are provided:
1. Call `mcp__atlassian__getAccessibleAtlassianResources` to get the `cloudId` (skip if already retrieved)
2. For each page ID, call `mcp__atlassian__getConfluencePage` with `contentFormat: "markdown"`

### Step 1: Analyze the Codebase

1. Read `CLAUDE.md` to identify the backend stack and source directories
2. Study the main feature/domain slice — models, validation schemas, repository/data layer, service/business logic
3. Study the database setup — schema, client, migrations (if applicable)
4. Study shared utilities and error handling patterns

## Output

Produce a scannable summary of what you learned:

- **Purpose**: What the data layer does
- **Tech Stack**: Language, framework, ORM or DB driver (e.g. Drizzle, SQLAlchemy, GORM, or other), validation library (e.g. Zod, Pydantic, Jakarta Bean Validation, or other), logging
- **Data Model**: Core tables/entities and their relationships
- **Patterns**: How the layers are organized (e.g. models → repository → service → handler), error classes and HTTP status codes
- **Mutations**: How writes flow from entry point through service to data layer

Use bullet points. Keep it concise.
