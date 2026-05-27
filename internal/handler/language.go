package handler

import (
	"context"
	"net/http"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/i18n"
)

const (
	// languageCookie stores the user's chosen UI language across requests.
	languageCookie = "lang"
	// languageKey is the context key under which the resolved language is stored.
	languageKey contextKey = "language"
	// languageCookieMaxAge keeps the choice for roughly one year.
	languageCookieMaxAge = 365 * 24 * 60 * 60
)

// LanguageFromContext returns the language resolved by languageMiddleware. It
// falls back to English if no middleware ran (e.g. in isolated handler tests).
func LanguageFromContext(ctx context.Context) domain.Language {
	if lang, ok := ctx.Value(languageKey).(domain.Language); ok {
		return lang
	}
	return domain.LanguageEN
}

// languageMiddleware resolves the active language from the session cookie or the
// Accept-Language header and stores it in the request context for downstream
// rendering.
func languageMiddleware(bundle *i18n.Bundle, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cookieVal string
		if c, err := r.Cookie(languageCookie); err == nil {
			cookieVal = c.Value
		}
		lang := i18n.Detect(cookieVal, r.Header.Get("Accept-Language"), bundle, bundle.Default())
		ctx := context.WithValue(r.Context(), languageKey, lang)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SetLanguage handles the language switcher: it validates the requested language,
// persists it in a cookie, and redirects back so the page re-renders.
func SetLanguage(bundle *i18n.Bundle) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := domain.Language(r.FormValue("lang"))
		if !bundle.Has(lang) {
			http.Error(w, "unsupported language", http.StatusBadRequest)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     languageCookie,
			Value:    string(lang),
			Path:     "/",
			MaxAge:   languageCookieMaxAge,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(w, r, redirectTarget(r), http.StatusSeeOther)
	}
}

// redirectTarget returns a safe same-origin path to redirect to after a language
// change, defaulting to the home page. Only same-origin relative referers are
// honored to avoid open-redirects.
func redirectTarget(r *http.Request) string {
	ref := r.Referer()
	if ref == "" {
		return "/"
	}
	if len(ref) > 1 && ref[0] == '/' && ref[1] != '/' {
		return ref
	}
	return "/"
}
