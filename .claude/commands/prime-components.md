---
description: Learn how to build components in this codebase
argument-hint: [jira-issues] [confluence-pages]
---

# Prime Components: How to Build Components

**Input**: $ARGUMENTS

## Objective

Understand the component patterns used in this codebase so you can build new components correctly.

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

1. Read `CLAUDE.md` to find the component/UI directories and framework
2. Study the shared UI primitives (e.g. `src/components/ui/` or equivalent)
3. Study utility helpers used for styling or class composition (e.g. `cn()`, or equivalent)
4. Study 2-3 representative feature components as examples of the established pattern

## Output

Produce a scannable summary of what you learned:

- **UI Library**: Available components and how they are composed (e.g. shadcn/ui, Material UI, custom, or other)
- **Styling**: How styles are applied and how conditional classes work (e.g. Tailwind + `cn()`, CSS modules, or other)
- **Props Pattern**: How props are typed (inline types, exported interfaces, or other)
- **Component Split**: Which components are "dumb" vs stateful, server-rendered vs client-side (framework-dependent)
- **Forms**: How form state, submission, and validation are handled (e.g. useActionState + Server Actions, React Hook Form, or other)

Use bullet points. Keep it concise.
