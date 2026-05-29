# Implementation Report — CH-21 Beta Testing & Bug Bash

**Plan:** `.agents/plans/completed/ch-21-beta-testing-bug-bash.plan.md`
**Issue:** [#21](https://github.com/AntonKilk/cooking-helper/issues/21) · Phase 4 (Polish & Beta)
**Branch:** `claude/prime-21-XPmb6`
**Status:** COMPLETE (sandbox-authorable scope) — host-gated items recorded, not closed

## Summary

CH-21 is the MVP deploy gate. This change delivers the **operational artifacts** that make
the Mac mini deploy, the deferred verification checks, and the 2-week family beta repeatable
— everything that can be authored without a Docker daemon, tailnet HTTPS, or a physical iPad.
The actual deploy, on-device checks, and the family trial are inherently host-only; they are
recorded as host-gated checkboxes in the runbook and `beta-1.md`, not silently dropped.

Two latent bugs in the documented backup approach were fixed in the process:
- **F1:** the Alpine runtime image had no `sqlite3` CLI, so the tech-design §7 backup command
  (`docker exec … sqlite3 .backup`) would have failed. Added `sqlite` to the run stage.
- **F2:** `launchd` execs a binary directly (no shell), so `$(date)` / `find` in a bare
  command wouldn't work. Shipped `backup.sh` and invoke it via `/bin/sh -lc` from the plist.
- **F3:** `.backup` writes to `/backups` *inside* the container, which wasn't mounted →
  backups lost on recreate. Added a `./backups:/backups` bind mount.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Beta-testing checklist (≥10 story-traced scenarios + metric sheet) | `.agents/reports/beta-1-checklist.md` | ✅ |
| 2 | Persist + expose backups via bind mount | `docker-compose.yml` | ✅ |
| 3 | Add `sqlite` CLI to runtime image | `Dockerfile` | ✅ |
| 4 | Backup script (`.backup` + 14-day retention) | `ops/backup/backup.sh` | ✅ |
| 5 | launchd daily-03:00 job (shell-wrapped) | `ops/backup/com.cookinghelper.backup.plist` | ✅ |
| 6 | Deploy + deferred-check + backup runbook | `ops/deploy-runbook.md` | ✅ |
| 7 | Final-report scaffold + README pointers | `.agents/reports/beta-1.md`, `README.md` | ✅ |
| 8 | Record deferred gate (sandbox probes) | beta-1.md / runbook | ✅ |

## Validation Results (in-sandbox)

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ 0 |
| `golangci-lint run ./...` (v2 under pinned go1.26.3) | ✅ 0 issues |
| `go test ./...` | ✅ all pass |
| `CGO_ENABLED=0 go build ./cmd/server` | ✅ OK (run-stage change can't affect build) |
| `sh -n ops/backup/backup.sh` | ✅ syntax OK |
| plist XML well-formedness | ✅ valid |
| docker-compose YAML parse | ✅ valid |

## Deferred — gated by the host (run per `ops/deploy-runbook.md`)

Recorded, not closed (no Docker daemon / HTTPS / device / `sqlite3` CLI in sandbox):

- `govulncheck ./...` — **sandbox probe this session: install OK, `vuln.go.dev` → 403**, so deferred to networked host.
- `docker build` / `docker compose up` on real base images; `docker compose config`.
- Live `.backup` run + `PRAGMA integrity_check` restore-test (container has `sqlite` after the Dockerfile fix).
- Service Worker registration + offline cache over tailnet HTTPS on iPad Safari.
- 2-week family trial; time open→shopping-list `< 2 min` measurement; P0/P1 bug closure.

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `Dockerfile` | UPDATE | +4/-1 |
| `docker-compose.yml` | UPDATE | +6 |
| `README.md` | UPDATE | +16/-3 |
| `.agents/reports/beta-1-checklist.md` | CREATE | +~85 |
| `.agents/reports/beta-1.md` | CREATE | +~70 |
| `ops/backup/backup.sh` | CREATE | +45 |
| `ops/backup/com.cookinghelper.backup.plist` | CREATE | +60 |
| `ops/deploy-runbook.md` | CREATE | +~110 |

## Deviations from Plan

None of substance. The plan anticipated the live `.backup` E2E might run in-sandbox; in this
session `sqlite3` is not installed locally, so that single command is host-gated (the
container ships it via the Dockerfile fix). All other tasks matched the plan.

## Tests Written

No Go code changed, so no unit tests were added. Verification for this ops change is the
artifact-consistency + syntax suite above (`sh -n`, plist/YAML well-formedness, full Go check
suite to confirm no regression), plus the host-gated restore-test in the runbook.
