package service

// The generated* types decode the LLM's JSON reply. They are service-internal:
// the protein tag and raw category strings are transient, mapped to domain types
// (or dropped) by toDomainRecipes — they never reach the persistence layer as-is.

type generatedIngredient struct {
	Name     string  `json:"name"`
	Amount   float64 `json:"amount"`
	Unit     string  `json:"unit"`
	Category string  `json:"category"`
}

type generatedRecipe struct {
	Title           string                `json:"title"`
	Description     string                `json:"description"`
	CookTimeMinutes int                   `json:"cook_time_minutes"`
	Servings        int                   `json:"servings"`
	Protein         string                `json:"protein"`
	Ingredients     []generatedIngredient `json:"ingredients"`
	Steps           []string              `json:"steps"`
}

type generatedWeek struct {
	Recipes []generatedRecipe `json:"recipes"`
}

// generatedSwap is the single-recipe reply for a swap call.
type generatedSwap struct {
	Recipe generatedRecipe `json:"recipe"`
}
