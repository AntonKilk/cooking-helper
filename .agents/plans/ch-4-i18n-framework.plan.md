# Plan: CH-4 — i18n framework with RU/FI/EN

## Summary

Add a self-contained internationalization layer so every UI string is served in
Russian, Finnish, or English. JSON dictionaries (`i18n/{ru,fi,en}.json`) are
**embedded** in the binary (the Dockerfile ships only the binary — disk loading is
not an option), parsed once at startup into an `i18n.Bundle`. A handler middleware
resolves the active language per request (session cookie first, then the
`Accept-Language` header, else a default), stores it in the request context, and a
per-request `t(key, args...)` translator is bound into the `html/template` FuncMap
via `Template.Clone`. A language switcher (`POST /settings/language`) writes the
cookie and redirects. The store-category table from PRD §15 is the first localized
entity and doubles as the render smoke-test (verifying Finnish diacritics and
Cyrillic round-trip in each language).

## User Story

As a member of a multilingual family,
I want to pick the UI language (RU / FI / EN),
So that everyone can read the app in their own language.

## Metadata

| Field | Value |
|-------|-------|
| Type | NEW_CAPABILITY |
| Complexity | MEDIUM |
| Systems Affected | `i18n/` (new), `internal/i18n`, `internal/handler`, `templates/` (new), `cmd/server` |
| GitHub Issue | #4 (CH-4) |

---

## Key Decisions (read before implementing)

1. **Dictionaries are embedded, not read from disk.** The Dockerfile multi-stage
   build copies only `/out/server`. Mirror the established `migrations/embed.go`
   pattern: a root `i18n/` dir holding the JSON files plus an `embed.go`.
2. **Package-name collision.** Root dir `i18n/` (package `i18n`, just the embed) and
   `internal/i18n` (package `i18n`, the logic) share a name. Import the root one
   under an alias, e.g. `dict "github.com/AntonKilk/cooking-helper/i18n"`. This
   exactly parallels `migrations` (root) being imported by `internal/repository`.
3. **Reuse `domain.Language`.** ru/fi/en constants already exist in
   `internal/domain/household.go`. `internal/i18n` imports `domain` for the
   `Language` type — do not redefine it. (domain has no deps; importing it from a
   util package is fine and does not violate the handler→service→repo→domain chain.)
4. **`t(key, args...)` binds language per request via `Template.Clone`.** Templates
   are parsed once at startup; per request we `Clone()` and `.Funcs()` a translator
   closed over the resolved language, so templates call `{{ t "key" }}` with no lang
   argument (satisfies the AC literally). `args...` go through `fmt.Sprintf`.
5. **Dependency injection mirrors `db`.** `main.go` is the composition root: it
   builds the `*i18n.Bundle` and parses templates (failing fast on bad JSON/markup),
   then injects both into `NewRouter`. `NewRouter`'s signature grows accordingly and
   the one test call site is updated.
6. **Fallback chain:** requested lang → English → the key itself (so a missing
   string is visible in the UI rather than blank). Missing keys are logged at
   warn level (no secrets, no personal data).

---

## Patterns to Follow

