# Plan: CH-21 Beta Testing & Bug Bash

## Summary

CH-21 is the **deploy gate** for the MVP: it deploys Cooking Helper to the Mac mini
(Docker + Tailscale Serve), runs the verification checks that the web sandbox could not
run during Phases 1–3, sets up daily DB backups, and runs a structured 2-week family
beta against the PRD §11 success metrics. The work splits cleanly in two:

1. **Authorable in this sandbox now** — the operational *artifacts* that make the
   deploy + beta repeatable and the deferred checks runnable on the host: a beta-testing
   checklist (≥10 scenarios traced to user stories), a `launchd` daily-backup job +
   retention, a deploy/runbook doc, a fix to the container so the documented backup
   command actually works, and the `beta-1.md` final-report scaffold.
2. **Deferred-and-recorded — must run on the networked Mac mini / tailnet / iPad** —
   the actual deploy, `govulncheck`, `docker build`/`compose up` on real base images,
   Service-Worker-over-HTTPS verification on iPad Safari, the 2-week trial, the
   time-to-shopping-list measurement, and P0/P1 bug closure.

The sandbox **probed** the two network-gated checks this session (recorded below):
`govulncheck` installs fine, but `vuln.go.dev` returns **403** → genuinely deferred.

## User Story

As a developer
I want to run a structured MVP test with the author's family on real hardware
So that I can confirm the PRD §11 success metrics are met before calling the MVP done.

## Metadata

| Field | Value |
|-------|-------|
| Type | TECHNICAL (ops + verification, minimal app code) |
| Complexity | SMALL (per stories.md) — mostly artifacts + a one-line Dockerfile fix |
| Systems Affected | Dockerfile (backup tooling), `ops/` (new), `.agents/reports/` |
| GitHub Issue | #21 |
| Branch | `claude/prime-21-XPmb6` |

---

## Source-of-Truth References

- Issue #21 acceptance criteria (the master checklist).
- PRD §11 Success Criteria — metrics to confirm:
  - Generation ≤ 30s; shopping list auto-categorized; disliked 100% excluded;
    pantry basics never in list; iPad Safari no artifacts; UI in RU/FI/EN.
  - **Time from open → shopping list < 2 min** (the headline beta metric).
  - ≥60% weeks with ≥1 unswapped recipe; >70% positive cook feedback (4wk);
    retention ≥3 consecutive weeks.
- PRD §12 Phase 4 validation — family uses MVP 2 weeks unaided.
- tech-design §7 Operations — backup recipe (the canonical command below):
  ```bash
  /usr/bin/docker exec cooking-helper sqlite3 /data/cooking.db \
    ".backup /backups/$(date +%Y-%m-%d).db"
  find /backups -name "*.db" -mtime +14 -delete
  ```

---

## Patterns to Follow

### Report format (mirror CH-20 report)
```
// SOURCE: .agents/reports/ch-20-ipad-ux-polish-report.md:1-12
# CH-20 — iPad UX polish & accessibility
**Issue:** [#20](https://github.com/AntonKilk/cooking-helper/issues/20) · Phase 4 (Polish & Beta)
**Branch:** `claude/prime-20-a6b3a`
## Summary
...
## Validation (in-sandbox)
## Deferred — must run on a real device over tailnet HTTPS (gated by CH-21)
```
The CH-20 report already lists deferred items "gated by CH-21" — CH-21's report
(`beta-1.md`) is where those get **closed out**. Pull that deferred list forward.

### Container build (mirror existing Dockerfile)
```
// SOURCE: Dockerfile:18-26  (run stage)
FROM alpine:3.20
RUN apk add --no-cache wget ca-certificates \
    && adduser -D -u 10001 app \
    && mkdir -p /data \
    && chown app:app /data
```
`apk add` is the established idiom; `wget` was added solely for the healthcheck —
adding `sqlite` follows the same precedent.

### Compose service name (backup target)
```
// SOURCE: docker-compose.yml
container_name: cooking-helper      # the name `docker exec` targets in §7
volumes: [ cooking-data:/data ]     # DB at /data/cooking.db
```

