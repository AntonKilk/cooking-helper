# Plan: CH-6 — App navigation & layout shell

## Summary

Introduce the app's navigation skeleton: three server-rendered routes (`/`, `/recipe/{id}`,
`/settings`) sharing one base layout (header with app title + settings button), with HTMX
partial swaps for in-app navigation (no SPA). Wire the PWA manifest and a Service Worker
registration into the base layout, and add a `static/` asset pipeline (embedded + served) for
CSS, vendored HTMX, the manifest, the SW, and placeholder icons. Templates are intentionally
**stubs** — the home and recipe screens are placeholders; this story delivers the shell, not
the weekly-plan feature. Self-hosted Fraunces/Public Sans fonts are **deferred** to a later UI
story (consistent with the CH-5 styling deferral); CSS ships the Nordic Kitchen palette + sizing
with system-font fallbacks.

## User Story

As a household cook
I want basic navigation between the home screen, a recipe, and settings
So that I can use the app like an ordinary tablet app on the kitchen iPad.

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY |
| Complexity | MEDIUM (story is "Small", but touches templating, static pipeline, SW, i18n) |
| Systems Affected | `internal/handler`, `templates/`, `static/` (new), `i18n/`, `cmd/server` |
| GitHub Issue | #6 (AntonKilk/cooking-helper) |
| Blocked by | CH-2 (done) |

---

## Decisions (read before implementing)

1. **`/settings` is a new hub page**; the existing `/settings/profile` (CH-5) and
   `POST /settings/language` (CH-4) stay exactly as they are and are linked from it.
2. **Shared layout via namespaced template pairs**, not a single `base` wrapper. The renderer
   parses every `*.gohtml` into one tree and executes by name, so a single `{{define "content"}}`
   would collide across files. Each page therefore defines **two** templates:
   `"<page>/page"` (full HTML document) and `"<page>/content"` (inner fragment for HTMX swaps).
   Shared chrome lives in `base.gohtml` as collision-free named partials (`"head"`, `"header"`,
   `"sw-register"`) included by every `/page` template.
3. **HTMX partial swap**: the renderer auto-selects `"<page>/content"` when the request carries
   `HX-Request: true` (and is not an `HX-History-Restore-Request`), otherwise `"<page>/page"`.
   Header links use `hx-get` + `hx-target="#content"` + `hx-push-url="true"`; they keep a real
   `href` so they work without JS and so the SW/refresh path renders a full page.
4. **Service Worker is served from root (`/sw.js`)** so its scope covers the whole origin.
   The manifest is served from root (`/manifest.webmanifest`). Other assets are served under
   `/static/` from an embedded FS.
5. **Fonts deferred.** CSS uses `font-family` fallback stacks (serif for headings, system sans
   for body). Add a TODO note; do not fetch font binaries in this story.
6. **Icons**: generate two solid-terracotta placeholder PNGs (192, 512) with a throwaway
   `image/png` snippet — no external dependency, reproducible, satisfies install requirements.
7. **No new Go dependencies** ⇒ `govulncheck` not required for this story. HTMX is a vendored
   static file, not a Go module.

---

## Patterns to Follow

### Handler as a renderer method (home/settings/recipe pages)
```go
// SOURCE: internal/handler/home.go:24-30
func (rd *renderer) Home(w http.ResponseWriter, r *http.Request) {
	data := homeData{
		Lang:         string(LanguageFromContext(r.Context())),
		CategoryKeys: categoryKeys,
	}
	rd.render(w, r, "layout", data)
}
```

### Standalone testable handler func (static / sw / manifest)
```go
// SOURCE: internal/handler/health.go:18-32
func Health(db pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		...
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
```

### Buffered, language-bound render (do NOT bypass this)
```go
// SOURCE: internal/handler/render.go:37-56
func (rd *renderer) renderStatus(w http.ResponseWriter, r *http.Request, status int, name string, data any) {
	lang := LanguageFromContext(r.Context())
	clone, err := rd.tmpl.Clone()
	...
	clone.Funcs(template.FuncMap{"t": rd.bundle.Translator(lang)})
	var buf bytes.Buffer
	if err := clone.ExecuteTemplate(&buf, name, data); err != nil { ... }
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}
```

