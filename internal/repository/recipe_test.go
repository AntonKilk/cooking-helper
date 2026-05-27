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
