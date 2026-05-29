package repository

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

func TestHouseholdCRUD(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	h := &domain.HouseholdProfile{
		Language:            domain.LanguageRU,
		FamilySize:          domain.FamilySize{Adults: 2, Kids: 2},
		DislikedIngredients: []string{"кинза", "оливки"},
		PantryBasics:        []string{"соль", "сахар"},
	}
	if err := store.CreateHousehold(ctx, h); err != nil {
		t.Fatalf("create: %v", err)
	}
	if h.ID == "" {
		t.Fatal("expected generated ID")
	}
	if h.CreatedAt.IsZero() || h.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to be set")
	}

	got, err := store.GetHousehold(ctx, h.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Language != domain.LanguageRU {
		t.Fatalf("language = %q, want %q", got.Language, domain.LanguageRU)
	}
	if !reflect.DeepEqual(got.DislikedIngredients, h.DislikedIngredients) {
		t.Fatalf("disliked = %v, want %v", got.DislikedIngredients, h.DislikedIngredients)
	}
	if !reflect.DeepEqual(got.PantryBasics, h.PantryBasics) {
		t.Fatalf("pantry = %v, want %v", got.PantryBasics, h.PantryBasics)
	}
	if got.FamilySize != h.FamilySize {
		t.Fatalf("family = %+v, want %+v", got.FamilySize, h.FamilySize)
	}

	got.Language = domain.LanguageFI
	got.DislikedIngredients = []string{"korianteri"}
	if err := store.UpdateHousehold(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	reloaded, err := store.GetHousehold(ctx, h.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if reloaded.Language != domain.LanguageFI {
		t.Fatalf("language = %q, want %q", reloaded.Language, domain.LanguageFI)
	}
	if !reflect.DeepEqual(reloaded.DislikedIngredients, []string{"korianteri"}) {
		t.Fatalf("disliked = %v, want [korianteri]", reloaded.DislikedIngredients)
	}

	if err := store.DeleteHousehold(ctx, h.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetHousehold(ctx, h.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete err = %v, want ErrNotFound", err)
	}
}

func TestFirstHousehold(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.FirstHousehold(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("first on empty store err = %v, want ErrNotFound", err)
	}

	h := &domain.HouseholdProfile{
		Language:   domain.LanguageEN,
		FamilySize: domain.FamilySize{Adults: 2, Kids: 1},
	}
	if err := store.CreateHousehold(ctx, h); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.FirstHousehold(ctx)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if got.ID != h.ID {
		t.Fatalf("id = %q, want %q", got.ID, h.ID)
	}
	if got.FamilySize != h.FamilySize {
		t.Fatalf("family = %+v, want %+v", got.FamilySize, h.FamilySize)
	}
}

func TestHouseholdOnboardedRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	h := &domain.HouseholdProfile{
		Language:   domain.LanguageEN,
		FamilySize: domain.FamilySize{Adults: 2, Kids: 0},
	}
	if err := store.CreateHousehold(ctx, h); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.GetHousehold(ctx, h.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Onboarded {
		t.Fatal("a fresh household should default to not onboarded")
	}

	got.Onboarded = true
	if err := store.UpdateHousehold(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	reloaded, err := store.GetHousehold(ctx, h.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if !reloaded.Onboarded {
		t.Fatal("onboarded flag did not persist")
	}
}

func TestHouseholdNotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetHousehold(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := store.UpdateHousehold(context.Background(), &domain.HouseholdProfile{ID: "missing", Language: domain.LanguageEN}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update err = %v, want ErrNotFound", err)
	}
	if err := store.DeleteHousehold(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete err = %v, want ErrNotFound", err)
	}
}