### Route registration (Go 1.22 method+pattern mux)
```go
// SOURCE: internal/handler/router.go:30-37
mux := http.NewServeMux()
mux.HandleFunc("GET /healthz", Health(db))
mux.HandleFunc("GET /{$}", rd.Home)
mux.HandleFunc("POST /settings/language", SetLanguage(bundle))
mux.HandleFunc("GET /settings/profile", ph.Show)
mux.HandleFunc("POST /settings/profile", ph.Save)
return requestLogger(logger, languageMiddleware(bundle, mux))
```

### Embed FS package (mirror for `static/`)
```go
// SOURCE: templates/embed.go:1-9
package templates

import "embed"

//go:embed *.gohtml
var FS embed.FS
```

### Handler test (httptest + body assertions)
```go
// SOURCE: internal/handler/language_test.go:35-74
func newTestRouter(t *testing.T) http.Handler { ... }
req := httptest.NewRequest(http.MethodGet, "/", nil)
rec := httptest.NewRecorder()
srv.ServeHTTP(rec, req)
if rec.Code != http.StatusOK { t.Fatalf(...) }
if !strings.Contains(rec.Body.String(), c.want) { t.Errorf(...) }
```

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `templates/base.gohtml` | CREATE | Shared partials: `head` (meta/manifest/css/title), `header` (title + settings link), `sw-register` (SW registration script). |
| `templates/home.gohtml` | CREATE | `home/page` + `home/content`. Main-screen stub; keeps the localized shopping-category demo list so existing i18n tests stay green. |
| `templates/recipe.gohtml` | CREATE | `recipe/page` + `recipe/content`. Recipe stub showing the path id; a not-found variant. |
| `templates/settings.gohtml` | CREATE | `settings/page` + `settings/content`. Hub: link to `/settings/profile` + language switcher buttons. |
| `templates/profile.gohtml` | UPDATE | Convert to `profile/page` + `profile/content` using shared `head`/`header`; preserve the form markup verbatim (CH-5 tests assert it). |
| `templates/layout.gohtml` | DELETE | Replaced by `base.gohtml` + `home.gohtml`. No code references the literal name `"layout"` except `home.go`. |
| `internal/handler/render.go` | UPDATE | Make `render`/`renderStatus` page-aware: pick `"<page>/content"` for HTMX requests, else `"<page>/page"`. Add `isHTMXNavigation(r)`. |
| `internal/handler/home.go` | UPDATE | Render `"home"` (was `"layout"`). Keep `categoryKeys`/`homeData`. |
| `internal/handler/settings.go` | CREATE | `rd.Settings` renderer method → renders `"settings"`. |
| `internal/handler/recipe.go` | CREATE | `rd.Recipe` method → reads `r.PathValue("id")`, renders `"recipe"`; empty/blank id → 404 not-found render. |
| `internal/handler/profile.go` | UPDATE | Swap `render`/`renderStatus` calls from name `"profile"` to page `"profile"` (no behavior change — same base name). |
| `internal/handler/static.go` | CREATE | `StaticFiles(fsys)`, `ServiceWorker(fsys)`, `Manifest(fsys)` handlers with correct Content-Types. |
| `internal/handler/router.go` | UPDATE | Register `/recipe/{id}`, `/settings`, `/static/`, `/sw.js`, `/manifest.webmanifest`; pass `static.FS` in. |
| `static/embed.go` | CREATE | `package static` embedding `css/**`, `js/**`, `icons/**`, `sw.js`, `manifest.webmanifest`. |
| `static/css/app.css` | CREATE | Nordic Kitchen palette + iPad sizing (≥18pt body / ≥24pt headings, 44px targets), `prefers-color-scheme` dark. Font fallback stacks (self-hosted fonts deferred). |
| `static/js/htmx.min.js` | CREATE (vendor) | Vendored HTMX 2.x, self-hosted (no CDN). |
| `static/sw.js` | CREATE | Minimal SW: precache app shell (cache-first for `/static/`), network-first w/ cache fallback for navigations; old-cache cleanup on activate. |
| `static/manifest.webmanifest` | CREATE | name/short_name, `start_url:"/"`, `display:"standalone"`, theme `#C2603A`, background `#F5EFE6`, 192+512 icons. |
| `static/icons/icon-192.png`, `static/icons/icon-512.png` | CREATE | Placeholder solid-color PNGs generated via `image/png`. |
| `i18n/en.json`, `i18n/fi.json`, `i18n/ru.json` | UPDATE | Add `nav.settings`, `nav.home`, `home.heading`, `home.subtitle`, `settings.heading`, `recipe.heading`, `recipe.not_found`. |
| `cmd/server/main.go` | UPDATE | Pass `static.FS` into `handler.NewRouter` (signature change). |

