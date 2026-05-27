package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

// supportedLanguages is the fixed set of UI languages the bundle loads.
var supportedLanguages = []domain.Language{
	domain.LanguageRU,
	domain.LanguageFI,
	domain.LanguageEN,
}

// Bundle holds the parsed dictionaries keyed by language and the default
// language used as a fallback when a key is missing in the requested language.
type Bundle struct {
	dicts       map[domain.Language]map[string]string
	defaultLang domain.Language
}

// Load reads <lang>.json for every supported language from fsys and returns a
// Bundle. The default language's dictionary is required; the others are loaded
// when present. JSON parse errors are returned wrapped.
func Load(fsys fs.FS, defaultLang domain.Language) (*Bundle, error) {
	dicts := make(map[domain.Language]map[string]string, len(supportedLanguages))
	for _, lang := range supportedLanguages {
		data, err := fs.ReadFile(fsys, string(lang)+".json")
		if err != nil {
			if lang == defaultLang {
				return nil, fmt.Errorf("load i18n %s: %w", lang, err)
			}
			continue
		}
		var dict map[string]string
		if err := json.Unmarshal(data, &dict); err != nil {
			return nil, fmt.Errorf("parse i18n %s: %w", lang, err)
		}
		dicts[lang] = dict
	}
	if _, ok := dicts[defaultLang]; !ok {
		return nil, fmt.Errorf("load i18n: default language %q has no dictionary", defaultLang)
	}
	return &Bundle{dicts: dicts, defaultLang: defaultLang}, nil
}

// Default returns the bundle's fallback language.
func (b *Bundle) Default() domain.Language { return b.defaultLang }

// Has reports whether the bundle has a dictionary for lang.
func (b *Bundle) Has(lang domain.Language) bool {
	_, ok := b.dicts[lang]
	return ok
}

// translate resolves key in lang, falling back to the default language and then
// to the key itself so a missing string is visible rather than blank. When args
// are supplied the resolved value is treated as a format string.
func (b *Bundle) translate(lang domain.Language, key string, args ...any) string {
	value, ok := b.lookup(lang, key)
	if !ok {
		value, ok = b.lookup(b.defaultLang, key)
	}
	if !ok {
		slog.Warn("i18n missing key", "key", key, "lang", lang)
		value = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(value, args...)
	}
	return value
}

func (b *Bundle) lookup(lang domain.Language, key string) (string, bool) {
	dict, ok := b.dicts[lang]
	if !ok {
		return "", false
	}
	value, ok := dict[key]
	return value, ok
}

// Translator returns a t(key, args...) function bound to lang, suitable for
// registering in an html/template FuncMap per request.
func (b *Bundle) Translator(lang domain.Language) func(key string, args ...any) string {
	return func(key string, args ...any) string {
		return b.translate(lang, key, args...)
	}
}

// Detect resolves the active language: a valid, supported cookie value wins;
// otherwise the first supported tag in the Accept-Language header is used;
// otherwise def is returned.
func Detect(cookie, acceptLanguage string, b *Bundle, def domain.Language) domain.Language {
	if lang := domain.Language(strings.TrimSpace(cookie)); b.Has(lang) {
		return lang
	}
	if lang, ok := parseAcceptLanguage(acceptLanguage, b); ok {
		return lang
	}
	return def
}

// parseAcceptLanguage returns the first supported language named in an
// Accept-Language header, matching on the primary subtag (e.g. "fi-FI" → "fi").
func parseAcceptLanguage(header string, b *Bundle) (domain.Language, bool) {
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(part)
		if i := strings.IndexByte(tag, ';'); i >= 0 {
			tag = tag[:i]
		}
		if i := strings.IndexByte(tag, '-'); i >= 0 {
			tag = tag[:i]
		}
		lang := domain.Language(strings.ToLower(strings.TrimSpace(tag)))
		if b.Has(lang) {
			return lang, true
		}
	}
	return "", false
}