---

## Critical Findings (resolve in the plan, not at deploy time)

**F1 — The documented backup command will fail as-is: the runtime image has no
`sqlite3`.** `Dockerfile:19` installs only `wget ca-certificates`, and the binary uses
the pure-Go `modernc.org/sqlite` driver (no CLI). `docker exec cooking-helper sqlite3 …`
→ `executable file not found`. **Fix:** add `sqlite` to the run-stage `apk add`. This is
the lowest-risk option — it keeps tech-design §7's exact command working and uses
SQLite's online `.backup` (WAL-safe), rather than copying a live file.

**F2 — `launchd` does not run a shell, so `$(date +%Y-%m-%d)` and `find … -delete`
won't expand/chain.** A `launchd` `ProgramArguments` array execs a binary directly.
**Fix:** ship a `backup.sh` script (does the dated `.backup` + retention `find`) and
have the plist run `/bin/sh -lc /…/backup.sh` (or invoke the script directly with a
shebang). Never put `$(…)` or `&&` in a bare `ProgramArguments` entry.

**F3 — `/backups` must exist on the host and be reachable by the exec.** The `.backup`
writes to `/backups/…` *inside the container*, but `/backups` is not mounted →
the file lands in the container's ephemeral layer and is lost on recreate. **Fix:**
either (a) mount a host dir to `/backups` in compose, or (b) write to `/data/backups`
(already a persisted volume) and let `launchd` copy/prune on the host. Plan picks
**(a)**: add a `cooking-backups:/backups` (or host bind) mount so backups survive
container recreation and are visible to the host retention `find`. Decision flagged in
Task 2 — pick the host bind so macOS `find -mtime` prunes them directly.

**F4 — Backups can race a write (SQLite single-writer).** `.backup` is online-safe, but
schedule at 03:00 (per §7) when the family isn't generating. No code change; note in runbook.

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `.agents/reports/beta-1-checklist.md` | CREATE | ≥10 beta scenarios traced to user stories + metric capture sheet. The "checklist составлен" criterion. |
| `ops/backup/backup.sh` | CREATE | Dated `.backup` via `docker exec` + 14-day retention `find`. Resolves F1/F2. |
| `ops/backup/com.cookinghelper.backup.plist` | CREATE | `launchd` daily-03:00 job invoking `backup.sh` through a shell. Resolves F2. |
| `ops/deploy-runbook.md` | CREATE | Mac mini deploy steps (Docker + Tailscale Serve), the deferred-check run order, backup install + restore-test, iPad PWA verification steps. |
| `Dockerfile` | UPDATE | Add `sqlite` to run-stage `apk add` so §7's backup command works. Resolves F1. |
| `docker-compose.yml` | UPDATE | Mount a backups location (host bind → `/backups`) so backups persist + are host-visible. Resolves F3. |
| `.agents/reports/beta-1.md` | CREATE | Final beta report **scaffold** — pre-fills structure, success-metric table, and the deferred-check close-out list; results filled in on the host after the trial. |
| `README.md` | UPDATE (small) | Add a "Beta users" pointer + backup/deploy note (PRD §12 deliverable: "README и инструкция для бета-пользователей"). |

No `internal/**` Go code changes are expected. If the on-device pass surfaces P0/P1 bugs,
those are fixed under their own tasks/commits referencing CH-21 (out of scope for *this* plan).

---

## Tasks

Execute in order. Tasks 1–7 are sandbox-authorable; Task 8 records the deferred gate.

### Task 1: Beta-testing checklist (≥10 scenarios)