---

## Tasks

Execute in order. Each task is atomic and verifiable. Run `gofmt -s -w .` after Go edits.

### Task 1: i18n keys
- **File**: `i18n/en.json`, `i18n/fi.json`, `i18n/ru.json`
- **Action**: UPDATE
- **Implement**: Add the same keys to all three dicts (translate values per language; mirror
  existing style). Keys: `nav.settings`, `nav.home`, `home.heading`, `home.subtitle`,
  `settings.heading`, `recipe.heading`, `recipe.not_found`. Keep existing keys untouched.
- **Mirror**: `i18n/en.json:1-22`
- **Validate**: `go test ./internal/i18n/...`

### Task 2: Static embed package
- **File**: `static/embed.go`
- **Action**: CREATE
- **Implement**: `package static` with `//go:embed css js icons sw.js manifest.webmanifest`
  exposing `var FS embed.FS`. (Embed directives require the listed paths to exist — create the
  asset files in Tasks 3–7 before building.)
- **Mirror**: `templates/embed.go:1-9`
- **Validate**: created later with assets; `go build ./...` after Task 7.

### Task 3: CSS (Nordic Kitchen, fonts deferred)
- **File**: `static/css/app.css`
- **Action**: CREATE
- **Implement**: `:root` CSS variables for the palette (§4.5: cream `#F5EFE6`, ivory `#FCFAF5`,
  oak `#2B2118`, muted `#6B5F52`, terracotta `#C2603A`, moss `#5C7A4E`, sand `#E5DBC9`).
  Body font-size ≥1.25rem, headings ≥2rem, line-height generous, page padding 24–32px,
  single column. `.app-header` flex row (title left, settings button right), settings button
  min `44px × 44px`. `@media (prefers-color-scheme: dark)` inverts (oak bg + cream text).
  `font-family` fallback stacks — add `/* TODO: self-host Fraunces/Public Sans (deferred) */`.
- **Validate**: visual only; file exists.

### Task 4: Vendor HTMX
- **File**: `static/js/htmx.min.js`
- **Action**: CREATE (download, pinned)
- **Implement**: Fetch a pinned HTMX 2.x minified release into the file
  (`curl -fsSL https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js -o static/js/htmx.min.js`).
  Verify it is non-empty (>30 KB) and starts with valid JS.
- **Risk**: outbound network may be restricted by the environment policy. If blocked, STOP and
  ask the owner (do not commit a placeholder that silently breaks HTMX). See Risks table.
- **Validate**: `test -s static/js/htmx.min.js && head -c 40 static/js/htmx.min.js`

### Task 5: Manifest + placeholder icons
- **File**: `static/manifest.webmanifest`, `static/icons/icon-192.png`, `static/icons/icon-512.png`
- **Action**: CREATE
- **Implement**: Manifest JSON: `name` "Cooking Helper", `short_name` "Cooking",
  `start_url:"/"`, `scope:"/"`, `display:"standalone"`, `background_color:"#F5EFE6"`,
  `theme_color:"#C2603A"`, `icons` → the two PNGs with `sizes`/`type:"image/png"`.
  Generate the PNGs with a throwaway `image/png` program (solid `#C2603A` squares) and commit
  the output — no external image tooling, no new Go dep.
- **Validate**: `file static/icons/icon-192.png` reports PNG; manifest parses as JSON.

