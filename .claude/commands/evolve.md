---
description: Review implementation reports and evolve the AI layer (rules, commands, stories)
argument-hint: [report-path | phase-name | blank = all unprocessed]
---

# Evolve: System Evolution from Implementation Reports

**Input**: $ARGUMENTS

## Objective

Close the outer loop. After issues are implemented, each `/implement` run drops a report
in `.agents/reports/`. This command reads those reports, separates one-off bugs from
**systemic gaps**, and proposes targeted improvements to the AI layer so the same class of
mistake does not recur.

**Core principle**: when the coding agent does something wrong, there is usually something
in the context we gave it that we can improve. We fix the *system that allowed the bug*,
not just the bug.

**Golden rule**: HUMAN IN THE LOOP. Never edit rules, commands, skills, or stories
silently — propose first, apply only after the user confirms.

---

## Phase 1: LOAD

### Select reports

| Input | Action |
|-------|--------|
| Blank | Process every report in `.agents/reports/` not already in `.agents/reports/processed/` |
| A `*-report.md` path | Process just that report |
| A phase name (e.g. `Phase 1`) | Process reports whose plan/story belongs to that phase |

```bash
ls .agents/reports/*.md 2>/dev/null
ls .agents/reports/processed/*.md 2>/dev/null   # already-evolved, skip these
```

If there are no unprocessed reports, say so and stop.

### Load the AI layer

Read these so proposals are grounded in what currently exists:
- `CLAUDE.md` — global rules, conventions, validation commands
- `.claude/commands/*.md` — the commands/skills that drove the work
- `.agents/stories/stories.md` — story definitions and acceptance criteria
- `.agents/tech-design.md` and `.agents/PRDs/PRD.md` — locked-in decisions (do NOT propose
  changes that re-open these without flagging it loudly)
- Recent history: `git log --oneline -10`

---

## Phase 2: ANALYZE

For each selected report, extract signals:

- **Deviations from plan** — where did the implementation diverge, and why?
- **Validation friction** — checks that failed, were missing, or were run with the wrong command
- **Bugs / rework** — anything that needed a second pass or a follow-up fix
- **Surprises** — assumptions that turned out wrong, missing context the agent had to guess

### Classify each signal

| Class | Meaning | Action |
|-------|---------|--------|
| **One-off** | A normal bug, specific to this change | No system change — note and move on |
| **Systemic** | A gap in the rules/process/context that will recur | Propose an AI-layer change |

A signal is systemic if you can finish the sentence: *"If `CLAUDE.md` / the command / the
story had said X, the agent would not have made this mistake."*

### Map each systemic signal to a target

The four things worth evolving (from the workshop methodology):

| Target | When it's the right fix |
|--------|------------------------|
| **Global rules** (`CLAUDE.md`) | A convention was violated, or a constraint was implicit and should be explicit |
| **Commands / skills** (`.claude/commands/*.md`) | The *process* let the gap through (missing validation step, unclear instruction) |
| **On-demand context** (`tech-design.md`, refs) | The agent lacked durable reference material it needed |
| **Story / plan templates** | Acceptance criteria were ambiguous or missed a recurring concern |

---

## Phase 3: PROPOSE

Present findings to the user **before changing anything**:

```markdown
## Evolution Proposal — {report name(s)}

### One-off issues (no system change)
- {bug}: {why it's not systemic}

### Systemic gaps → proposed changes
| # | Signal (from report) | Target file | Proposed change |
|---|----------------------|-------------|-----------------|
| 1 | {what went wrong} | `CLAUDE.md` | {specific edit, quoted} |
| 2 | {what went wrong} | `.claude/commands/implement.md` | {specific edit} |

### Out of scope / needs a decision
- {anything that would touch PRD/tech-design locked-in decisions — flag, do not auto-apply}
```

Use `AskUserQuestion` if any proposed change is ambiguous or architecturally significant.
Keep proposals minimal and specific — do not over-engineer or add rules for hypothetical
problems. One concrete recurring problem → one tight change.

---

## Phase 4: APPLY (after confirmation)

For each approved change:
1. Make the edit (`CLAUDE.md`, command file, story, etc.).
2. Keep edits surgical — match the existing style; do not rewrite whole files.

Then mark the reports as processed so they are not re-evaluated next run:

```bash
mkdir -p .agents/reports/processed
git mv .agents/reports/{name}-report.md .agents/reports/processed/
```

Commit the AI-layer changes with a message that explains the *why* (the recurring problem),
not just the *what*. Treat these like code: they can be PR-reviewed so the whole team stays
in sync.

---

## Phase 5: OUTPUT

```markdown
## Evolution Complete

**Reports processed**: {N} (moved to `.agents/reports/processed/`)

### Applied changes
- `CLAUDE.md`: {summary}
- `.claude/commands/{x}.md`: {summary}

### Deferred (need owner decision)
- {item, or "none"}

### Next
- Changes committed{, pushed to <branch>}.
- Re-run the PIV loop for the next phase with the improved AI layer.
```

---

## Notes

- This is the **outer loop**. If a phase went perfectly, there may be nothing to change —
  that is a valid outcome. Don't manufacture changes.
- Never delete a report; move it to `processed/` so the evolution history is auditable.
- Respect locked-in decisions: changes to `PRD.md` / `tech-design.md` are not part of this
  command — surface them as a decision for the owner instead.
