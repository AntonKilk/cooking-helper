# Implementation Report

**Plan**: `.agents/plans/completed/ch-4-i18n-framework.plan.md`
**Branch**: `claude/prime-4-fh1dM`
**Status**: COMPLETE

## Summary

Implemented the CH-4 i18n framework for RU/FI/EN. JSON dictionaries are embedded
in the binary and parsed once at startup into an `i18n.Bundle`. A handler
middleware resolves the active language per request (session cookie → `Accept-Language`
header → default EN) and stores it in the request context. Templates are parsed
once with a no-op `t`; each request clones them and binds a language-scoped
`t(key, args...)` translator into the `html/template` FuncMap. A `POST /settings/language`
switcher validates the language, writes an `HttpOnly; SameSite=Lax` cookie, and
redirects (303). Store categories from PRD §15 are the first localized entity and
render as the demo/smoke-test page at `GET /`.

## Tasks Completed

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Three dictionaries | `i18n/{en,ru,fi}.json` | ✅ |
| 2 | Embed dictionaries | `i18n/embed.go` | ✅ |
| 3 | Bundle, translator, detection | `internal/i18n/bundle.go` | ✅ |
| 4 | Bundle tests | `internal/i18n/bundle_test.go` | ✅ |
| 5 | Language middleware + switcher | `internal/handler/language.go` | ✅ |
| 6 | Per-request `t` renderer | `internal/handler/render.go` | ✅ |
| 7 | Demo template + embed | `templates/layout.gohtml`, `templates/embed.go` | ✅ |
| 8 | Router wiring | `internal/handler/router.go` | ✅ |
| 9 | Home/demo handler | `internal/handler/home.go` | ✅ |
| 10 | Compose in main + fix test call site | `cmd/server/main.go`, `internal/handler/health_test.go` | ✅ |

## Validation Results

| Check | Result |
|-------|--------|
| `gofmt -s -l .` | ✅ clean |
| `go vet ./...` | ✅ |
| `golangci-lint run ./...` | ✅ 0 issues |
| `go test ./...` | ✅ all packages pass |
| E2E over HTTP | ✅ all scenarios pass |

## E2E Verification (live server)

- `GET /healthz` → `{"status":"ok"}`
- `GET /` with `Accept-Language: fi` → `<html lang="fi">`, `Vihannekset ja hedelmät`, `Pakasteet`
- `GET /` with `Accept-Language: ru` → `<html lang="ru">`, `Овощи и фрукты`, `Заморозка`
- `POST /settings/language lang=en` → `303 See Other`, `Set-Cookie: lang=en; Path=/; Max-Age=31536000; HttpOnly; SameSite=Lax`
- `GET /` with `Cookie: lang=ru` + `Accept-Language: fi` → renders RU (cookie wins)
- `POST /settings/language lang=zz` → `400` (raw input not echoed)

## Files Changed

| File | Action |
|------|--------|
| `i18n/en.json` | CREATE |
| `i18n/ru.json` | CREATE |
| `i18n/fi.json` | CREATE |
| `i18n/embed.go` | CREATE |
| `internal/i18n/bundle.go` | CREATE |
| `internal/i18n/bundle_test.go` | CREATE |
| `internal/handler/language.go` | CREATE |
| `internal/handler/render.go` | CREATE |
| `internal/handler/home.go` | CREATE |
| `internal/handler/language_test.go` | CREATE |
| `templates/layout.gohtml` | CREATE |
| `templates/embed.go` | CREATE |
| `internal/handler/router.go` | UPDATE |
| `internal/handler/health_test.go` | UPDATE |
| `cmd/server/main.go` | UPDATE |

## Deviations from Plan

- **`GET /` route pattern**: used `GET /{$}` (Go 1.22+ exact-match) instead of `GET /`
  so the home handler does not act as a catch-all for unmatched paths.
- **`internal/i18n/doc.go`**: left unchanged. The package comment lives there; `bundle.go`
  carries no package comment, so no duplicate-comment issue arose (plan listed it as a
  possible UPDATE — none was needed).
- **Shared handler test helpers** (`testBundle`, `testTemplates`, `newTestRouter`) were
  placed in `language_test.go` and reused by `health_test.go` (same package).
- No `govulncheck` run: no new third-party dependencies were added (plan noted this).

## Tests Written

| Test File | Test Cases |
|-----------|------------|
| `internal/i18n/bundle_test.go` | LoadAllLanguages, TranslateLocalizedChars (FI/RU), TranslateWithArgs, TranslateFallback, LoadMissingDefault, Detect (5 sub-cases) |
| `internal/handler/language_test.go` | HomeRendersByAcceptLanguage (4 langs), HomeCookieWinsOverHeader, SetLanguageSetsCookieAndRedirects, SetLanguageRejectsUnsupported |