### Task 6: Service Worker
- **File**: `static/sw.js`
- **Action**: CREATE
- **Implement**: Per tech-design §4.2. `const CACHE='cooking-shell-v1';` precache shell on
  `install` (`/`, `/static/css/app.css`, `/static/js/htmx.min.js`, `/manifest.webmanifest`,
  both icons). `activate`: delete caches whose name ≠ current. `fetch`: cache-first for same-origin
  `/static/` GETs; network-first with cache fallback for navigations; otherwise pass through.
  Keep it small but correct (guard non-GET, cross-origin).
- **Validate**: `node --check static/sw.js` if node present, else manual read.

### Task 7: Base template partials
- **File**: `templates/base.gohtml`
- **Action**: CREATE
- **Implement**: Three collision-free defines:
  - `"head"`: `<meta charset>`, viewport, `<meta name="theme-color" content="#C2603A">`,
    `<link rel="manifest" href="/manifest.webmanifest">`, `<link rel="apple-touch-icon" href="/static/icons/icon-192.png">`,
    `<link rel="stylesheet" href="/static/css/app.css">`, `<title>{{ t "app.title" }}</title>`.
  - `"header"`: `<header class="app-header">` with a title link
    (`<a href="/" hx-get="/" hx-target="#content" hx-push-url="true">{{ t "app.title" }}</a>`)
    and a settings link (`<a href="/settings" hx-get="/settings" hx-target="#content"
    hx-push-url="true" aria-label="{{ t "nav.settings" }}">…</a>`).
  - `"sw-register"`: `<script>` registering `/sw.js` on `window load`, guarded by
    `'serviceWorker' in navigator`, `.catch(()=>{})`.
- **Mirror**: `templates/layout.gohtml:1-32` (chrome it replaces)
- **Validate**: parses (covered by Task 13 tests / `go build`).

### Task 8: Home template (replaces layout.gohtml)
- **File**: `templates/home.gohtml` (CREATE), `templates/layout.gohtml` (DELETE)
- **Action**: CREATE + DELETE
- **Implement**: `"home/page"` = full document: `<html lang="{{.Lang}}"><head>{{template "head" .}}</head>`,
  body with `{{template "header" .}}`, `<main id="content">{{template "home/content" .}}</main>`,
  `<script src="/static/js/htmx.min.js"></script>`, then `{{template "sw-register" .}}`.
  `"home/content"` = main-screen stub: `<h1>{{ t "home.heading" }}</h1>`, a `home.subtitle`
  paragraph, and the existing shopping-category demo list
  (`{{ range .CategoryKeys }}<li>{{ t . }}</li>{{ end }}`) so `TestHomeRendersByAcceptLanguage`
  keeps passing. Delete `layout.gohtml`.
- **Mirror**: `templates/layout.gohtml:9-29`, `templates/profile.gohtml` structure
- **Validate**: `go test ./internal/handler/... -run TestHome`

### Task 9: Recipe + Settings templates
- **File**: `templates/recipe.gohtml`, `templates/settings.gohtml`
- **Action**: CREATE
- **Implement**: Same `/page` + `/content` shape as home, including `<head>`/`header`/htmx/sw in
  the `/page` variants.
  - `recipe/content`: heading `recipe.heading`, shows `{{ .ID }}`; if `.NotFound`, render
    `recipe.not_found` instead.
  - `settings/content`: heading `settings.heading`; link to `/settings/profile`
    (`{{ t "settings.profile" }}`, hx-get + target + push-url); the language switcher form
    (the three buttons `POST /settings/language`, copied from old `layout.gohtml:24-28`).
- **Validate**: `go build ./...`

### Task 10: Convert profile template
- **File**: `templates/profile.gohtml`
- **Action**: UPDATE
- **Implement**: Split into `"profile/page"` (uses shared `head`/`header`, htmx, sw) and
  `"profile/content"` (the existing form + error block, **markup unchanged**). Move the
  `<h1>` and form into `/content`; the home link can stay or be dropped (header now provides nav).
- **Mirror**: `templates/profile.gohtml:1-39`
- **Validate**: `go test ./internal/handler/... -run TestProfile`

