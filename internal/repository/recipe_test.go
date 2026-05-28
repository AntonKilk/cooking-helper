package repository

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

func newTestHousehold(t *testing.T, store *Store) *domain.HouseholdProfile {
	t.Helper()
	h := &domain.HouseholdProfile{
		Language:   domain.LanguageEN,
		FamilySize: domain.FamilySize{Adults: 2, Kids: 1},
	}
	if err := store.CreateHousehold(context.Background(), h); err != nil {
		t.Fatalf("create household: %v", err)
	}
	return h
}

func TestRecipeCRUD(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	r := &domain.Recipe{
		HouseholdID:     h.ID,
		Language:        domain.LanguageEN,
		Title:           "Creamy Pasta",
		Description:     "Quick weeknight dish",
		CookTimeMinutes: 25,
		Servings:        4,
		Ingredients: []domain.Ingredient{
			{Name: "pasta", Amount: 400, Unit: "g", Category: domain.CategoryPantry},
			{Name: "cream", Amount: 2, Unit: "dl", Category: domain.CategoryDairy},
		},
		Steps:  []string{"Boil pasta", "Add cream"},
		Source: domain.SourceLLM,
	}
	if err := store.CreateRecipe(ctx, r); err != nil {
		t.Fatalf("create: %v", err)
	}
	if r.ID == "" {
		t.Fatal("expected generated ID")
	}

	got, err := store.GetRecipe(ctx, r.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != r.Title || got.Servings != r.Servings || got.Source != domain.SourceLLM {
		t.Fatalf("scalar mismatch: %+v", got)
	}
	if !reflect.DeepEqual(got.Ingredients, r.Ingredients) {
		t.Fatalf("ingredients = %v, want %v", got.Ingredients, r.Ingredients)
	}
	if !reflect.DeepEqual(got.Steps, r.Steps) {
		t.Fatalf("steps = %v, want %v", got.Steps, r.Steps)
	}
	if got.Feedback != nil {
		t.Fatalf("feedback = %+v, want nil", got.Feedback)
	}

	// Add feedback and update.
	fbTime := time.Date(2026, 5, 27, 18, 30, 0, 0, time.UTC)
	got.Feedback = &domain.Feedback{Liked: true, CookAgain: true, CreatedAt: fbTime}
	got.Title = "Creamy Pasta v2"
	if err := store.UpdateRecipe(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}

	reloaded, err := store.GetRecipe(ctx, r.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if reloaded.Title != "Creamy Pasta v2" {
		t.Fatalf("title = %q, want v2", reloaded.Title)
	}
	if reloaded.Feedback == nil {
		t.Fatal("expected feedback after update")
	}
	if !reloaded.Feedback.Liked || reloaded.Feedback.Disliked || !reloaded.Feedback.CookAgain {
		t.Fatalf("feedback flags = %+v", reloaded.Feedback)
	}
	if !reloaded.Feedback.CreatedAt.Equal(fbTime) {
		t.Fatalf("feedback time = %v, want %v", reloaded.Feedback.CreatedAt, fbTime)
	}

	if err := store.DeleteRecipe(ctx, r.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetRecipe(ctx, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete err = %v, want ErrNotFound", err)
	}
}

func TestRecentRecipesOrderingAndLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	// Insert oldest-to-newest; created_at is assigned per insert, so RecentRecipes
	// (ORDER BY created_at DESC) must return them newest-first.
	for _, title := range []string{"r1", "r2", "r3"} {
		r := &domain.Recipe{HouseholdID: h.ID, Language: domain.LanguageEN, Title: title, Source: domain.SourceLLM}
		if err := store.CreateRecipe(ctx, r); err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
	}

	all, err := store.RecentRecipes(ctx, h.ID, 10)
	if err != nil {
		t.Fatalf("recent (limit 10): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("recent len = %d, want 3", len(all))
	}
	if all[0].Title != "r3" || all[2].Title != "r1" {
		t.Fatalf("order = [%s,%s,%s], want newest-first [r3,r2,r1]", all[0].Title, all[1].Title, all[2].Title)
	}

	limited, err := store.RecentRecipes(ctx, h.ID, 2)
	if err != nil {
		t.Fatalf("recent (limit 2): %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("limited len = %d, want 2", len(limited))
	}
	if limited[0].Title != "r3" || limited[1].Title != "r2" {
		t.Fatalf("limited = [%s,%s], want [r3,r2]", limited[0].Title, limited[1].Title)
	}
}

func TestRecentRecipesScopedToHousehold(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	other := &domain.HouseholdProfile{Language: domain.LanguageEN, FamilySize: domain.FamilySize{Adults: 1}}
	if err := store.CreateHousehold(ctx, other); err != nil {
		t.Fatalf("create other household: %v", err)
	}
	if err := store.CreateRecipe(ctx, &domain.Recipe{HouseholdID: other.ID, Language: domain.LanguageEN, Title: "theirs", Source: domain.SourceLLM}); err != nil {
		t.Fatalf("create other recipe: %v", err)
	}

	got, err := store.RecentRecipes(ctx, h.ID, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("recent len = %d, want 0 (another household's recipe leaked)", len(got))
	}
}

func TestRecipesByIDsPreservesOrder(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	titles := []string{"a", "b", "c"}
	created := make([]*domain.Recipe, len(titles))
	for i, title := range titles {
		r := &domain.Recipe{HouseholdID: h.ID, Language: domain.LanguageEN, Title: title, Source: domain.SourceLLM}
		if err := store.CreateRecipe(ctx, r); err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		created[i] = r
	}

	// Request in a non-insertion order — result must preserve the request order.
	got, err := store.RecipesByIDs(ctx, []string{created[2].ID, created[0].ID, created[1].ID})
	if err != nil {
		t.Fatalf("recipes by ids: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Title != "c" || got[1].Title != "a" || got[2].Title != "b" {
		t.Fatalf("order = [%s,%s,%s], want [c,a,b]", got[0].Title, got[1].Title, got[2].Title)
	}
}

func TestRecipesByIDsMissingReturnsNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	r := &domain.Recipe{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "only", Source: domain.SourceLLM}
	if err := store.CreateRecipe(ctx, r); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := store.RecipesByIDs(ctx, []string{r.ID, "missing-id"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRecipesByIDsEmpty(t *testing.T) {
	store := newTestStore(t)
	got, err := store.RecipesByIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("recipes by ids: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestRecipeCascadeOnHouseholdDelete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	h := newTestHousehold(t, store)

	r := &domain.Recipe{HouseholdID: h.ID, Language: domain.LanguageEN, Title: "X", Source: domain.SourceHistory}
	if err := store.CreateRecipe(ctx, r); err != nil {
		t.Fatalf("create recipe: %v", err)
	}

	if err := store.DeleteHousehold(ctx, h.ID); err != nil {
		t.Fatalf("delete household: %v", err)
	}
	if _, err := store.GetRecipe(ctx, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("recipe should have cascaded, err = %v", err)
	}
}
