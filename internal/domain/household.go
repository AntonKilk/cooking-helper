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