### Task 11: Page-aware renderer
- **File**: `internal/handler/render.go`
- **Action**: UPDATE
- **Implement**: Add `func isHTMXNavigation(r *http.Request) bool` →
  `r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-History-Restore-Request") != "true"`.
  In `renderStatus`, treat the `name` arg as the page base: choose
  `name+"/content"` when `isHTMXNavigation(r)`, else `name+"/page"`; execute that. `render`
  still delegates to `renderStatus(…, http.StatusOK, …)`. Keep buffering + `fail` behavior.
- **Mirror**: `internal/handler/render.go:30-56`
- **Validate**: `go build ./...`

### Task 12: Home/Profile handler call-site updates + new handlers
- **File**: `internal/handler/home.go`, `internal/handler/profile.go`, `internal/handler/settings.go` (CREATE), `internal/handler/recipe.go` (CREATE)
- **Action**: UPDATE + CREATE
- **Implement**:
  - `home.go`: change `rd.render(w, r, "layout", data)` → `rd.render(w, r, "home", data)`.
  - `profile.go`: `Show` → `rd.render(w, r, "profile", …)` (already name "profile" — now resolves
    to `profile/page` or `profile/content`); `renderInvalid` keeps `renderStatus(…, "profile", …)`.
  - `settings.go`: `settingsData{Lang string}`; `func (rd *renderer) Settings(w,r)` renders `"settings"`.
  - `recipe.go`: `recipeData{Lang, ID string; NotFound bool}`;
    `func (rd *renderer) Recipe(w,r)` reads `id := r.PathValue("id")`; if `strings.TrimSpace(id)==""`
    render `"recipe"` with `NotFound:true` and `renderStatus(... http.StatusNotFound ...)`,
    else render with the id. (No service/repo wiring — recipe persistence is a later story.)
- **Mirror**: `internal/handler/home.go:24-30`
- **Validate**: `go build ./...`

### Task 13: Static / SW / manifest handlers
- **File**: `internal/handler/static.go`
- **Action**: CREATE
- **Implement**:
  - `func StaticFiles(fsys fs.FS) http.Handler` → strip `/static/` prefix, serve via
    `http.FileServerFS(fsys)` (sub-FS rooted appropriately) for css/js/icons.
  - `func ServiceWorker(fsys fs.FS) http.HandlerFunc` → read `sw.js`, set
    `Content-Type: text/javascript`, write bytes (served at root for full scope).
  - `func Manifest(fsys fs.FS) http.HandlerFunc` → read `manifest.webmanifest`, set
    `Content-Type: application/manifest+json`, write bytes (`.webmanifest` isn't in Go's mime db).
  - Return generic errors via `http.Error` on read failure; never leak paths.
- **Mirror**: `internal/handler/health.go:18-32`
- **Validate**: `go build ./...`

### Task 14: Router + main wiring
- **File**: `internal/handler/router.go`, `cmd/server/main.go`
- **Action**: UPDATE
- **Implement**:
  - `NewRouter` gains a `staticFS fs.FS` param. Register:
    `GET /recipe/{id}` → `rd.Recipe`; `GET /settings` → `rd.Settings`;
    `GET /static/` → `StaticFiles(staticFS)`; `GET /sw.js` → `ServiceWorker(staticFS)`;
    `GET /manifest.webmanifest` → `Manifest(staticFS)`. Keep existing routes.
  - `main.go`: `import "…/static"`; pass `static.FS` to `handler.NewRouter(...)`.
  - Update `newTestRouter`/`newProfileRouter` in tests to pass `static.FS` (or a route subset) so
    they compile against the new signature — or keep test routers building their own mux (they
    already do) and only add static routes where a test needs them.
- **Mirror**: `internal/handler/router.go:25-38`, `cmd/server/main.go:69-78`
- **Validate**: `go build ./... && go vet ./...`

