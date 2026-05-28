package shopping

import (
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

// recipeWith builds a recipe carrying only the ingredients a consolidation test
// cares about.
func recipeWith(ings ...domain.Ingredient) domain.Recipe {
	return domain.Recipe{Ingredients: ings}
}

// findItem returns the consolidated line for name (case-sensitive on the display
// name) or fails the test.
func findItem(t *testing.T, items []domain.ShoppingListItem, name string) domain.ShoppingListItem {
	t.Helper()
	for _, it := range items {
		if it.Name == name {
			return it
		}
	}
	t.Fatalf("no shopping line named %q in %+v", name, items)
	return domain.ShoppingListItem{}
}

func TestConsolidateSumsCompatibleUnits(t *testing.T) {
	items := Consolidate([]domain.Recipe{
		recipeWith(domain.Ingredient{Name: "carrot", Amount: 250, Unit: "g"}),
		recipeWith(domain.Ingredient{Name: "Carrot", Amount: 100, Unit: "g"}),
	}, nil)

	if len(items) != 1 {
		t.Fatalf("items = %d, want 1 (carrots merged)", len(items))
	}
	got := items[0]
	if got.Amount != 350 || got.Unit != "g" {
		t.Fatalf("carrot = %v %s, want 350 g", got.Amount, got.Unit)
	}
	if got.Category != domain.CategoryProduce {
		t.Fatalf("carrot category = %q, want produce", got.Category)
	}
}

func TestConsolidateConvertsWithinFamily(t *testing.T) {
	items := Consolidate([]domain.Recipe{
		recipeWith(domain.Ingredient{Name: "flour", Amount: 1, Unit: "kg"}),
		recipeWith(domain.Ingredient{Name: "flour", Amount: 200, Unit: "g"}),
	}, nil)

	got := findItem(t, items, "flour")
	if got.Amount != 1.2 || got.Unit != "kg" {
		t.Fatalf("flour = %v %s, want 1.2 kg", got.Amount, got.Unit)
	}
}

func TestConsolidateIncompatibleUnitsStaySeparate(t *testing.T) {
	items := Consolidate([]domain.Recipe{
		recipeWith(domain.Ingredient{Name: "onion", Amount: 1, Unit: "шт"}),
		recipeWith(domain.Ingredient{Name: "onion", Amount: 100, Unit: "g"}),
	}, nil)

	if len(items) != 2 {
		t.Fatalf("items = %d, want 2 (count vs mass not merged)", len(items))
	}
	var sawCount, sawMass bool
	for _, it := range items {
		if it.Name != "onion" {
			t.Fatalf("unexpected line %q", it.Name)
		}
		switch it.Unit {
		case "pcs":
			sawCount = it.Amount == 1
		case "g":
			sawMass = it.Amount == 100
		}
	}
	if !sawCount || !sawMass {
		t.Fatalf("expected separate 1 pcs and 100 g onion lines, got %+v", items)
	}
}

func TestConsolidateOpaqueUnitsMergeOnlyWhenIdentical(t *testing.T) {
	items := Consolidate([]domain.Recipe{
		recipeWith(domain.Ingredient{Name: "basil", Amount: 1, Unit: "pinch"}),
		recipeWith(domain.Ingredient{Name: "basil", Amount: 2, Unit: "pinch"}),
		recipeWith(domain.Ingredient{Name: "basil", Amount: 1, Unit: "clove"}),
	}, nil)

	var pinch, clove bool
	for _, it := range items {
		if it.Name != "basil" {
			continue
		}
		if it.Unit == "pinch" && it.Amount == 3 {
			pinch = true
		}
		if it.Unit == "clove" && it.Amount == 1 {
			clove = true
		}
	}
	if !pinch {
		t.Fatalf("expected merged 3 pinch basil, got %+v", items)
	}
	if !clove {
		t.Fatalf("expected separate 1 clove basil, got %+v", items)
	}
}

func TestConsolidateExcludesPantryBasics(t *testing.T) {
	items := Consolidate([]domain.Recipe{
		recipeWith(
			domain.Ingredient{Name: "Salt", Amount: 1, Unit: "tl"},
			domain.Ingredient{Name: "chicken breast", Amount: 500, Unit: "g"},
		),
	}, []string{"salt", "olive oil"})

	for _, it := range items {
		if it.Name == "Salt" {
			t.Fatalf("pantry basic salt should have been excluded: %+v", items)
		}
	}
	if len(items) != 1 || items[0].Name != "chicken breast" {
		t.Fatalf("items = %+v, want only chicken breast", items)
	}
}

func TestConsolidateLeavesUnknownCategoryEmpty(t *testing.T) {
	items := Consolidate([]domain.Recipe{
		recipeWith(domain.Ingredient{Name: "quinoa", Amount: 200, Unit: "g"}),
	}, nil)

	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].Category != "" {
		t.Fatalf("unknown ingredient category = %q, want empty (for caller to resolve)", items[0].Category)
	}
}

func TestConsolidateOrderIsDeterministic(t *testing.T) {
	recipes := []domain.Recipe{
		recipeWith(
			domain.Ingredient{Name: "salmon", Amount: 400, Unit: "g"},
			domain.Ingredient{Name: "carrot", Amount: 2, Unit: "шт"},
			domain.Ingredient{Name: "flour", Amount: 100, Unit: "g"},
		),
	}
	first := Consolidate(recipes, nil)
	second := Consolidate(recipes, nil)
	if len(first) != len(second) {
		t.Fatalf("length differs: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Name != second[i].Name || first[i].Category != second[i].Category {
			t.Fatalf("order not deterministic at %d: %q/%q vs %q/%q",
				i, first[i].Name, first[i].Category, second[i].Name, second[i].Category)
		}
	}
	// Ordered by category: meat_fish < pantry < produce lexicographically.
	if first[0].Category != domain.CategoryMeatFish {
		t.Fatalf("first category = %q, want meat_fish (sorted)", first[0].Category)
	}
}
