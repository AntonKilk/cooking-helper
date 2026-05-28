package repository

import (
	"context"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

func TestIngredientCategoryRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.SaveCategory(ctx, "carrot", domain.CategoryProduce); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := store.SaveCategory(ctx, "salmon", domain.CategoryMeatFish); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.CategoriesByNames(ctx, []string{"carrot", "salmon", "missing"})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d categories, want 2 (missing absent): %+v", len(got), got)
	}
	if got["carrot"] != domain.CategoryProduce {
		t.Fatalf("carrot = %q, want produce", got["carrot"])
	}
	if got["salmon"] != domain.CategoryMeatFish {
		t.Fatalf("salmon = %q, want meat_fish", got["salmon"])
	}
	if _, ok := got["missing"]; ok {
		t.Fatal("missing name should be absent from the result")
	}
}

func TestCategoriesByNamesEmpty(t *testing.T) {
	store := newTestStore(t)
	got, err := store.CategoriesByNames(context.Background(), nil)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d, want empty", len(got))
	}
}

func TestSaveCategoryIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.SaveCategory(ctx, "carrot", domain.CategoryProduce); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// Re-saving the same name is a no-op and must not error; the first value wins.
	if err := store.SaveCategory(ctx, "carrot", domain.CategoryOther); err != nil {
		t.Fatalf("second save: %v", err)
	}

	got, err := store.CategoriesByNames(ctx, []string{"carrot"})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got["carrot"] != domain.CategoryProduce {
		t.Fatalf("carrot = %q, want produce (first-writer-wins)", got["carrot"])
	}
}