### Task 15: Tests
- **File**: `internal/handler/recipe_test.go` (CREATE), `internal/handler/settings_test.go` (CREATE), `internal/handler/static_test.go` (CREATE), `internal/handler/language_test.go` (UPDATE)
- **Action**: CREATE + UPDATE
- **Implement**:
  - `recipe_test.go`: `GET /recipe/abc123` → 200 and body contains `abc123`; `GET /recipe/abc123`
    with `HX-Request: true` → body contains the id but **not** `<!doctype` (fragment only).
  - `settings_test.go`: `GET /settings` → 200, body contains `/settings/profile` link and the
    three language buttons.
  - `static_test.go`: `Manifest`/`ServiceWorker` handlers return 200 with the expected
    Content-Type; `StaticFiles` serves `css/app.css` (200). Test the handler funcs directly with
    `static.FS` (mirror `health_test.go` style).
  - `language_test.go`: add a partial-swap assertion — `GET /` with `HX-Request: true` returns a
    fragment without `<!doctype`; keep the existing category/lang assertions (still satisfied by
    `home/page`). Update `newTestRouter` to register `/recipe/{id}` and `/settings` if a shared
    router is used.
- **Mirror**: `internal/handler/language_test.go:44-74`, `internal/handler/health_test.go`
- **Validate**: `go test ./...`

### Task 16: Full validation + manual iPad check
- **File**: —
- **Action**: VALIDATE
- **Implement**: Run the full gate (below). Note in the implementation report that PWA
  install + SW activation must be verified over the **tailnet HTTPS URL** (SW won't register over
  plain-HTTP `go run`), per the issue's technical notes — flag this as a manual owner step.
- **Validate**: see Validation section.

---

## Risks

| Risk | Mitigation |
|------|------------|
| Outbound network blocked → can't vendor `htmx.min.js` (Task 4) | Detect failure immediately (empty/HTML error file). Do **not** commit a broken placeholder; stop and ask the owner, or have them drop the file in. HTMX is required for the partial-swap AC. |
| Two `{{define "content"}}` collide in the single parsed tree | Avoided by namespacing every page as `"<page>/page"` + `"<page>/content"`; shared chrome uses unique partial names. |
| Refactoring `profile.gohtml` breaks CH-5 tests | Keep the form/input/error markup byte-for-byte inside `profile/content`; tests assert substrings that survive the move. |
| Changing `render`'s `name` semantics breaks existing callers | All callers pass a base name that now has both `/page` and `/content` defines; non-HX requests resolve to `/page`. Covered by existing + new tests. |
| `.webmanifest` served with wrong Content-Type → manifest ignored | Dedicated `Manifest` handler sets `application/manifest+json` explicitly. |
| SW scope too narrow if served from `/static/` | Serve `sw.js` from root (`GET /sw.js`) so scope is `/`. |
| SW can't be tested locally (HTTP) | Document the tailnet-HTTPS manual check; unit tests cover routes/markup, not SW activation. |
| `NewRouter` signature change ripples into tests | Update test router builders in the same task; they already construct their own mux. |

---

## Validation

```bash
gofmt -s -l .            # formatting — no output = clean
go vet ./...             # vet
golangci-lint run ./...  # lint
go test ./...            # tests
# govulncheck not required: no Go dependency added (HTMX is a vendored static asset)
```

Manual (owner, over tailnet HTTPS — not plain-HTTP `go run`):
- App opens on iPad Safari with no visual artifacts; header shows title + settings button.
- Tapping settings / title swaps content via HTMX and updates the URL.
- "Add to Home Screen" works (manifest + icons); Service Worker registers (DevTools / reload offline).

---

## Acceptance Criteria

- [ ] Routes `/`, `/recipe/{id}`, `/settings` render via `html/template`
- [ ] In-app navigation uses HTMX partial swaps (fragment for `HX-Request`, full page otherwise); no SPA
- [ ] Every screen shares the base layout header (title + settings button)
- [ ] PWA manifest linked and Service Worker registered from the base layout; `/sw.js` + `/manifest.webmanifest` + `/static/` served
- [ ] Opens on iPad Safari over tailnet HTTPS without visual artifacts (manual)
- [ ] Existing CH-4/CH-5 tests still pass; new tests cover recipe/settings/static + partial swap
- [ ] `gofmt`, `go vet`, `golangci-lint`, `go test ./...` all clean
```
