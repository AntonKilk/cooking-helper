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

func TestCreateWeekWithRecipes(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	recipes := []domain.Recipe{
		{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "Chicken Pasta", Servings: 3, Source: domain.SourceLLM},
		{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "Beef Tacos", Servings: 3, Source: domain.SourceLLM},
		{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "Salmon Bowl", Servings: 3, Source: domain.SourceLLM},
	}
	p := &domain.WeeklyPlan{HouseholdID: h.ID, WeekStart: time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)}

	if err := store.CreateWeekWithRecipes(ctx, p, recipes); err != nil {
		t.Fatalf("create week: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected generated plan ID")
	}
	if len(p.RecipeIDs) != 3 {
		t.Fatalf("plan recipe_ids = %v, want 3", p.RecipeIDs)
	}
	for i := range recipes {
		if recipes[i].ID == "" {
			t.Fatalf("recipe %d missing generated ID", i)
		}
		if p.RecipeIDs[i] != recipes[i].ID {
			t.Fatalf("plan.RecipeIDs[%d] = %q, want %q", i, p.RecipeIDs[i], recipes[i].ID)
		}
		if _, err := store.GetRecipe(ctx, recipes[i].ID); err != nil {
			t.Fatalf("get recipe %d: %v", i, err)
		}
	}

	got, err := store.GetWeeklyPlan(ctx, p.ID)
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if len(got.RecipeIDs) != 3 || got.RecipeIDs[0] != recipes[0].ID {
		t.Fatalf("loaded recipe_ids = %v, want %v", got.RecipeIDs, p.RecipeIDs)
	}

	recent, err := store.RecentRecipes(ctx, h.ID, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("recent len = %d, want 3", len(recent))
	}
}

func TestCreateWeekWithRecipesRollsBackOnFailure(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	// Reserve a plan ID so the plan INSERT collides (duplicate PK) AFTER the
	// recipes are inserted in the same tx — exercising full rollback.
	existing := &domain.WeeklyPlan{HouseholdID: h.ID, WeekStart: time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)}
	if err := store.CreateWeeklyPlan(ctx, existing); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	recipes := []domain.Recipe{
		{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "Doomed One", Source: domain.SourceLLM},
		{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "Doomed Two", Source: domain.SourceLLM},
	}
	clash := &domain.WeeklyPlan{ID: existing.ID, HouseholdID: h.ID, WeekStart: time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)}

	if err := store.CreateWeekWithRecipes(ctx, clash, recipes); err == nil {
		t.Fatal("expected duplicate-plan error, got nil")
	}

	// The recipes from the failed tx must not have persisted.
	recent, err := store.RecentRecipes(ctx, h.ID, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(recent) != 0 {
		t.Fatalf("recent len = %d, want 0 (recipes should have rolled back)", len(recent))
	}
}

func TestCurrentWeeklyPlanIgnoresArchived(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	older := &domain.WeeklyPlan{HouseholdID: h.ID, WeekStart: time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)}
	newer := &domain.WeeklyPlan{HouseholdID: h.ID, WeekStart: time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)}
	if err := store.CreateWeeklyPlan(ctx, older); err != nil {
		t.Fatalf("create older: %v", err)
	}
	if err := store.CreateWeeklyPlan(ctx, newer); err != nil {
		t.Fatalf("create newer: %v", err)
	}

	got, err := store.CurrentWeeklyPlan(ctx, h.ID)
	if err != nil {
		t.Fatalf("current (newest active): %v", err)
	}
	if got.ID != newer.ID {
		t.Fatalf("current.ID = %q, want %q (newest)", got.ID, newer.ID)
	}

	// Archive both via the same write that creates a fresh plan, leaving exactly
	// one active row — the freshly created one.
	freshest := &domain.WeeklyPlan{HouseholdID: h.ID, WeekStart: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	if err := store.ArchiveAndCreateWeek(ctx, newer.ID, freshest, nil); err != nil {
		t.Fatalf("archive+create: %v", err)
	}

	got, err = store.CurrentWeeklyPlan(ctx, h.ID)
	if err != nil {
		t.Fatalf("current after archive: %v", err)
	}
	if got.ID != freshest.ID {
		t.Fatalf("current.ID = %q, want %q (freshest)", got.ID, freshest.ID)
	}

	// Confirm the archived row is loadable but flagged.
	archived, err := store.GetWeeklyPlan(ctx, newer.ID)
	if err != nil {
		t.Fatalf("get archived: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("archived plan ArchivedAt is nil")
	}
}

func TestCurrentWeeklyPlanNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	if _, err := store.CurrentWeeklyPlan(ctx, h.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestArchiveAndCreateWeekAtomicity(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	prev := &domain.WeeklyPlan{HouseholdID: h.ID, WeekStart: time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)}
	if err := store.CreateWeeklyPlan(ctx, prev); err != nil {
		t.Fatalf("create prev: %v", err)
	}

	// Force a failure: collide the new plan's ID with an already-existing one so
	// the INSERT inside the tx fails and the whole tx rolls back.
	collision := &domain.WeeklyPlan{ID: prev.ID, HouseholdID: h.ID, WeekStart: time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)}
	if err := store.ArchiveAndCreateWeek(ctx, prev.ID, collision, nil); err == nil {
		t.Fatal("expected error on PK collision")
	}

	// Previous plan must still be active (rollback restored the archived_at = NULL state).
	got, err := store.GetWeeklyPlan(ctx, prev.ID)
	if err != nil {
		t.Fatalf("get prev after rollback: %v", err)
	}
	if got.ArchivedAt != nil {
		t.Fatalf("prev archived = %v, want nil after rollback", got.ArchivedAt)
	}
}