### Embed (mirror this exactly for the root `i18n/` dir)
```go
// SOURCE: migrations/embed.go
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

### Startup parse from embedded FS, fail fast with wrapped errors
```go
// SOURCE: internal/repository/migrate.go:17-24
source, err := iofs.New(migrations.FS, ".")
if err != nil {
	return fmt.Errorf("migrations source: %w", err)
}
```

### Context-stored per-request value + middleware (mirror for language)
```go
// SOURCE: internal/handler/router.go:11-14,38-49
type contextKey string
const requestIDKey contextKey = "request_id"

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
```

### Handler unit test with httptest (mirror for language tests)
```go
// SOURCE: internal/handler/health_test.go:23-39
req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
rec := httptest.NewRecorder()
Health(fakePinger{})(rec, req)
if rec.Code != http.StatusOK {
	t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
}
```

### Table/round-trip test style with explicit `want` messages
```go
// SOURCE: internal/repository/household_test.go
if got.Language != domain.LanguageRU {
	t.Fatalf("language = %q, want %q", got.Language, domain.LanguageRU)
}
```

---

## Files to Change

| File | Action | Purpose |
|------|--------|---------|
| `i18n/en.json` | CREATE | English dictionary (default/fallback). Store categories + UI/test keys. |
| `i18n/ru.json` | CREATE | Russian dictionary (Cyrillic). |
| `i18n/fi.json` | CREATE | Finnish dictionary (ä/ö diacritics). |
| `i18n/embed.go` | CREATE | `package i18n`, embeds `*.json`. Mirrors `migrations/embed.go`. |
| `internal/i18n/bundle.go` | CREATE | Parse embedded dicts → `Bundle`; `Load`, `Translator`, `Detect`, `Languages`. |
| `internal/i18n/doc.go` | UPDATE | Keep package doc; ensure no duplicate package comment in `bundle.go`. |
| `internal/i18n/bundle_test.go` | CREATE | Parse all dicts, render each lang (FI/Cyrillic), args substitution, fallback, detection. |
| `internal/handler/language.go` | CREATE | Language middleware, `LanguageFromContext`, `SetLanguage` handler, cookie const. |
| `internal/handler/render.go` | CREATE | Clone base templates + bind per-request `t`; render helper. |
| `internal/handler/home.go` | CREATE | `GET /` demo handler rendering localized store categories + switcher (the render smoke-test surface). |
| `internal/handler/language_test.go` | CREATE | Detection precedence, cookie set+redirect, validation of bad lang, render in each lang. |
| `templates/layout.gohtml` | CREATE | Minimal page using `{{ t "..." }}`, lists store categories, renders switcher form. |
| `templates/embed.go` | CREATE | `package templates`, embeds `*.gohtml`. Mirrors `migrations/embed.go`. |
| `internal/handler/router.go` | UPDATE | New `NewRouter` signature; wire language middleware; add `GET /` + `POST /settings/language`. |
| `internal/handler/health_test.go` | UPDATE | Update the one `NewRouter(...)` call site to the new signature. |
| `cmd/server/main.go` | UPDATE | Build `*i18n.Bundle` + parse templates at startup (fail fast); inject into `NewRouter`. |

---

## Tasks

Execute in order. Each task is atomic and verifiable.

### Task 1: Create the three dictionaries

- **Files**: `i18n/en.json`, `i18n/ru.json`, `i18n/fi.json`
- **Action**: CREATE
- **Implement**: Flat `{"key": "value"}` JSON, UTF-8, same key set in all three.
  - Store categories (first localized entity, from PRD §15 lines 465-470):
    `category.produce`, `category.meat_fish`, `category.dairy`, `category.pantry`,
    `category.frozen`, `category.other`.
    - EN: Produce / Meat & Fish / Dairy / Pantry / Frozen / Other
    - RU: Овощи и фрукты / Мясо и рыба / Молочное / Бакалея / Заморозка / Прочее
    - FI: Vihannekset ja hedelmät / Liha ja kala / Maitotuotteet / Kuivatuotteet / Pakasteet / Muut
  - UI keys for the demo + switcher: `app.title`, `settings.language`,
    `lang.ru`, `lang.fi`, `lang.en`, `shopping.categories_heading`.
  - One **args** key to exercise `t(key, args...)`: `greeting` →
    EN `"Hello, %s"`, RU `"Привет, %s"`, FI `"Hei, %s"`.
- **Validate**: `python3 -c "import json,glob;[json.load(open(f)) for f in glob.glob('i18n/*.json')]"` (or rely on the Go parse test in Task 4).

### Task 2: Embed the dictionaries

- **File**: `i18n/embed.go`
- **Action**: CREATE
- **Implement**: `package i18n`; `//go:embed *.json`; `var FS embed.FS`. Package doc
  one line: "Package i18n embeds the RU/FI/EN JSON dictionaries…".
- **Mirror**: `migrations/embed.go`
- **Validate**: `go build ./...`

### Task 3: Bundle, translator, detection

- **File**: `internal/i18n/bundle.go`
- **Action**: CREATE (package `i18n`; keep the package doc in `doc.go`, none here)
- **Implement**:
  - `type Bundle struct { dicts map[domain.Language]map[string]string; defaultLang domain.Language }`
  - `func Load(fsys fs.FS, defaultLang domain.Language) (*Bundle, error)` — walk the
    three known languages, `fs.ReadFile` each `<lang>.json`, `json.Unmarshal` into the
    map; wrap errors `fmt.Errorf("load i18n %s: %w", lang, err)`. Reject if the default
    lang's dict is missing.
  - `func (b *Bundle) translate(lang domain.Language, key string, args ...any) string`
    — lookup in `lang`, then `defaultLang`, else return `key`; if `len(args) > 0`,
    `fmt.Sprintf(tmpl, args...)`. Log warn on missing key (no PII).
  - `func (b *Bundle) Translator(lang domain.Language) func(key string, args ...any) string`
    — returns a closure over `lang` for the template FuncMap.
  - `func (b *Bundle) Has(lang domain.Language) bool` and `func (b *Bundle) Default() domain.Language`.
  - `func Detect(cookie string, acceptLanguage string, b *Bundle, def domain.Language) domain.Language`
    — cookie (if a known, present lang) → first matching tag parsed from
    `Accept-Language` (split on `,`, strip `;q=`, match `ru`/`fi`/`en` prefix) → `def`.
