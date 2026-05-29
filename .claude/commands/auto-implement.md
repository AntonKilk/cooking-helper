---
description: Orchestrate parallel issue implementation — build a dependency graph from open issues, then run prime→plan→implement across independent issues concurrently
argument-hint: <start-issue-number> [max-parallel]
---

# Auto-Implement: Parallel Issue Orchestrator

**Input**: $ARGUMENTS

## What this command does

Starting from a given GitHub issue number, this command implements **every open issue
with a number ≥ start**, automatically, in **dependency order**. Issues that do **not**
depend on each other run **in parallel** in isolated git worktrees (separate threads).
Each issue goes through the project's standard pipeline: **`/prime` → `/plan` →
`/implement`**.

You (the model running this command) are the **orchestrator**. You do not write feature
code yourself — you build the schedule, launch one background sub-agent per issue, watch
them complete, and dispatch newly-unblocked issues as slots free up.

### Locked-in behaviour (decided with the owner — do not re-litigate)

| Decision | Value |
|----------|-------|
| Git output per issue | One branch per issue `claude/ch-{N}-{slug}`, pushed. **No PR** (the owner opens PRs manually). |
| Max parallel threads | Default **5** (overridable by the 2nd argument). |
| When a sub-agent has a genuine question for the owner | **Pause only that issue**; other in-flight issues keep running. Surface the question to the owner via `AskUserQuestion`, then resume that issue with the answer. |
| Issue queue & stop boundary | All **open** issues with number **≥ start**, ordered by the `Blocked by` / `Blocks` dependency graph. Run until the queue is empty. |

---

## Phase 0: PARSE INPUT

1. **Start issue** = first token of `$ARGUMENTS` (e.g. `16`). Required. If absent, stop and
   ask the owner for a starting issue number.
2. **Max parallel** = second token if present, else **5**. Clamp to `1..5`.
3. Resolve `owner`/`repo` from `git remote get-url origin` (here: `antonkilk/cooking-helper`).

---

## Phase 1: BUILD THE DEPENDENCY GRAPH

1. Call `mcp__github__list_issues` with `state: OPEN`, `perPage: 100` (paginate if needed).
2. **Filter** to issues whose number ≥ start. This is the **work set**.
3. For each issue in the work set, parse its body for the dependency block:
   - `**Blocked by:** CH-8 (#8, ✅ done), CH-12 (#12)` → this issue depends on #8 and #12.
   - `**Blocks:** CH-17` → reverse edge (informational; the real constraint is `Blocked by`).
   - Extract issue numbers from the `#N` references. Also map `CH-N` → issue number when
     the `#N` is missing (issue titles are `[CH-N] ...`, so build a `CH-N → number` map
     from the work set + any referenced closed issues).
4. A dependency is **satisfied** when the blocker issue is **closed** (done) OR **not in
   the work set and already closed on GitHub**. Verify ambiguous blockers with
   `mcp__github__issue_read` (`method: get`) and check `state`.
5. **Detect cycles.** If the graph has a cycle, stop and report the cycle to the owner —
   do not guess an order.

Produce a short table the owner can scan before execution begins:

```
Work set (N issues, start = #{start}):
  #16 CH-16 Recipe feedback        deps: #8 ✅            → READY
  #17 CH-17 Feedback integration   deps: #16             → blocked by #16
  #19 CH-19 Onboarding             deps: #5 ✅, #14 ✅     → READY
  ...
Max parallel: 5
```

---

## Phase 2: SCHEDULE

Maintain four sets:

- **READY** — all dependencies satisfied, not yet dispatched.
- **RUNNING** — dispatched, sub-agent in flight (cap = max parallel).
- **PAUSED** — waiting on an owner answer (does NOT count against the parallel cap).
- **DONE** / **FAILED**.

### Hotspot serialization (conflict guard)

Even when two issues are dependency-independent, they may edit the **same hotspot file**
and collide at merge time. Treat these as shared hotspots for this repo and **avoid running
two issues that both touch the same hotspot concurrently** (serialize them — pick one, defer
the other to READY):

