package domain

import "time"

// IngredientCategory maps an ingredient to a store section for the shopping list.
type IngredientCategory string

const (
	CategoryProduce  IngredientCategory = "produce"
	CategoryMeatFish IngredientCategory = "meat_fish"
	CategoryDairy    IngredientCategory = "dairy"
	CategoryPantry   IngredientCategory = "pantry"
	CategoryFrozen   IngredientCategory = "frozen"
	CategoryOther    IngredientCategory = "other"
)

// RecipeSource records whether a recipe was freshly generated or replayed from history.
type RecipeSource string

const (
	SourceLLM     RecipeSource = "llm"
	SourceHistory RecipeSource = "history"
)

// Ingredient is one line of a recipe, with the unit kept as free text
// (g, ml, tl, rkl, dl, ...) and a store category for consolidation.
type Ingredient struct {
	Name     string
	Amount   float64
	Unit     string
	Category IngredientCategory
}

// Feedback is the household's reaction to a recipe. It is optional: a Recipe
// with no feedback yet has a nil *Feedback.
type Feedback struct {
	Liked     bool
	Disliked  bool
	CookAgain bool
	CreatedAt time.Time
}

// Recipe is a single dish with its ingredients and steps.
type Recipe struct {
	ID              string
	HouseholdID     string
	Language        Language
	Title           string
	Description     string
	CookTimeMinutes int
	Servings        int
	Ingredients     []Ingredient
	Steps           []string
	Source          RecipeSource
	Feedback        *Feedback
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
