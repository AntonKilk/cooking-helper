# CH-20 — iPad UX polish & accessibility

**Issue:** [#20](https://github.com/AntonKilk/cooking-helper/issues/20) · Phase 4 (Polish & Beta)
**Branch:** `claude/prime-20-a6b3a`

## Summary

Closed the gaps between the rendered UI and the CH-20 acceptance checklist. The
dominant problem was that large parts of the app shipped with **no CSS** — the
weekly menu cards, home generate section, archive, disliked, and pantry views
fell back to raw browser defaults. Plus measurable WCAG AA contrast failures on
the terracotta accent and an incomplete dark-mode palette.

## Changes

### `static/css/app.css`
- **New token `--accent-text` (`#A84B28`)** — an AA-safe darker terracotta for
  body links and button labels. The bright `--accent` (`#C2603A`) is kept for
  decorative / large use only. Measured ratios:
  - `#C2603A` link text on cream `#F5EFE6`: **3.44:1** (failed AA) → `#A84B28`: **4.97:1** ✅
  - light text on terracotta button fill: **3.67:1** (failed) → fixed by pairing `--accent-text` with `--bg`: **4.97:1** ✅
- Repointed links, header nav links, recipe back-link, shopping remove/undo, and
  the active feedback button to `--accent-text` / `--bg`.
- **Dark-mode palette completed** — added `--accent` / `--accent-text` (`#D17A4D`)
  and `--success` (`#8FB07E`) overrides so links and active fills clear AA on the
  dark oak background (previously these tokens weren't overridden, giving ~3.35:1
  on active buttons). Dark active button = `~4.95:1` ✅.
- **Archive nav touch-target parity** — `.app-header__archive` now gets
  `min-height: var(--touch)` like its siblings (was unstyled → <44pt).
- **Added the missing component CSS**: home/generate, weekly menu cards
  (responsive 1-col portrait → 3-col landscape grid), archive (search, list,
  cards, cook-again dialog, done/error states), disliked, pantry. Plus a default
  (secondary) button look, text-input styling, an HTMX `.htmx-indicator` rule
  (we self-host the HTMX JS only, so its stylesheet wasn't present), a global
  `:focus-visible` ring, and `env(safe-area-inset-*)` padding for the iPad
  landscape home indicator / standalone PWA.

### `templates/base.gohtml`
- `viewport-fit=cover` (pairs with the safe-area CSS). Page zoom left enabled
  (accessibility).
- Light/dark `theme-color` variants.
- iOS standalone meta tags: `apple-mobile-web-app-capable`,
  `apple-mobile-web-app-status-bar-style`, `apple-mobile-web-app-title`.

### `static/sw.js`
- Caches HTMX partial GETs so tapping through to a recipe works offline, not
  just a full reload. Because `/recipe/{id}` returns a **full page** for a
  navigation but only the **content fragment** for an HTMX request (varies on
  `HX-Request`, no `Vary` header), HTMX partials go in a **separate cache**
  (`cooking-htmx-v2`) so the two representations never collide. Full navigations
  and static assets use `cooking-shell-v2`. Cache version bumped to v2;
  cooking-steps.js / shopping-filter.js added to the precache shell.

## Validation (in-sandbox)

- `gofmt -s -l .` — clean
- `go vet ./...` — clean
- `golangci-lint run ./...` (v2 under pinned go1.26.3) — **0 issues**
- `go test ./...` — all pass (handler tests render the templates → parse verified)
- `CGO_ENABLED=0 go build ./cmd/server` — OK

## Deferred — must run on a real device over tailnet HTTPS (gated by CH-21)

These cannot be verified in the sandbox (no HTTPS for SW activation, no real
device, no Docker):

- [ ] iPad Safari visual pass, portrait **and** landscape — no layout artifacts
- [ ] Dark-mode visual pass on-device (palette correctness)
- [ ] PWA install to home-screen; standalone launch shows the iOS web-app chrome
- [ ] Service Worker activation + offline cache: full reload of a recipe **and**
      HTMX tap-through to a recipe both work offline (verifies the split-cache fix)
- [ ] Spot-check 44×44pt targets and AA contrast on-device with the inspector