- `internal/handler/router.go` (the wiring site — almost every feature touches it)
- `migrations/*` (migration sequence numbers must not clash)
- `i18n/{ru,fi,en}.json` and `internal/i18n/*` (shared dictionaries)

You cannot know the exact files before planning. Apply the guard with the information you
have: an issue's title, Acceptance Criteria, and Technical Notes usually reveal whether it
adds a route/handler (→ `router.go`), a table (→ `migrations/`), or UI strings (→ i18n).
When in doubt, prefer serializing the suspected-overlapping pair over a merge conflict.
This is a heuristic, not a hard gate — the per-issue branches mean any residual conflict is
resolved by the owner at PR-merge time, not lost.

---

## Phase 3: DISPATCH (one background sub-agent per issue)

For each READY issue while `len(RUNNING) < maxParallel`, launch a sub-agent with the
**`Agent` tool**:

- `subagent_type`: `general-purpose`
- `isolation`: `"worktree"` — each issue gets its own checkout so parallel threads never
  stomp each other's working tree.
- `run_in_background`: `true` — so the orchestrator keeps scheduling while it runs and is
  notified on completion.
- `description`: e.g. `Implement #16 CH-16`.

Use the **prompt below** (fill in the placeholders). Move the issue from READY → RUNNING.

### Sub-agent prompt template

```
You are implementing ONE GitHub issue end-to-end in an isolated git worktree, following
this repository's established pipeline. Repo: antonkilk/cooking-helper.

ISSUE: #{N} ({CH-id} — {title})

PIPELINE — follow the repo's own command files exactly (read them, then do what they say):
  1. PRIME:     Read and follow `.claude/commands/prime.md` with argument "{N}".
                (This reads CLAUDE.md + the issue via mcp__github__issue_read and loads context.)
  2. PLAN:      Read and follow `.claude/commands/plan.md` with argument "#{N}".
                Output goes to `.agents/plans/{kebab-name}.plan.md`. Put "#{N}" in the
                plan's Metadata › GitHub Issue field.
  3. IMPLEMENT: Read and follow `.claude/commands/implement.md` with that plan path.
                Honor every rule in CLAUDE.md (validation commands, layer boundaries,
                security, fault tolerance). Run the full validation suite that works in
                this sandbox (gofmt -s -l ., go vet ./..., golangci-lint run ./...,
                go test ./...). Defer-and-record sandbox-blocked checks per CLAUDE.md
                (govulncheck, docker build, Service-Worker-over-HTTPS) — do NOT treat them
                as failures.

GIT — STRICT:
  - Work on branch `claude/ch-{N}-{slug}` (create from the default branch in your worktree).
  - Commit with a clear message referencing the issue, e.g. "CH-{N}: <summary> (#{N})".
  - End the commit body with: https://claude.ai/code/session_019C9KuKQU6JGDGGevd4nwVW
  - Push with: git push -u origin claude/ch-{N}-{slug}
    (retry on network error only: 2s,4s,8s,16s backoff). Pushing persists your work —
    the container is ephemeral, so an unpushed branch is lost.
  - Do NOT open a pull request. Do NOT push to any other branch.

GITHUB ISSUE: implement.md Phase 6 will add an implementation comment and close #{N}.
That is expected and desired (it marks the issue done for the orchestrator).

IF YOU HIT A QUESTION ONLY THE OWNER CAN ANSWER:
  Do NOT guess and do NOT silently pick a default for a genuine product/architecture
  decision. STOP and return immediately with a result of exactly this shape:

    STATUS: NEEDS_INPUT
    QUESTION: <the single clear question, with enough context to answer without scrollback>
    OPTIONS: <2-4 candidate answers if applicable, else "free-form">
    WORK_SO_FAR: <branch name; what's committed/pushed; where you stopped>

  (Reserve this for real blockers — missing product decisions, ambiguous acceptance
  criteria, an external dependency that needs a key/authorization. Routine implementation
  choices you make yourself, following CLAUDE.md.)

ON SUCCESS, return a result of exactly this shape:
    STATUS: DONE
    BRANCH: claude/ch-{N}-{slug}  (pushed: yes/no)
    REPORT: .agents/reports/{name}-report.md
    VALIDATION: gofmt/vet/lint/test results (and any deferred-and-recorded checks)
    FILES: created/updated counts
    DEVIATIONS: from plan, or "none"
    ISSUE: "#{N} closed" or why not

ON UNRECOVERABLE FAILURE (validation can't be made green, etc.), return:
    STATUS: FAILED
    BRANCH: claude/ch-{N}-{slug}  (pushed: yes/no — push partial work anyway)
    WHERE: which task/validation failed and the error
    DIAGNOSIS: what's wrong and what you tried
```

