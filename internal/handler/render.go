package handler

import (
	"bytes"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/AntonKilk/cooking-helper/internal/i18n"
)

// noopT is the placeholder t() used at template-parse time. The real,
// language-bound translator is injected per request via renderer.render.
func noopT(key string, _ ...any) string { return key }

// ParseFuncMap is the FuncMap templates must be parsed with so that {{ t ... }}
// resolves at parse time. render then rebinds t to a per-request translator.
func ParseFuncMap() template.FuncMap {
	return template.FuncMap{"t": noopT}
}

// renderer executes templates with a request-scoped t() bound to the active
// language. Templates are parsed once; each request clones them and rebinds t.
type renderer struct {
	tmpl   *template.Template
	bundle *i18n.Bundle
}

// render executes the named page with a 200 status.
func (rd *renderer) render(w http.ResponseWriter, r *http.Request, page string, data any) {
	rd.renderStatus(w, r, http.StatusOK, page, data)
}

// renderStatus resolves page to a concrete template — "<page>/content" for an
// HTMX navigation (the fragment swapped into #content), "<page>/page" otherwise —
// and executes it into a buffer first so a mid-render failure does not emit a
// half-written page, then writes it as UTF-8 HTML with the given status code.
func (rd *renderer) renderStatus(w http.ResponseWriter, r *http.Request, status int, page string, data any) {
	lang := LanguageFromContext(r.Context())

	clone, err := rd.tmpl.Clone()
	if err != nil {
		rd.fail(w, r, "clone template", err)
		return
	}
	clone.Funcs(template.FuncMap{"t": rd.bundle.Translator(lang)})

	name := page + "/page"
	if isHTMXNavigation(r) {
		name = page + "/content"
	}

	var buf bytes.Buffer
	if err := clone.ExecuteTemplate(&buf, name, data); err != nil {
		rd.fail(w, r, "execute template", err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// renderFragment executes a specific named template (e.g. "generate/cards") with
// the active language bound to t(), buffering first so a mid-render failure never
// emits a half-written response. Used for HTMX partials that are not full
// page/content pairs.
func (rd *renderer) renderFragment(w http.ResponseWriter, r *http.Request, status int, name string, data any) {
	lang := LanguageFromContext(r.Context())

	clone, err := rd.tmpl.Clone()
	if err != nil {
		rd.fail(w, r, "clone template", err)
		return
	}
	clone.Funcs(template.FuncMap{"t": rd.bundle.Translator(lang)})

	var buf bytes.Buffer
	if err := clone.ExecuteTemplate(&buf, name, data); err != nil {
		rd.fail(w, r, "execute fragment", err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// isHTMXNavigation reports whether the request is an HTMX-driven navigation that
// should receive only the inner content fragment. History restores are excluded
// so the browser caches the full page, not a bare fragment.
func isHTMXNavigation(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true" &&
		r.Header.Get("HX-History-Restore-Request") != "true"
}

func (rd *renderer) fail(w http.ResponseWriter, r *http.Request, msg string, err error) {
	slog.Error("render failed", "msg", msg, "err", err, "request_id", RequestIDFromContext(r.Context()))
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
