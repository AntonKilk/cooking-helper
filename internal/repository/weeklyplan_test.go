package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

func TestWeeklyPlanCRUDWithItems(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	weekStart := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	p := &domain.WeeklyPlan{
		HouseholdID: h.ID,
		WeekStart:   weekStart,
		RecipeIDs:   []string{"r1", "r2", "r3"},
		ShoppingList: []domain.ShoppingListItem{
			{Name: "pasta", Amount: 400, Unit: "g", Category: domain.CategoryPantry},
			{Name: "cream", Amount: 2, Unit: "dl", Category: domain.CategoryDairy, Checked: true},
		},
	}
	if err := store.CreateWeeklyPlan(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected generated plan ID")
	}
	for i, item := range p.ShoppingList {
		if item.ID == "" {
			t.Fatalf("item %d missing generated ID", i)
		}
	}

	got, err := store.GetWeeklyPlan(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.WeekStart.Equal(weekStart) {
		t.Fatalf("week_start = %v, want %v", got.WeekStart, weekStart)
	}
	if len(got.RecipeIDs) != 3 {
		t.Fatalf("recipe_ids = %v, want 3", got.RecipeIDs)
	}
	if len(got.ShoppingList) != 2 {
		t.Fatalf("shopping list len = %d, want 2", len(got.ShoppingList))
	}
	if got.ShoppingList[0].Name != "pasta" || got.ShoppingList[1].Name != "cream" {
		t.Fatalf("shopping order/content wrong: %+v", got.ShoppingList)
	}
	if !got.ShoppingList[1].Checked {
		t.Fatal("expected cream item to be checked")
	}
	if got.ShoppingList[0].Category != domain.CategoryPantry {
		t.Fatalf("category = %q, want pantry", got.ShoppingList[0].Category)
	}

	if err := store.DeleteWeeklyPlan(ctx, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetWeeklyPlan(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete err = %v, want ErrNotFound", err)
	}
	// Items must have cascaded.
	items, err := store.listShoppingItems(ctx, p.ID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items after delete = %d, want 0", len(items))
	}
}

func TestWeeklyPlanCascadeOnHouseholdDelete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	p := &domain.WeeklyPlan{
		HouseholdID:  h.ID,
		WeekStart:    time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
		RecipeIDs:    []string{"r1"},
		ShoppingList: []domain.ShoppingListItem{{Name: "salt", Amount: 1, Unit: "pkt", Category: domain.CategoryPantry}},
	}
	if err := store.CreateWeeklyPlan(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := store.DeleteHousehold(ctx, h.ID); err != nil {
		t.Fatalf("delete household: %v", err)
	}
	if _, err := store.GetWeeklyPlan(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("plan should have cascaded, err = %v", err)
	}
	items, err := store.listShoppingItems(ctx, p.ID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items after cascade = %d, want 0", len(items))
	}
}