- **Mirror**: `internal/repository/migrate.go:17-24` (FS source + wrapped errors),
  `internal/domain/household.go:11-17` (Language constants — reuse, don't redefine).
- **Validate**: `go build ./... && go vet ./...`

### Task 4: Bundle tests

- **File**: `internal/i18n/bundle_test.go`
- **Action**: CREATE
- **Implement**: Use the real embedded `dict.FS` (import root `i18n` aliased).
  - `Load` succeeds; all three langs present.
  - `Translator(fi)("category.frozen")` == `"Pakasteet"`; `(ru)("category.produce")`
    == `"Овощи и фрукты"` — asserts FI diacritics + Cyrillic survive embed+parse.
  - Args: `Translator(ru)("greeting", "Антон")` == `"Привет, Антон"`.
  - Fallback: unknown key returns the key; missing-in-lang falls back to EN.
  - `Detect`: cookie wins over header; header parsed when no cookie; default when neither.
- **Mirror**: `internal/repository/household_test.go` (assertion style),
  `internal/handler/health_test.go:14-16` (`slogDiscard` if a logger is needed).
- **Validate**: `go test ./internal/i18n/...`

### Task 5: Language middleware + switcher handler

- **File**: `internal/handler/language.go`
- **Action**: CREATE
- **Implement**:
  - `const languageCookie = "lang"`; `const languageKey contextKey = "language"`
    (reuse the existing `contextKey` type in `router.go`).
  - `func LanguageFromContext(ctx) domain.Language`.
  - `func languageMiddleware(b *i18n.Bundle, next http.Handler) http.Handler` —
    read cookie + `Accept-Language`, call `i18n.Detect`, store in context, chain.
  - `func SetLanguage(b *i18n.Bundle) http.HandlerFunc` — `POST`; read `lang` form
    value; validate against `b.Has`; on invalid, 400 generic message (no echoing raw
    input); on valid, `http.SetCookie` (`Path:"/"`, `HttpOnly`, `SameSite=Lax`,
    `MaxAge` ~1yr) then `http.Redirect` 303 to `/` (HTMX-friendly; honor `Referer`
    if present and same-origin).
- **Mirror**: `internal/handler/router.go:11-14,38-49` (contextKey + middleware shape).
- **Validate**: `go build ./...`

### Task 6: Template renderer with per-request `t`

- **File**: `internal/handler/render.go`
- **Action**: CREATE
- **Implement**:
  - `type renderer struct { tmpl *template.Template; bundle *i18n.Bundle }`
  - `func (rd *renderer) render(w, r, name string, data any)` — resolve lang from
    context, `rd.tmpl.Clone()`, `.Funcs(template.FuncMap{"t": rd.bundle.Translator(lang)})`,
    `ExecuteTemplate` into a `bytes.Buffer` first (so a render error doesn't write a
    half page), set `Content-Type: text/html; charset=utf-8`, copy buffer out; on
    error log + generic 500.
  - Note: register a no-op `t` in the base parse FuncMap in `main.go` so parsing
    succeeds before per-request binding (template funcs must exist at parse time).
- **Mirror**: error-hiding + generic message rule (CLAUDE.md Security); buffer-then-write.
- **Validate**: `go build ./...`

### Task 7: Demo template + embed

- **Files**: `templates/layout.gohtml`, `templates/embed.go`
- **Action**: CREATE
- **Implement**:
  - `layout.gohtml`: `<!doctype html>`, `<html lang>`, `<title>{{ t "app.title" }}</title>`,
    a heading `{{ t "shopping.categories_heading" }}`, a `<ul>` listing the six
    `{{ t "category.*" }}` strings, and a switcher `<form method="post"
    action="/settings/language">` with a button per language (`{{ t "lang.ru" }}` …).
    Nordic Kitchen styling is **out of scope for CH-4** — minimal markup only; CH wiring
    of the design system comes later. (Do not pull in the frontend-design skill here.)
  - `templates/embed.go`: `package templates`; `//go:embed *.gohtml`; `var FS embed.FS`.
- **Mirror**: `migrations/embed.go`.
- **Validate**: `go build ./...`

### Task 8: Wire the router

- **File**: `internal/handler/router.go`
- **Action**: UPDATE
- **Implement**: Change signature to
  `func NewRouter(logger *slog.Logger, db *sql.DB, bundle *i18n.Bundle, tmpl *template.Template) http.Handler`.
  Build the `renderer`; register `GET /` (home/demo handler from Task 9),
  `POST /settings/language` (`SetLanguage(bundle)`), keep `GET /healthz`. Wrap the mux
  with `languageMiddleware(bundle, …)` **inside** `requestLogger` so both context
  values are present. `/healthz` must remain unaffected by language logic.
- **Mirror**: existing `NewRouter` body (router.go:17-22) + middleware composition.
- **Validate**: `go build ./...`

### Task 9: Home/demo handler

- **File**: `internal/handler/home.go`
- **Action**: CREATE
- **Implement**: `func (rd *renderer) Home(w, r)` (or a `Home(rd)` constructor) that
  renders `layout.gohtml`. Data struct carries the current `domain.Language` (for
  `<html lang>`) and the ordered list of category keys. Keep it thin.
- **Mirror**: `internal/handler/health.go:16-32` (handler-returns-HandlerFunc shape).
- **Validate**: `go build ./...`

### Task 10: Compose in main + fix test call site

- **Files**: `cmd/server/main.go`, `internal/handler/health_test.go`
- **Action**: UPDATE
- **Implement**:
  - `main.go`: after migrations, `bundle, err := i18n.Load(dict.FS, domain.LanguageEN)`
    (wrap error `fmt.Errorf("load i18n: %w", err)`); parse templates:
    `template.New("").Funcs(template.FuncMap{"t": noopT}).ParseFS(templates.FS, "*.gohtml")`
    (wrap error). Pass both into `handler.NewRouter(logger, db, bundle, tmpl)`. Import
    root `i18n` aliased as `dict`.
  - `health_test.go:62`: update `NewRouter(slogDiscard(), db)` → build a test bundle
    (`i18n.Load(dict.FS, domain.LanguageEN)`) + parsed templates and pass them, OR add
    a small test helper. `/healthz` assertion stays identical.
- **Mirror**: `cmd/server/main.go:50-58` (open db, run migrations, wrapped errors,
  `logger.Info` readiness line).
- **Validate**: full suite below.

---

## Risks

| Risk | Mitigation |
|------|------------|
| `embed` cannot reference `../i18n` from inside `internal/i18n` | Put JSON + `embed.go` in the **root** `i18n/` package (Task 2), import aliased — mirrors `migrations`. |
| Package-name clash (`i18n` root vs `internal/i18n`) | Import the root embed package under alias `dict`. |
| `html/template` funcs must exist at parse time, but `t` is per-request | Parse base template with a no-op `t`; bind the real translator per request via `Template.Clone().Funcs()`. |
| Dockerfile ships only the binary | Dictionaries + templates are embedded, not disk-loaded — no Dockerfile change needed. |
| Changing `NewRouter` signature breaks callers | Only two call sites (`main.go`, `health_test.go`); both updated in Tasks 8/10. |
| Raw user input echoed in switcher error (XSS / injection) | Validate `lang` against `bundle.Has`; return a generic 400, never reflect the raw value (CLAUDE.md Security). |
| FI/Cyrillic corruption through embed→parse→render | Explicit assertions on `Pakasteet` and `Овощи и фрукты` in Tasks 4 and the render test (AC: renders correctly incl. FI/RU chars). |

---

## Validation

```bash
gofmt -s -l .            # formatting — no output = clean
go vet ./...             # vet
golangci-lint run ./...  # lint (if installed)
go test ./...            # tests
# No new third-party deps are added, so govulncheck is not required for CH-4.
```

---

## Acceptance Criteria

- [ ] First request resolves language from `Accept-Language`; thereafter from the session cookie (Tasks 5, 3; covered by `language_test.go`).
- [ ] Language switcher updates the cookie and re-renders (303 redirect, HTMX-friendly) (Task 5).
- [ ] All UI strings go through `t(key, args...)` registered in `template.FuncMap` — no hardcoded strings in templates (Tasks 6, 7).
- [ ] `i18n/ru.json` / `fi.json` / `en.json` are loaded (embedded + parsed at startup) (Tasks 1, 2, 3, 10).
- [ ] A test template renders correctly in each language, including Finnish/Russian characters (Tasks 4, 9; `bundle_test.go` + `language_test.go`).
- [ ] Store categories (PRD §15) are the first localized entity (Task 1).
- [ ] Generated recipes are not re-translated — no recipe code touched (scope guard).
- [ ] `gofmt`, `go vet`, `go test ./...` all pass.
- [ ] Follows existing embed / middleware / DI / test patterns.
```
