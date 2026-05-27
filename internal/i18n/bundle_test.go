package i18n_test

import (
	"testing"

	dict "github.com/AntonKilk/cooking-helper/i18n"
	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/i18n"
)

func loadBundle(t *testing.T) *i18n.Bundle {
	t.Helper()
	b, err := i18n.Load(dict.FS, domain.LanguageEN)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return b
}

func TestLoadAllLanguages(t *testing.T) {
	b := loadBundle(t)
	for _, lang := range []domain.Language{domain.LanguageRU, domain.LanguageFI, domain.LanguageEN} {
		if !b.Has(lang) {
			t.Errorf("missing dictionary for %q", lang)
		}
	}
	if b.Default() != domain.LanguageEN {
		t.Errorf("default = %q, want %q", b.Default(), domain.LanguageEN)
	}
}

func TestTranslateLocalizedChars(t *testing.T) {
	b := loadBundle(t)
	cases := []struct {
		lang domain.Language
		key  string
		want string
	}{
		{domain.LanguageFI, "category.frozen", "Pakasteet"},
		{domain.LanguageFI, "category.produce", "Vihannekset ja hedelmät"},
		{domain.LanguageRU, "category.produce", "Овощи и фрукты"},
		{domain.LanguageRU, "category.meat_fish", "Мясо и рыба"},
		{domain.LanguageEN, "category.dairy", "Dairy"},
	}
	for _, c := range cases {
		if got := b.Translator(c.lang)(c.key); got != c.want {
			t.Errorf("t(%q, %q) = %q, want %q", c.lang, c.key, got, c.want)
		}
	}
}

func TestTranslateWithArgs(t *testing.T) {
	b := loadBundle(t)
	if got := b.Translator(domain.LanguageRU)("greeting", "Антон"); got != "Привет, Антон" {
		t.Errorf("greeting = %q, want %q", got, "Привет, Антон")
	}
	if got := b.Translator(domain.LanguageEN)("greeting", "Anton"); got != "Hello, Anton" {
		t.Errorf("greeting = %q, want %q", got, "Hello, Anton")
	}
}

func TestTranslateFallback(t *testing.T) {
	b := loadBundle(t)
	if got := b.Translator(domain.LanguageFI)("does.not.exist"); got != "does.not.exist" {
		t.Errorf("unknown key = %q, want the key echoed", got)
	}
}

func TestLoadMissingDefault(t *testing.T) {
	if _, err := i18n.Load(dict.FS, domain.Language("xx")); err == nil {
		t.Fatal("expected error for missing default-language dictionary")
	}
}

func TestDetect(t *testing.T) {
	b := loadBundle(t)
	cases := []struct {
		name   string
		cookie string
		header string
		want   domain.Language
	}{
		{"cookie wins over header", "fi", "ru,en;q=0.8", domain.LanguageFI},
		{"header when no cookie", "", "ru-RU,ru;q=0.9,en;q=0.8", domain.LanguageRU},
		{"primary subtag match", "", "fi-FI", domain.LanguageFI},
		{"default when neither", "", "de,fr;q=0.7", domain.LanguageEN},
		{"invalid cookie ignored", "zz", "fi", domain.LanguageFI},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := i18n.Detect(c.cookie, c.header, b, domain.LanguageEN); got != c.want {
				t.Errorf("Detect(%q, %q) = %q, want %q", c.cookie, c.header, got, c.want)
			}
		})
	}
}
