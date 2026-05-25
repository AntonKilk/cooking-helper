---
description: Learn how to build new API endpoints end-to-end
argument-hint: [jira-issues] [confluence-pages]
---

# Prime Endpoint: How to Build New Endpoints

**Input**: $ARGUMENTS

## Objective

Understand the full endpoint pattern from database to entry point so you can build new endpoints correctly.

## Process

### Step 0: Load External Context (if provided)

The first argument is an optional Jira issue key or comma-separated list (e.g., `RH-5` or `RH-5,RH-6`). The second argument is an optional Confluence page ID or comma-separated list.

If Jira issues are provided:
1. Call `mcp__atlassian__getAccessibleAtlassianResources` to get the `cloudId`
2. For each issue key, call `mcp__atlassian__getJiraIssue` with `responseContentFormat: "markdown"`

If Confluence page IDs are provided:
1. Call `mcp__atlassian__getAccessibleAtlassianResources` to get the `cloudId` (skip if already retrieved)
2. For each page ID, call `mcp__atlassian__getConfluencePage` with `contentFormat: "markdown"`

### Step 1: Analyze the Codebase

1. Read `CLAUDE.md` to identify the backend architecture and source directories
2. Find an existing feature slice and study each layer in order:
   - **Models/Types** — how domain types are defined (e.g. inferred from DB schema, hand-written structs, or other)
   - **Validation schemas** — where and how inputs are validated (e.g. Zod in `schemas.ts`, Pydantic models, Jakarta Bean Validation, or other)
   - **Repository/Data layer** — how DB queries are written (e.g. Drizzle, SQLAlchemy, GORM, raw SQL, or other)
   - **Service/Business logic** — how business rules are applied and errors are thrown
   - **Entry point** — how the service is exposed (e.g. REST handler, Server Action, gRPC handler, or other)

## Output

Produce a scannable summary of what you learned:

- **Type Flow**: How types move from DB schema through service to entry point
- **Validation**: Where and how inputs are validated (e.g. Zod schemas in service layer, or other validation framework/layer)
- **Service Pattern**: How the service calls the data layer, catches errors, and throws domain errors
- **Entry Point Pattern**: How the entry point validates input, calls the service, catches domain errors, and returns a response
- **Component/Client Pattern** (if applicable): How the frontend calls the entry point and handles state

Use bullet points. Keep it concise.
