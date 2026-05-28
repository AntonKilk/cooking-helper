package shopping

import (
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

// fixtureItem is one ingredient name from the test corpus with the store category
// it should ultimately land in.
type fixtureItem struct {
	Name string
	Want domain.IngredientCategory
}

// weekFixtures is a corpus drawn from five representative weeks across RU / FI /
// EN. It exercises the dictionary against names it should place; the service
// layer owns the matching ≥95%-accuracy pipeline test (CH-12 AC).
var weekFixtures = [][]fixtureItem{
	{ // Week 1 — EN roast dinner
		{"chicken breast", domain.CategoryMeatFish},
		{"potatoes", domain.CategoryProduce},
		{"carrots", domain.CategoryProduce},
		{"butter", domain.CategoryDairy},
		{"plain flour", domain.CategoryPantry},
		{"frozen peas", domain.CategoryFrozen},
	},
	{ // Week 2 — FI weeknight
		{"jauheliha", domain.CategoryMeatFish},
		{"sipuli", domain.CategoryProduce},
		{"tomaatti", domain.CategoryProduce},
		{"juusto", domain.CategoryDairy},
		{"riisi", domain.CategoryPantry},
		{"oliiviöljy", domain.CategoryPantry},
	},
	{ // Week 3 — RU comfort food
		{"куриное филе", domain.CategoryMeatFish},
		{"морковь", domain.CategoryProduce},
		{"картофель", domain.CategoryProduce},
		{"молоко", domain.CategoryDairy},
		{"мука пшеничная", domain.CategoryPantry},
		{"соль", domain.CategoryPantry},
	},
	{ // Week 4 — fish + dairy mix
		{"salmon fillet", domain.CategoryMeatFish},
		{"lohi", domain.CategoryMeatFish},
		{"лосось", domain.CategoryMeatFish},
		{"kerma", domain.CategoryDairy},
		{"сметана", domain.CategoryDairy},
		{"spinach", domain.CategoryProduce},
	},
	{ // Week 5 — pantry-heavy bake
		{"sugar", domain.CategoryPantry},
		{"sokeri", domain.CategoryPantry},
		{"baking powder", domain.CategoryPantry},
		{"eggs", domain.CategoryDairy},
		{"munat", domain.CategoryDairy},
		{"dark chocolate", domain.CategoryPantry},
	},
}

func TestLookupCategoryNoFalseHits(t *testing.T) {
	// Whenever the dictionary returns a category, it must be the correct one — a
	// wrong hit is worse than a miss (a miss defers to the LLM).
	hits, total := 0, 0
	for _, week := range weekFixtures {
		for _, item := range week {
			total++
			got, ok := LookupCategory(item.Name)
			if !ok {
				continue
			}
			hits++
			if got != item.Want {
				t.Errorf("LookupCategory(%q) = %q, want %q", item.Name, got, item.Want)
			}
		}
	}
	// Sanity floor: the dictionary should place the clear majority on its own so
	// the LLM fallback stays cheap. This is not the 95% AC (that is the full
	// pipeline, asserted in the service test) — just a regression guard.
	if hits*100 < total*70 {
		t.Fatalf("dictionary coverage = %d/%d (<70%%); fixture or dictionary regressed", hits, total)
	}
	t.Logf("dictionary coverage: %d/%d", hits, total)
}

func TestLookupCategorySpecificCases(t *testing.T) {
	cases := []struct {
		name string
		want domain.IngredientCategory
		ok   bool
	}{
		{"Fresh Mushrooms", domain.CategoryProduce, true},
		{"грибы", domain.CategoryProduce, true},
		{"sienet", domain.CategoryProduce, true},
		{"naudan jauheliha", domain.CategoryMeatFish, true},
		{"crème fraîche", domain.CategoryDairy, true},
		{"olive oil", domain.CategoryPantry, true},
		{"pakaste marjat", domain.CategoryFrozen, true},
		{"unobtainium", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := LookupCategory(c.name)
			if ok != c.ok || got != c.want {
				t.Fatalf("LookupCategory(%q) = (%q, %v), want (%q, %v)", c.name, got, ok, c.want, c.ok)
			}
		})
	}
}