---

## Phase 4: MONITOR & RESCHEDULE (the orchestration loop)

You will be notified as each background sub-agent completes. **Do not poll with
`sleep`** — react to completion notifications. On each completion:

1. **Parse the returned STATUS.**

2. **`DONE`** → move issue RUNNING → DONE. Re-evaluate the graph: any issue whose
   `Blocked by` set is now fully satisfied (this issue closed) moves into READY. Then go
   to Phase 3 and dispatch from READY up to the parallel cap (respecting the hotspot guard).

3. **`NEEDS_INPUT`** → move issue RUNNING → PAUSED (this frees a parallel slot, so
   immediately dispatch the next READY issue so other threads keep working). Surface the
   sub-agent's QUESTION to the owner with **`AskUserQuestion`** — pass the question and its
   OPTIONS verbatim, adding enough context that the owner can answer without scrolling back.
   When the owner answers, **resume** that issue: launch a fresh sub-agent (same worktree
   branch `claude/ch-{N}-{slug}`) whose prompt includes the original issue, the WORK_SO_FAR,
   and the OWNER'S ANSWER so it continues from where it stopped. Move PAUSED → RUNNING.

4. **`FAILED`** → move issue RUNNING → FAILED. Mark every issue that was `Blocked by` it as
   **BLOCKED-BY-FAILURE** (cannot run). Keep going with everything still runnable — one
   failure must not halt independent threads. Report the failure in the final summary; do
   not retry blindly more than once.

5. **Hotspot freed** → if you serialized an issue behind a hotspot and the holder is now
   DONE, the deferred issue becomes eligible again.

Continue until READY, RUNNING and PAUSED are all empty.

### Batching rule

When you dispatch multiple READY issues at once, launch them **in a single message with
multiple `Agent` tool calls** so they actually start concurrently.

---

## Phase 5: FINAL SUMMARY

When the queue is drained, report to the owner:

```
## Auto-Implement Complete

Start: #{start} · Max parallel: {n} · Work set: {N} issues

| Issue | CH | Branch | Status | Report |
|-------|----|--------|--------|--------|
| #16 | CH-16 | claude/ch-16-recipe-feedback | ✅ DONE (pushed, #16 closed) | .agents/reports/...-report.md |
| #17 | CH-17 | claude/ch-17-...             | ✅ DONE | ... |
| #20 | CH-20 | claude/ch-20-...             | ❌ FAILED — <one line> | — |
| #21 | CH-21 | —                           | ⛔ blocked by #20 failure | — |

DONE: {x}  ·  FAILED: {y}  ·  BLOCKED-BY-FAILURE: {z}

Branches are pushed but NO PRs were opened (per your setting). Open PRs when ready.

⚠️ Possible merge order: issues touching the same hotspot (router.go / migrations / i18n)
were serialized where detected, but review these branches' overlaps before merging:
{list any pairs you serialized or suspect}

Questions raised & answered during the run: {list, or "none"}
```

---

## Guardrails

- **Never** push to a branch other than the per-issue `claude/ch-{N}-{slug}` branches.
  Do not open PRs.
- **Never** guess on a genuine owner-decision question — that's what `NEEDS_INPUT` +
  `AskUserQuestion` are for. But also don't escalate routine implementation choices.
- **One failure does not stop the run** — only its dependents are blocked.
- Respect every rule in `CLAUDE.md` inside each sub-agent (it reads CLAUDE.md during PRIME).
- If the work set is empty (no open issues ≥ start), say so and stop.
```
