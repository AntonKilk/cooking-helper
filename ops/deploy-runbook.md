# Cooking Helper — Deploy & Beta Runbook (Mac mini)

**Issue:** [#21](https://github.com/AntonKilk/cooking-helper/issues/21) (CH-21) · Phase 4
**Refs:** tech-design §3.5 (hosting), §7 (operations); README "Run with Docker" / "HTTPS / PWA"

This is the host-side runbook for what the web sandbox **cannot** do: deploy to the Mac
mini, run the deferred verification checks, install backups, and verify the PWA on a real
iPad. Work top to bottom; tick each box.

---

## 0. Prerequisites (Mac mini)

- [ ] Docker Desktop installed and running
- [ ] Tailscale installed, host joined to the tailnet, MagicDNS on
- [ ] `ANTHROPIC_API_KEY` exported in the shell that runs `docker compose`
      (never commit it; compose reads it from the host env — see `docker-compose.yml`)
- [ ] Repo checked out at a known absolute path (used by the launchd plist below)

---

## 1. Deferred checks from dev (run these FIRST — they gate release)

These were impossible in the web sandbox and are blockers per issue #21.

- [ ] **`govulncheck`** — never run against the CH-3 deps (`modernc.org/sqlite`,
      `golang-migrate`, `google/uuid`). On a networked host:
      ```bash
      GOTOOLCHAIN=go1.26.3 go install golang.org/x/vuln/cmd/govulncheck@latest
      GOTOOLCHAIN=go1.26.3 govulncheck ./...
      ```
      Expect green. (In the sandbox, install worked but `vuln.go.dev` returned 403, so it
      was deferred here.) **If a real CVE surfaces, bump the dep before release.**
- [ ] **`docker build` on real base images** — not exercised since CH-2:
      ```bash
      docker compose build        # pulls golang:1.26.3-alpine + alpine:3.20 for real
      docker compose config       # validate the compose file (incl. the new ./backups mount)
      ```
- [ ] **Static build sanity** (also runs in CI, cheap to re-confirm):
      ```bash
      CGO_ENABLED=0 go build -trimpath -o /tmp/server ./cmd/server
      ```

---

## 2. Bring up the app

```bash
docker compose up -d --build
curl -fsS http://localhost:8080/healthz   # → 200 {"status":"ok"}
docker compose logs -f cooking-helper      # confirm migrations applied, no errors
```

- [ ] `/healthz` returns 200 locally
- [ ] Logs show DB connected + migrations applied, structured JSON, no secrets/prompt bodies

---

## 3. Tailscale Serve (tailnet HTTPS — required for the Service Worker)

```bash
# Front the container's :8080 with tailnet HTTPS (Let's Encrypt, automatic).
tailscale serve --bg 8080
tailscale serve status         # note the https://cooking-helper.<tailnet>.ts.net URL
```

- [ ] HTTPS URL resolves: `curl -fsS https://cooking-helper.<tailnet>.ts.net/healthz` → 200
- [ ] **Funnel is OFF** — access must stay tailnet-only (tech-design §3.5). Do not enable
      external exposure without explicit owner approval.

---

## 4. iPad verification (Service Worker / PWA — CH-6, CH-20)

On the iPad (Tailscale client installed, joined to the tailnet):

- [ ] Open the tailnet HTTPS URL in Safari
- [ ] Share → **Add to Home Screen**; launch standalone (iOS web-app chrome shows)
- [ ] **Service Worker registers** (HTTPS satisfied) — confirm cache populated
- [ ] **Offline test** (Airplane mode): a previously opened recipe loads on **full reload**
      AND on **HTMX tap-through** (verifies the CH-20 split-cache: `cooking-shell-v2` +
      `cooking-htmx-v2`)
- [ ] Run the full `beta-1-checklist.md` (scenarios A + B) on-device

---

## 5. Daily DB backup (tech-design §7)

Files: `ops/backup/backup.sh`, `ops/backup/com.cookinghelper.backup.plist`.

> Why a script + shell wrapper: launchd execs a binary directly (no shell), so
> `$(date)`/`find` only work when wrapped in `/bin/sh -lc`. And the runtime image now
> ships the `sqlite` CLI (Dockerfile run stage) so `docker exec … sqlite3 .backup` resolves
> — the server itself still uses the pure-Go driver.

1. [ ] Ensure the host backups dir exists and matches the compose bind mount
       (`./backups:/backups`, or an absolute path on the mini):
       ```bash
       mkdir -p /Users/anton/cooking-helper/backups
       ```
2. [ ] Edit absolute paths in `com.cookinghelper.backup.plist` (script path + log paths)
       to match this checkout.
3. [ ] Install + load:
       ```bash
       chmod +x ops/backup/backup.sh
       cp ops/backup/com.cookinghelper.backup.plist ~/Library/LaunchAgents/
       launchctl load ~/Library/LaunchAgents/com.cookinghelper.backup.plist
       ```
4. [ ] **Smoke test now** (don't wait for 03:00):
       ```bash
       launchctl start com.cookinghelper.backup
       ls -la /Users/anton/cooking-helper/backups/    # a YYYY-MM-DD.db should appear
       cat   /Users/anton/cooking-helper/backups/backup.log
       ```
5. [ ] **Restore-test** the dump is valid (prove backups are usable before relying on them):
       ```bash
       sqlite3 /Users/anton/cooking-helper/backups/$(date +%F).db "PRAGMA integrity_check;"
       # → "ok"
       ```
6. [ ] Confirm retention: dumps older than 14 days are pruned (`backup.sh` runs the `find`).

---

## 6. Beta trial & sign-off

- [ ] Family uses the MVP **2 weeks unaided** (PRD §12)
- [ ] Capture metrics weekly in `beta-1-checklist.md` §C
- [ ] Confirm **time open → shopping list < 2 min** (PRD §11)
- [ ] Triage bugs; **close all P0/P1** (issue #21)
- [ ] Write up results in `.agents/reports/beta-1.md` and tick the deferred-check close-out there
