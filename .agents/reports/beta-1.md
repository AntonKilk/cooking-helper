# Beta-1 — Cooking Helper MVP Beta Report

**Issue:** [#21](https://github.com/AntonKilk/cooking-helper/issues/21) (CH-21) · Phase 4 (Polish & Beta)
**Branch:** `claude/prime-21-XPmb6`
**Status:** 🟡 SCAFFOLD — measured cells marked `TBD (host)` are filled in on the Mac mini after deploy + the 2-week trial.

> This is the final sign-off report for the MVP. The artifacts (checklist, backup job,
> runbook, container fixes) were authored in the dev environment; the **measured** results
> below can only come from the networked Mac mini + a real iPad + the family trial, per the
> environment constraints in `CLAUDE.md` › Validation.

---

## 1. Deployment outcome

| Item | Status | Notes |
|------|--------|-------|
| `docker compose up -d --build` on Mac mini | TBD (host) | |
| `/healthz` 200 over localhost | TBD (host) | |
| Tailscale Serve HTTPS URL (tailnet-only, no Funnel) | TBD (host) | |
| iPad sees server over tailnet | TBD (host) | |

## 2. Deferred-check close-out

These were impossible in the web sandbox and are gated here (issue #21 + the CH-20 report's
deferred list). Tick on the host:

- [ ] `govulncheck ./...` green against CH-3 deps (`modernc.org/sqlite`, `golang-migrate`, `google/uuid`)
      — _sandbox probe: install OK, `vuln.go.dev` → 403, so deferred._
- [ ] `docker build` / `docker compose up` succeeds on real base images (`golang:1.26.3-alpine`, `alpine:3.20`)
- [ ] `docker compose config` valid (incl. the new `./backups:/backups` mount)
- [ ] Service Worker registers + caches shell over tailnet HTTPS on iPad Safari (CH-6)
- [ ] PWA installs to home screen; offline reload **and** HTMX tap-through both work (CH-20 split-cache)
- [ ] iPad Safari visual pass portrait + landscape, light + dark; 44×44pt targets; AA contrast (CH-20)
- [ ] Daily backup job installed, smoke-tested, **restore-tested** (`PRAGMA integrity_check` → ok)

## 3. Success-metric results (PRD §11)

| Metric | Target | Measured | Pass? |
|--------|--------|----------|-------|
| Week generation time | ≤ 30 s | TBD (host) | ☐ |
| **Open → shopping list** | **< 2 min** | TBD (host) | ☐ |
| Shopping list auto-categorized | 100%, no manual tagging | TBD (host) | ☐ |
| Disliked ingredients excluded | 100% | TBD (host) | ☐ |
| Pantry basics never in list | 100% | TBD (host) | ☐ |
| iPad Safari, no artifacts | yes | TBD (host) | ☐ |
| UI available RU/FI/EN | yes | TBD (host) | ☐ |
| Weeks with ≥1 unswapped recipe | > 60% | TBD (host) | ☐ |
| Positive cook-feedback (by wk 4) | > 70% | TBD (host) | ☐ |
| Retention (consecutive weeks) | ≥ 3 | TBD (host) | ☐ |
| Shopping list used without edits | > 80% | TBD (host) | ☐ |

## 4. Two-week usage log

| Week | Generations | Recipes cooked | Notable feedback | Issues raised |
|------|-------------|----------------|------------------|---------------|
| 1 | TBD | TBD | TBD | TBD |
| 2 | TBD | TBD | TBD | TBD |

## 5. Bug log (P0/P1 must be closed)

| ID | Severity | Description | Status |
|----|----------|-------------|--------|
| | | | |

## 6. Verdict

TBD (host) — state whether the MVP meets PRD §11 success criteria and is ready to be
declared "done", or list the blocking items that remain.
