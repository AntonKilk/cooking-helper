package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

// seedPlanWithItems creates a household and a weekly plan with two shopping-list
// items, returning the persisted plan (items carry their generated IDs).
func seedPlanWithItems(t *testing.T, store *Store) *domain.WeeklyPlan {
	t.Helper()
	h := newTestHousehold(t, store)
	p := &domain.WeeklyPlan{
		HouseholdID: h.ID,
		WeekStart:   time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
		RecipeIDs:   []string{"r1", "r2", "r3"},
		ShoppingList: []domain.ShoppingListItem{
			{Name: "pasta", Amount: 400, Unit: "g", Category: domain.CategoryPantry},
			{Name: "cream", Amount: 2, Unit: "dl", Category: domain.CategoryDairy},
		},
	}
	if err := store.CreateWeeklyPlan(context.Background(), p); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return p
}

func TestGetShoppingItem(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	p := seedPlanWithItems(t, store)

	got, err := store.GetShoppingItem(ctx, p.ShoppingList[0].ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "pasta" || got.Category != domain.CategoryPantry {
		t.Fatalf("item = %+v, want pasta/pantry", got)
	}
	if got.Checked || got.ManuallyRemoved {
		t.Fatalf("fresh item should be unchecked and present: %+v", got)
	}

	if _, err := store.GetShoppingItem(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing err = %v, want ErrNotFound", err)
	}
}

func TestSetShoppingItemCheckedIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	p := seedPlanWithItems(t, store)
	id := p.ShoppingList[0].ID

	// Applying the same checked=true twice is a no-op the second time (a replayed
	// offline write must not error or flip state).
	for i := 0; i < 2; i++ {
		if err := store.SetShoppingItemChecked(ctx, id, true); err != nil {
			t.Fatalf("set checked (call %d): %v", i+1, err)
		}
	}
	got, err := store.GetShoppingItem(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Checked {
		t.Fatal("expected item checked after double-apply")
	}

	if err := store.SetShoppingItemChecked(ctx, id, false); err != nil {
		t.Fatalf("uncheck: %v", err)
	}
	got, err = store.GetShoppingItem(ctx, id)
	if err != nil {
		t.Fatalf("get after uncheck: %v", err)
	}
	if got.Checked {
		t.Fatal("expected item unchecked")
	}

	if err := store.SetShoppingItemChecked(ctx, "missing", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("set checked missing err = %v, want ErrNotFound", err)
	}
}

func TestSetShoppingItemRemovedIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	p := seedPlanWithItems(t, store)
	id := p.ShoppingList[1].ID

	for i := 0; i < 2; i++ {
		if err := store.SetShoppingItemRemoved(ctx, id, true); err != nil {
			t.Fatalf("set removed (call %d): %v", i+1, err)
		}
	}
	got, err := store.GetShoppingItem(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.ManuallyRemoved {
		t.Fatal("expected item removed after double-apply")
	}

	// Restore.
	if err := store.SetShoppingItemRemoved(ctx, id, false); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err = store.GetShoppingItem(ctx, id)
	if err != nil {
		t.Fatalf("get after restore: %v", err)
	}
	if got.ManuallyRemoved {
		t.Fatal("expected item restored")
	}

	if err := store.SetShoppingItemRemoved(ctx, "missing", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("set removed missing err = %v, want ErrNotFound", err)
	}
}