func TestArchiveAndCreateWeekWithoutPrevious(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	recipes := []domain.Recipe{
		{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "A", Source: domain.SourceLLM},
		{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "B", Source: domain.SourceLLM},
	}
	p := &domain.WeeklyPlan{HouseholdID: h.ID, WeekStart: time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)}

	// Empty previousPlanID is the "first ever plan" case; must succeed.
	if err := store.ArchiveAndCreateWeek(ctx, "", p, recipes); err != nil {
		t.Fatalf("archive+create: %v", err)
	}
	if p.ID == "" || len(p.RecipeIDs) != 2 {
		t.Fatalf("plan not persisted: %+v", p)
	}
}

func TestSwapRecipeInPlanRotatesIDs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	recipes := []domain.Recipe{
		{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "First", Source: domain.SourceLLM},
		{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "Second", Source: domain.SourceLLM},
		{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "Third", Source: domain.SourceLLM},
	}
	p := &domain.WeeklyPlan{
		HouseholdID:  h.ID,
		WeekStart:    time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
		ShoppingList: []domain.ShoppingListItem{{Name: "salt", Amount: 1, Unit: "pkt", Category: domain.CategoryPantry}},
	}
	if err := store.CreateWeekWithRecipes(ctx, p, recipes); err != nil {
		t.Fatalf("seed week: %v", err)
	}

	replacement := &domain.Recipe{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "Replacement", Source: domain.SourceLLM}
	rebuilt := []domain.ShoppingListItem{
		{Name: "tortilla", Amount: 8, Unit: "pcs", Category: domain.CategoryPantry},
		{Name: "beef", Amount: 400, Unit: "g", Category: domain.CategoryMeatFish},
	}
	if err := store.SwapRecipeInPlan(ctx, p.ID, recipes[1].ID, replacement, rebuilt); err != nil {
		t.Fatalf("swap: %v", err)
	}
	if replacement.ID == "" {
		t.Fatal("expected replacement ID")
	}

	got, err := store.GetWeeklyPlan(ctx, p.ID)
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if len(got.RecipeIDs) != 3 {
		t.Fatalf("recipe_ids len = %d, want 3", len(got.RecipeIDs))
	}
	if got.RecipeIDs[0] != recipes[0].ID || got.RecipeIDs[1] != replacement.ID || got.RecipeIDs[2] != recipes[2].ID {
		t.Fatalf("recipe_ids = %v, want [%s,%s,%s]",
			got.RecipeIDs, recipes[0].ID, replacement.ID, recipes[2].ID)
	}
	// Old recipe row must still exist (archive history).
	if _, err := store.GetRecipe(ctx, recipes[1].ID); err != nil {
		t.Fatalf("old recipe should remain: %v", err)
	}
	// Shopping list must have been replaced with the rebuilt items.
	items, err := store.listShoppingItems(ctx, p.ID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("shopping items = %d, want 2 (rebuilt) after swap", len(items))
	}
	if items[0].Name != "tortilla" || items[1].Name != "beef" {
		t.Fatalf("rebuilt shopping items = %+v, want [tortilla, beef]", items)
	}
	if items[0].Category != domain.CategoryPantry {
		t.Fatalf("rebuilt item category = %q, want pantry", items[0].Category)
	}
}

func TestSwapRecipeInPlanNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	r := &domain.Recipe{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "Only", Source: domain.SourceLLM}
	p := &domain.WeeklyPlan{HouseholdID: h.ID, WeekStart: time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)}
	if err := store.CreateWeekWithRecipes(ctx, p, []domain.Recipe{*r}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	replacement := &domain.Recipe{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "X", Source: domain.SourceLLM}

	// Unknown plan id.
	if err := store.SwapRecipeInPlan(context.Background(), "missing-plan", p.RecipeIDs[0], replacement, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (missing plan)", err)
	}
	// Plan exists, but oldRecipeID is not in it.
	if err := store.SwapRecipeInPlan(context.Background(), p.ID, "missing-recipe", replacement, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (missing old recipe)", err)
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