- **File**: `.agents/reports/beta-1-checklist.md`
- **Action**: CREATE
- **Implement**: A runnable test script the family follows, each row traced to a
  user story / PRD criterion, with pass/fail + notes columns. Cover at minimum:
  1. **First-run onboarding** (CH-19) — 3–4 screens, set family size + language, pantry defaults, skip works, doesn't reappear.
  2. **One-tap week generation** (CH-8/F-1) — ≤2 taps, completes ≤30s; **start a stopwatch → record time-to-shopping-list (<2 min target, PRD §11)**.
  3. **Recipe swap + full regenerate** (CH-9/F-2).
  4. **Disliked exclusion** (CH-10/CH-15/F-7) — add a disliked ingredient, regenerate, confirm 100% absent across all 3 recipes + list.
  5. **Pantry basics never in shopping list** (CH-14/F-6) — confirm 100%.
  6. **Shopping list auto-categorization + check-off** (CH-12/CH-13/F-3) — categories without manual tagging; checkbox state survives reload (HTMX absolute-state idiom).
  7. **Fullscreen recipe during cooking** (CH-11/F-4) — readable at 50cm, no zoom needed, cooking-steps stepping works.
  8. **Feedback collection** (CH-16/F-5) — like / dislike / cook-again recorded.
  9. **Feedback influences next generation** (CH-17) — disliked-feedback recipe not repeated next week.
  10. **Recipe archive search + "cook again"** (CH-18/F-8).
  11. **i18n switch RU↔FI↔EN** (CH-4/F-9) — all UI strings translate; existing recipes keep original language.
  12. **iPad Safari portrait + landscape, light + dark** (CH-20) — no artifacts, 44×44pt targets, AA contrast.
  13. **PWA install + offline** (CH-6/CH-20) — install to home screen; airplane-mode reload of a cached recipe + HTMX tap-through both work (verifies the split-cache fix).
  14. **Healthcheck** — `GET /healthz` → 200 over tailnet.
  Include a **metrics capture sheet**: per-week table for time-to-list, weeks-with-unswapped-recipe (≥60%), positive-cook-feedback % (>70%), retention weeks (≥3).
- **Mirror**: story IDs + criteria from `.agents/stories/stories.md` and PRD §11.
- **Validate**: `gofmt -s -l .` (no-op for `.md`); visual read-through that every PRD §11 functional + quality item maps to ≥1 row.

### Task 2: Persist + expose backups in compose

- **File**: `docker-compose.yml`
- **Action**: UPDATE
- **Implement**: Add a host bind mount for backups so the `.backup` output survives
  container recreation and is visible to the host's retention `find`, e.g.
  `- ./backups:/backups` (document that on the Mac mini this is an absolute host path,
  e.g. `/Users/anton/cooking-helper/backups:/backups`). Resolves F3. Keep the existing
  `cooking-data` volume untouched.
- **Mirror**: existing `volumes:` block in `docker-compose.yml`.
- **Validate**: `docker compose config` (deferred — no daemon in sandbox; YAML hand-checked here, run on host per Task 8).

### Task 3: Add sqlite to the runtime image

- **File**: `Dockerfile`
- **Action**: UPDATE
- **Implement**: Add `sqlite` to the run-stage `apk add --no-cache` line so
  `docker exec cooking-helper sqlite3 …` resolves. Resolves F1. Leave the build stage
  and `CGO_ENABLED=0` untouched (the app still uses pure-Go `modernc.org/sqlite`; the CLI
  is only for `.backup`).
- **Mirror**: `Dockerfile:19` (`apk add --no-cache wget ca-certificates`).
- **Validate**: `CGO_ENABLED=0 go build -o /tmp/server ./cmd/server` (build unaffected by run-stage change). Actual image build deferred to Task 8.

### Task 4: Backup script

- **File**: `ops/backup/backup.sh`
- **Action**: CREATE
- **Implement**: POSIX `sh` script: `set -eu`; run the §7 online backup via
  `docker exec cooking-helper sqlite3 /data/cooking.db ".backup /backups/$(date +%F).db"`;
  then prune `find /backups -name '*.db' -mtime +14 -delete`. Make it idempotent/safe to
  re-run (overwrite same-day file). Log start/finish with a timestamp to stdout (launchd
  captures to a log path set in the plist). Resolves F2. `chmod +x`.
- **Mirror**: tech-design §7 commands verbatim (adapted to a script).
- **Validate**: `sh -n ops/backup/backup.sh` (syntax). Functional run deferred to host (Task 8).

