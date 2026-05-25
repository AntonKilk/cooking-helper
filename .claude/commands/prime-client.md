---
description: Prime agent with client/frontend codebase understanding
argument-hint: [jira-issues] [confluence-pages]
---

# Prime Client: Load Frontend Context

**Input**: $ARGUMENTS

## Objective

Build comprehensive understanding of the client/frontend codebase by analyzing structure and key files.

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

1. Read `CLAUDE.md` to identify the frontend framework and source directories
2. Study the app routes or page structure
3. Study the feature/component directories
4. Study the shared UI primitives (if any)

## Output

Produce a scannable summary of what you learned:

- **Purpose**: What the UI does
- **Tech Stack**: Framework and routing (e.g. Next.js App Router, SvelteKit, or other), UI library (e.g. shadcn/ui, Material UI, or other), styling approach (e.g. Tailwind, CSS modules, or other)
- **Components**: Key components and their responsibilities
- **Data Flow**: How data gets from server to UI and how mutations are sent back (e.g. Server Actions, REST calls, GraphQL, or other)
- **Patterns**: Component split strategy (e.g. Server vs Client components, or equivalent), how forms handle state and submission

Use bullet points. Keep it concise.
