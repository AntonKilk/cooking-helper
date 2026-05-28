package shopping

import (
	"sort"
	"strings"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

// keySep separates the parts of a consolidation key. A NUL byte can never appear
// in a normalized name or unit, so it is a safe, collision-proof delimiter.
const keySep = "\x00"

// agg accumulates one consolidated shopping line while folding recipe ingredients.
type agg struct {
	name     string  // first-seen display name (kept verbatim for the list)
	class    string  // measurement class, or "" for an opaque unit
	unit     string  // first-seen display unit (used only for opaque units)
	known    bool    // true when the unit belongs to a known measurement class
	quantity float64 // running total: canonical base for known units, raw otherwise
}

// Consolidate merges the ingredients of all recipes into one shopping list:
//   - Identical ingredients with compatible units are summed (250 g + 100 g = 350 g).
//   - The same ingredient in incompatible units (1 pc + 100 g) yields two lines.
//   - Any ingredient matching a pantry-basics term is dropped — the household
//     always has it on hand (US-7).
//
// Each line's Category comes from the built-in dictionary; ingredients the
// dictionary cannot place keep an empty Category for the caller to resolve (via
// cache / LLM). Output is ordered by category then name so the list is stable.
func Consolidate(recipes []domain.Recipe, pantryBasics []string) []domain.ShoppingListItem {
	order := make([]string, 0)
	byKey := make(map[string]*agg)

	for _, r := range recipes {
		for _, ing := range r.Ingredients {
			if isPantryBasic(ing.Name, pantryBasics) {
				continue
			}

			class, factor, known := classifyUnit(ing.Unit)
			var key string
			if known {
				key = Normalize(ing.Name) + keySep + class
			} else {
				key = Normalize(ing.Name) + keySep + "opaque" + keySep + normUnit(ing.Unit)
			}

			a := byKey[key]
			if a == nil {
				a = &agg{name: ing.Name, class: class, unit: ing.Unit, known: known}
				byKey[key] = a
				order = append(order, key)
			}
			if known {
				a.quantity += ing.Amount * factor
			} else {
				a.quantity += ing.Amount
			}
		}
	}

	items := make([]domain.ShoppingListItem, 0, len(order))
	for _, key := range order {
		a := byKey[key]
		var amount float64
		var unit string
		if a.known {
			amount, unit = displayAmount(a.class, a.quantity)
		} else {
			amount, unit = round2(a.quantity), a.unit
		}
		category, _ := LookupCategory(a.name)
		items = append(items, domain.ShoppingListItem{
			Name:     a.name,
			Amount:   amount,
			Unit:     unit,
			Category: category,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Category != items[j].Category {
			return items[i].Category < items[j].Category
		}
		return items[i].Name < items[j].Name
	})
	return items
}

// isPantryBasic reports whether name matches any non-empty pantry term, using the
// same inflection-tolerant matcher as the disliked-ingredient guard.
func isPantryBasic(name string, pantryBasics []string) bool {
	for _, term := range pantryBasics {
		if strings.TrimSpace(term) == "" {
			continue
		}
		if ContainsTerm(name, term) {
			return true
		}
	}
	return false
}
