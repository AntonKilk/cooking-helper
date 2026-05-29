package domain

import "time"

// Language is the UI/recipe language. Recipes keep the language they were
// created in; switching the UI language does not re-translate them.
type Language string

const (
	LanguageRU Language = "ru"
	LanguageFI Language = "fi"
	LanguageEN Language = "en"
)

// FamilySize describes how many people a weekly plan is portioned for.
type FamilySize struct {
	Adults int
	Kids   int
}

// DefaultPantryBasics returns the localized "always at home" staples seeded into
// a new household (PRD §15 Appendix). These are persisted data values matched
// against recipe ingredient names, not UI chrome, so they live here rather than
// in the i18n bundle. An unknown language falls back to English. Each call yields
// a fresh slice the caller may mutate.
func DefaultPantryBasics(lang Language) []string {
	switch lang {
	case LanguageRU:
		return []string{"соль", "чёрный перец", "растительное масло", "сливочное масло", "мука пшеничная", "сахар"}
	case LanguageFI:
		return []string{"suola", "mustapippuri", "kasviöljy", "voi", "vehnäjauho", "sokeri"}
	default:
		return []string{"salt", "black pepper", "vegetable oil", "butter", "wheat flour", "sugar"}
	}
}

// HouseholdProfile is the root entity. Every other record references it via
// HouseholdID, carrying a UUID so a future multi-user mode can partition data.
type HouseholdProfile struct {
	ID                  string
	Language            Language
	FamilySize          FamilySize
	DislikedIngredients []string
	PantryBasics        []string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