### Task 5: launchd plist

- **File**: `ops/backup/com.cookinghelper.backup.plist`
- **Action**: CREATE
- **Implement**: A `launchd` Daily-at-03:00 job (`StartCalendarInterval` Hour 3 Minute 0)
  whose `ProgramArguments` is `["/bin/sh", "-lc", "/Users/<user>/cooking-helper/ops/backup/backup.sh"]`
  (placeholder path documented in the runbook). Set `StandardOutPath`/`StandardErrorPath`
  to a log file under the backups dir. `RunAtLoad` false. Resolves F2 (shell wrapper makes
  `$(date)` expand). Label `com.cookinghelper.backup`.
- **Mirror**: standard `launchd` plist schema (no repo precedent — keep minimal, documented).
- **Validate**: `plutil -lint` (deferred — macOS-only; XML well-formedness hand-checked here).

### Task 6: Deploy runbook

- **File**: `ops/deploy-runbook.md`
- **Action**: CREATE
- **Implement**: Step-by-step for the Mac mini operator:
  1. Prereqs: Docker Desktop, Tailscale, `ANTHROPIC_API_KEY` exported to the shell that runs compose.
  2. `docker compose up -d --build`; confirm `GET /healthz` → 200 locally.
  3. `tailscale serve` config to front `:8080` with tailnet HTTPS (Let's Encrypt) — **no Funnel** (tech-design §3.5). Confirm `https://cooking-helper.tail-xxxx.ts.net/healthz`.
  4. iPad: install Tailscale, open the tailnet URL, **install PWA to home screen**, verify Service Worker registers (HTTPS) + offline cache.
  5. **Deferred-check run order** (the issue's gated checks): `GOTOOLCHAIN=go1.26.3 govulncheck ./...` (now reachable on a networked host), `docker build`/`compose up` on real base images, SW-over-HTTPS on iPad Safari.
  6. Install backup: copy `ops/backup/*` to the host, `chmod +x backup.sh`, `cp` plist to `~/Library/LaunchAgents/`, `launchctl load`; **restore-test** a backup into a throwaway DB to prove it's valid.
  7. Where to record results: `.agents/reports/beta-1.md`.
- **Mirror**: README "Run with Docker" + "HTTPS / PWA" sections; tech-design §3.5, §7.
- **Validate**: read-through against issue #21 criteria — every "host-only" criterion has a step.

### Task 7: Final report scaffold + README beta note

- **File**: `.agents/reports/beta-1.md` (CREATE), `README.md` (UPDATE)
- **Action**: CREATE / UPDATE
- **Implement**:
  - `beta-1.md`: mirror the CH-report header (issue link, phase, branch); sections:
    Deployment outcome, **Deferred-check close-out** (pull the unchecked boxes from the
    CH-20 report + issue #21 and leave them as checkboxes to tick on the host:
    `govulncheck` green, `docker build`/`compose up`, SW-over-HTTPS on iPad), **Success-metric
    results table** (PRD §11 targets vs measured: time-to-list <2min, ≤30s gen, ≥60%, >70%,
    retention), **Bug log** (P0/P1/P2 with status), **2-week usage log**, Verdict. Mark
    measured cells `TBD (host)` so it's obviously a scaffold.
  - `README.md`: add a short "For beta users" pointer to the checklist + a one-line backup/deploy
    note linking `ops/deploy-runbook.md` (PRD §12 deliverable).
- **Mirror**: `.agents/reports/ch-20-ipad-ux-polish-report.md` structure.
- **Validate**: `gofmt -s -l .` (no-op); links resolve to created files.

### Task 8: Record the deferred gate (no code)

- **File**: (none — captured in `beta-1.md` + this plan's Verification table)
- **Action**: RECORD
- **Implement**: Confirm and document the sandbox probe results so the implementer
  doesn't re-attempt blocked checks:
  - `govulncheck` install: **OK** (proxy.golang.org reachable this session).
  - `govulncheck ./...` run: **BLOCKED** — `vuln.go.dev/index/modules.json.gz` → **403**
    (matches CLAUDE.md note). Deferred to networked host.
  - `docker build` / `compose up`: **BLOCKED** — no Docker daemon in sandbox. Deferred.
  - Service Worker over HTTPS on iPad: **BLOCKED** — no HTTPS / no device. Deferred.
  - 2-week trial, time-to-list measurement, P0/P1 closure: inherently on-device. Deferred.
- **Validate**: these appear as unticked, host-gated checkboxes in `beta-1.md`.

---

## Environment & Verification

| Verification | Runs in sandbox? | If blocked: where/when verified |
|--------------|------------------|---------------------------------|
| `gofmt -s -l .`, `go vet ./...`, `golangci-lint run ./...`, `go test ./...` | yes | — (run before commit) |
| `CGO_ENABLED=0 go build ./cmd/server` (Dockerfile change doesn't break build) | yes | — |
| `sh -n` on `backup.sh`, XML well-formedness of plist | yes | — |
| `govulncheck ./...` | **no — probed: `vuln.go.dev` 403** | Mac mini / any networked host (Task 6 step 5) |
| `docker build` / `docker compose up` on real base images | no (no daemon) | Mac mini (Task 6 step 2/5) |
| `docker compose config` (validate compose edit) | no | Mac mini (Task 6 step 2) |
| `plutil -lint` (plist), `launchctl load`, restore-test | no (macOS-only) | Mac mini (Task 6 step 6) |
| Service Worker register + offline cache over tailnet HTTPS | no (no HTTPS/device) | iPad Safari over tailnet (Task 6 step 4) |
| Time-to-shopping-list < 2 min; 2-week family trial; P0/P1 closure | no | iPad in kitchen, 2 weeks (checklist + beta-1.md) |

---

## Risks

| Risk | Mitigation |
|------|------------|
| Backup command fails on host (no `sqlite3` in image) | Task 3 adds `sqlite` to the image — verified by restore-test in runbook before relying on it. |
| `launchd` silently no-ops (`$(date)`/`&&` in bare args) | Task 5 wraps in `/bin/sh -lc`; runbook includes a manual `launchctl start` smoke test. |
| Backups lost on container recreate (not mounted) | Task 2 binds `/backups` to a host path. |
| `govulncheck` finds a real CVE in CH-3 deps on host | Out of *this* plan's scope to fix, but the runbook flags it as a release blocker; fix lands as a dep bump under its own commit. |
| Beta metrics under-measured | Checklist ships an explicit stopwatch step + per-week metrics sheet so data is captured consistently. |

---

## Validation

Run before commit (sandbox — exact commands from CLAUDE.md):

```bash
gofmt -s -l .            # formatting — no output = clean
go vet ./...
golangci-lint run ./...  # v2 under pinned go1.26.3
go test ./...
CGO_ENABLED=0 go build -o /tmp/server ./cmd/server   # Dockerfile run-stage change can't break this, but confirm
sh -n ops/backup/backup.sh                            # script syntax
# govulncheck ./...  → DEFERRED (vuln.go.dev 403 in sandbox; run on Mac mini — Task 6/8)
```

---

## Acceptance Criteria

- [ ] Beta-testing checklist with ≥10 story-traced scenarios + metrics sheet (`beta-1-checklist.md`)
- [ ] `launchd` backup job + `backup.sh` with 14-day retention; image carries `sqlite`; backups persisted via mount
- [ ] Deploy runbook covers Mac mini Docker + Tailscale Serve + deferred-check run order + restore-test
- [ ] `beta-1.md` scaffold with success-metric table + deferred-check close-out list
- [ ] README points beta users at the checklist + runbook
- [ ] Sandbox validation passes (gofmt/vet/lint/test/build/`sh -n`)
- [ ] Environment-blocked verifications recorded as host-gated checkboxes (govulncheck/docker/SW/2-week trial)
- [ ] Issue #21 remaining boxes explicitly mapped to host steps (nothing silently dropped)
```
