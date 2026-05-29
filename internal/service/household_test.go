package service

import (
	"context"
	"errors"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/repository"
)

// fakeRepo is an in-memory householdRepo for service tests, no database needed.
type fakeRepo struct {
	rows   map[string]*domain.HouseholdProfile
	nextID int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: make(map[string]*domain.HouseholdProfile)}
}

func (f *fakeRepo) FirstHousehold(_ context.Context) (*domain.HouseholdProfile, error) {
	for _, h := range f.rows {
		return h, nil
	}
	return nil, repository.ErrNotFound
}

func (f *fakeRepo) CreateHousehold(_ context.Context, h *domain.HouseholdProfile) error {
	f.nextID++
	h.ID = string(rune('a' + f.nextID))
	f.rows[h.ID] = h
	return nil
}

func (f *fakeRepo) GetHousehold(_ context.Context, id string) (*domain.HouseholdProfile, error) {
	h, ok := f.rows[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return h, nil
}

func (f *fakeRepo) UpdateHousehold(_ context.Context, h *domain.HouseholdProfile) error {
	if _, ok := f.rows[h.ID]; !ok {
		return repository.ErrNotFound
	}
	f.rows[h.ID] = h
	return nil
}

func TestCurrentCreatesDefaults(t *testing.T) {
	repo := newFakeRepo()
	svc := NewHouseholdService(repo)

	h, err := svc.Current(context.Background(), domain.LanguageFI)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if h.Language != domain.LanguageFI {
		t.Fatalf("language = %q, want fi", h.Language)
	}
	if h.FamilySize.Adults != defaultAdults || h.FamilySize.Kids != defaultKids {
		t.Fatalf("family = %+v, want adults=%d kids=%d", h.FamilySize, defaultAdults, defaultKids)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(repo.rows))
	}
}

func TestCurrentIsIdempotent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewHouseholdService(repo)
	ctx := context.Background()

	first, err := svc.Current(ctx, domain.LanguageEN)
	if err != nil {
		t.Fatalf("first current: %v", err)
	}
	second, err := svc.Current(ctx, domain.LanguageRU)
	if err != nil {
		t.Fatalf("second current: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second id = %q, want %q (no duplicate)", second.ID, first.ID)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(repo.rows))
	}
}

func TestUpdateProfilePersists(t *testing.T) {
	repo := newFakeRepo()
	svc := NewHouseholdService(repo)
	ctx := context.Background()

	h, err := svc.Current(ctx, domain.LanguageEN)
	if err != nil {
		t.Fatalf("current: %v", err)
	}

	updated, err := svc.UpdateProfile(ctx, h.ID, domain.LanguageRU, 3, 2)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Language != domain.LanguageRU {
		t.Fatalf("language = %q, want ru", updated.Language)
	}
	if updated.FamilySize.Adults != 3 || updated.FamilySize.Kids != 2 {
		t.Fatalf("family = %+v, want adults=3 kids=2", updated.FamilySize)
	}
	if repo.rows[h.ID].FamilySize.Adults != 3 {
		t.Fatal("change was not persisted to the repository")
	}
}

func TestAddDisliked(t *testing.T) {
	repo := newFakeRepo()
	svc := NewHouseholdService(repo)
	ctx := context.Background()

	h, err := svc.Current(ctx, domain.LanguageEN)
	if err != nil {
		t.Fatalf("current: %v", err)
	}

	t.Run("new term is appended and persisted", func(t *testing.T) {
		got, err := svc.AddDisliked(ctx, h.ID, "  Mushrooms  ")
		if err != nil {
			t.Fatalf("add: %v", err)
		}
		if len(got.DislikedIngredients) != 1 || got.DislikedIngredients[0] != "Mushrooms" {
			t.Fatalf("disliked = %v, want [Mushrooms] (trimmed)", got.DislikedIngredients)
		}
		if len(repo.rows[h.ID].DislikedIngredients) != 1 {
			t.Fatal("add was not persisted to the repository")
		}
	})

	t.Run("case-insensitive duplicate is a no-op", func(t *testing.T) {
		got, err := svc.AddDisliked(ctx, h.ID, "mushrooms")
		if err != nil {
			t.Fatalf("add dup: %v", err)
		}
		if len(got.DislikedIngredients) != 1 {
			t.Fatalf("disliked = %v, want length 1 after duplicate", got.DislikedIngredients)
		}
	})

	t.Run("blank term is rejected", func(t *testing.T) {
		if _, err := svc.AddDisliked(ctx, h.ID, "   "); !errors.Is(err, ErrEmptyIngredient) {
			t.Fatalf("err = %v, want ErrEmptyIngredient", err)
		}
		if len(repo.rows[h.ID].DislikedIngredients) != 1 {
			t.Fatalf("list mutated to %v on blank input", repo.rows[h.ID].DislikedIngredients)
		}
	})
}

func TestRemoveDisliked(t *testing.T) {
	repo := newFakeRepo()
	svc := NewHouseholdService(repo)
	ctx := context.Background()

	h, err := svc.Current(ctx, domain.LanguageEN)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if _, err := svc.AddDisliked(ctx, h.ID, "Mushrooms"); err != nil {
		t.Fatalf("seed add: %v", err)
	}

	t.Run("absent term is a no-op", func(t *testing.T) {
		got, err := svc.RemoveDisliked(ctx, h.ID, "olives")
		if err != nil {
			t.Fatalf("remove absent: %v", err)
		}
		if len(got.DislikedIngredients) != 1 {
			t.Fatalf("disliked = %v, want length 1 unchanged", got.DislikedIngredients)
		}
	})

	t.Run("case-insensitive match removes and leaves empty list", func(t *testing.T) {
		got, err := svc.RemoveDisliked(ctx, h.ID, "MUSHROOMS")
		if err != nil {
			t.Fatalf("remove: %v", err)
		}
		if len(got.DislikedIngredients) != 0 {
			t.Fatalf("disliked = %v, want empty after removal", got.DislikedIngredients)
		}
		if len(repo.rows[h.ID].DislikedIngredients) != 0 {
			t.Fatal("removal was not persisted to the repository")
		}
	})
}

func TestUpdateProfileRejectsOutOfRange(t *testing.T) {
	repo := newFakeRepo()
	svc := NewHouseholdService(repo)
	ctx := context.Background()

	h, err := svc.Current(ctx, domain.LanguageEN)
	if err != nil {
		t.Fatalf("current: %v", err)
	}

	cases := []struct {
		name         string
		adults, kids int
	}{
		{"adults too low", 0, 0},
		{"adults too high", 7, 0},
		{"kids too high", 2, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := svc.UpdateProfile(ctx, h.ID, domain.LanguageEN, c.adults, c.kids); !errors.Is(err, ErrInvalidFamilySize) {
				t.Fatalf("err = %v, want ErrInvalidFamilySize", err)
			}
			if repo.rows[h.ID].FamilySize != (domain.FamilySize{Adults: defaultAdults, Kids: defaultKids}) {
				t.Fatalf("family mutated to %+v on invalid input", repo.rows[h.ID].FamilySize)
			}
		})
	}
}
