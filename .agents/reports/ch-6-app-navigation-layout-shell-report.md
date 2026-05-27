# Implementation Report

**Plan**: `.agents/plans/completed/ch-6-app-navigation-layout-shell.plan.md`
**Branch**: `claude/prime-6-zLCQ7`
**Status**: COMPLETE

## Summary

Implemented the CH-6 app navigation & layout shell. Added three server-rendered routes
(`/`, `/recipe/{id}`, `/settings`) sharing one base layout (header with app title + settings
button), with HTMX partial swaps for in-app navigation (fragment for `HX-Request`, full page
otherwise — no SPA). Wired the PWA manifest and Service Worker registration into the base
layout and added an embedded `static/` asset pipeline serving CSS, vendored HTMX 2.0.4, the
manifest, the SW, and placeholder icons. Templates are intentional stubs; the weekly-plan
feature is out of scope.

## Tasks Completed

| # | Task | File(s) | Status |
|---|------|---------|--------|
| 1 | i18n keys (nav/home/settings/recipe) | `i18n/{en,fi,ru}.json` | ✅ |
| 2 | Static embed package | `static/embed.go` | ✅ |
| 3 | Nordic Kitchen CSS (fonts deferred) | `static/css/app.css` | ✅ |
| 4 | Vendor HTMX 2.0.4 | `static/js/htmx.min.js` | ✅ |
| 5 | Manifest + placeholder icons | `static/manifest.webmanifest`, `static/icons/icon-{192,512}.png` | ✅ |
| 6 | Service Worker | `static/sw.js` | ✅ |
| 7 | Base template partials | `templates/base.gohtml` | ✅ |
| 8 | Home template (replaces layout) | `templates/home.gohtml`, deleted `templates/layout.gohtml` | ✅ |
| 9 | Recipe + Settings templates | `templates/{recipe,settings}.gohtml` | ✅ |
| 10 | Convert profile template | `templates/profile.gohtml` | ✅ |
| 11 | Page-aware renderer | `internal/handler/render.go` | ✅ |
| 12 | Handler call-sites + new handlers | `internal/handler/{home,settings,recipe}.go` | ✅ |
| 13 | Static / SW / manifest handlers | `internal/handler/static.go` | ✅ |
| 14 | Router + main wiring | `internal/handler/router.go`, `cmd/server/main.go` | ✅ |
| 15 | Tests | `internal/handler/{recipe,settings,static}_test.go`, `language_test.go`, `health_test.go` | ✅ |
| 16 | Full validation + manual note | — | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ clean |
| `golangci-lint run ./...` | ✅ 0 issues |
| `go test ./...` | ✅ all packages pass |
| E2E (running server, all routes) | ✅ see below |

### E2E (server on :8099)

| Route | Result |
|-------|--------|
| `GET /` (full) | 200; doctype + `app-header` + manifest link + serviceWorker script + localized content (fi: Pakasteet) |
| `GET /` (`HX-Request`) | 200; content fragment only (no doctype) |
| `GET /recipe/xyz789` | 200; body contains id |
| `GET /settings` | 200; profile link + 3 language buttons |
| `GET /sw.js` | 200; `text/javascript` |
| `GET /manifest.webmanifest` | 200; `application/manifest+json` |
| `GET /static/css/app.css` | 200; `text/css` |
| `GET /static/js/htmx.min.js` | 200; `text/javascript` |
| `GET /healthz` (regression) | 200 |

## Files Changed

| File | Action |
|------|--------|
| `i18n/en.json`, `i18n/fi.json`, `i18n/ru.json` | UPDATE |
| `static/embed.go` | CREATE |
| `static/css/app.css` | CREATE |
| `static/js/htmx.min.js` | CREATE (vendored) |
| `static/manifest.webmanifest` | CREATE |
| `static/sw.js` | CREATE |
| `static/icons/icon-192.png`, `static/icons/icon-512.png` | CREATE |
| `templates/base.gohtml` | CREATE |
| `templates/home.gohtml` | CREATE |
| `templates/recipe.gohtml` | CREATE |
| `templates/settings.gohtml` | CREATE |
| `templates/profile.gohtml` | UPDATE |
| `templates/layout.gohtml` | DELETE |
| `internal/handler/render.go` | UPDATE |
| `internal/handler/home.go` | UPDATE |
| `internal/handler/settings.go` | CREATE |
| `internal/handler/recipe.go` | CREATE |
| `internal/handler/static.go` | CREATE |
| `internal/handler/router.go` | UPDATE |
| `cmd/server/main.go` | UPDATE |
| `internal/handler/health_test.go` | UPDATE (NewRouter signature) |
| `internal/handler/language_test.go` | UPDATE (routes + partial-swap tests) |
| `internal/handler/recipe_test.go` | CREATE |
| `internal/handler/settings_test.go` | CREATE |
| `internal/handler/static_test.go` | CREATE |

## Deviations from Plan

- **HTMX source**: unpkg and jsDelivr both returned HTTP 403 in this environment. Vendored the
  identical pinned release (htmx 2.0.4, 50917 bytes) from
  `raw.githubusercontent.com/bigskysoftware/htmx/v2.0.4/dist/htmx.min.js` instead. Same artifact.
- **Test routers**: `health_test.go` (the only test calling `NewRouter`) was updated for the new
  `staticFS` param; `language_test.go`'s `newTestRouter` builds its own mux and gained the
  `/recipe/{id}` and `/settings` routes. No separate router builder was needed.
- **Dockerfile**: no change required — assets are compiled into the binary via `//go:embed`, and
  the build stage already `COPY . .` (static/ present at build time).

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `language_test.go` | `TestHomeHTMXReturnsFragment`, `TestHomeFullPageHasShell` (+ existing pass) |
| `recipe_test.go` | `TestRecipeRendersID`, `TestRecipeBlankIDNotFound`, `TestRecipeHTMXReturnsFragment` |
| `settings_test.go` | `TestSettingsRendersProfileLinkAndLanguageSwitcher` |
| `static_test.go` | `TestManifestServedWithType`, `TestServiceWorkerServedAsJavaScript`, `TestStaticFilesServesCSS` |

## Manual Verification Required (owner)

Per the issue's technical notes, the Service Worker only registers over HTTPS — verify over the
**tailnet Tailscale Serve URL**, not plain-HTTP `go run`:
- App opens on iPad Safari with no visual artifacts; header shows title + settings button.
- Tapping settings/title swaps content via HTMX and updates the URL.
- "Add to Home Screen" works (manifest + icons); Service Worker registers and caches the shell.

Note: icons are placeholder solid-terracotta squares — replace with real artwork in a later UI
story. Self-hosted Fraunces/Public Sans fonts are deferred (CSS uses fallback stacks).
