package domain

import "time"

// WeeklyPlan is a generated menu: three recipes portioned for a week of eating,
// plus the consolidated shopping list derived from them.
type WeeklyPlan struct {
	ID           string
	HouseholdID  string
	WeekStart    time.Time
	RecipeIDs    []string
	ShoppingList []ShoppingListItem
	CreatedAt    time.Time
}

// ShoppingListItem is one consolidated, store-categorized line of a plan's
// shopping list. ManuallyRemoved keeps a line that the household crossed out
// without deleting the record, so regeneration can respect that choice.
type ShoppingListItem struct {
	ID              string
	Name            string
	Amount          float64
	Unit            string
	Category        IngredientCategory
	Checked         bool
	ManuallyRemoved bool
}
