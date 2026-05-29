package service

import (
	"context"
	"errors"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/repository"
)

// fakeRecipeRepo is an in-memory recipeRepo for the recipe-service tests.
type fakeRecipeRepo struct {
	rows      map[string]*domain.Recipe
	getErr    error
	updateErr error
	updates   int
}

func newFakeRecipeRepo(recipes ...*domain.Recipe) *fakeRecipeRepo {
	rows := make(map[string]*domain.Recipe, len(recipes))
	for _, r := range recipes {
		rows[r.ID] = r
	}
	return &fakeRecipeRepo{rows: rows}
}

func (f *fakeRecipeRepo) GetRecipe(_ context.Context, id string) (*domain.Recipe, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	r, ok := f.rows[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return r, nil
}

func (f *fakeRecipeRepo) UpdateRecipe(_ context.Context, r *domain.Recipe) error {
	f.updates++
	if f.updateErr != nil {
		return f.updateErr
	}
	f.rows[r.ID] = r
	return nil
}

// Compile-time guard: the real store satisfies the service's narrow repo
// interface so the wiring stays honest if either side drifts.
var _ recipeRepo = (*repository.Store)(nil)

func TestSetFeedbackPersistsWithTimestamp(t *testing.T) {
	repo := newFakeRecipeRepo(&domain.Recipe{ID: "r1", Title: "Soup"})
	svc := NewRecipeService(repo)

	got, err := svc.SetFeedback(context.Background(), "r1", domain.Feedback{Liked: true})
	if err != nil {
		t.Fatalf("set feedback: %v", err)
	}
	if got.Feedback == nil {
		t.Fatal("feedback should be set")
	}
	if !got.Feedback.Liked || got.Feedback.Disliked || got.Feedback.CookAgain {
		t.Fatalf("flags = %+v, want only liked", got.Feedback)
	}
	if got.Feedback.CreatedAt.IsZero() {
		t.Error("expected a non-zero feedback timestamp")
	}
	if repo.rows["r1"].Feedback == nil || !repo.rows["r1"].Feedback.Liked {
		t.Error("feedback was not persisted to the repository")
	}
}

func TestSetFeedbackIsChangeableLater(t *testing.T) {
	repo := newFakeRecipeRepo(&domain.Recipe{
		ID:       "r1",
		Feedback: &domain.Feedback{Liked: true},
	})
	svc := NewRecipeService(repo)
	ctx := context.Background()

	// Change from liked to disliked + cook-again — feedback is not final.
	got, err := svc.SetFeedback(ctx, "r1", domain.Feedback{Disliked: true, CookAgain: true})
	if err != nil {
		t.Fatalf("set feedback: %v", err)
	}
	if got.Feedback.Liked {
		t.Error("liked should have been cleared")
	}
	if !got.Feedback.Disliked || !got.Feedback.CookAgain {
		t.Fatalf("flags = %+v, want disliked+cookAgain", got.Feedback)
	}
}

func TestSetFeedbackAllFalseClears(t *testing.T) {
	repo := newFakeRecipeRepo(&domain.Recipe{
		ID:       "r1",
		Feedback: &domain.Feedback{Liked: true, CookAgain: true},
	})
	svc := NewRecipeService(repo)

	got, err := svc.SetFeedback(context.Background(), "r1", domain.Feedback{})
	if err != nil {
		t.Fatalf("set feedback: %v", err)
	}
	if got.Feedback != nil {
		t.Errorf("feedback = %+v, want nil (cleared)", got.Feedback)
	}
	if repo.rows["r1"].Feedback != nil {
		t.Error("cleared feedback was not persisted")
	}
}

func TestSetFeedbackIsIdempotent(t *testing.T) {
	repo := newFakeRecipeRepo(&domain.Recipe{ID: "r1"})
	svc := NewRecipeService(repo)
	ctx := context.Background()

	fb := domain.Feedback{Liked: true}
	first, err := svc.SetFeedback(ctx, "r1", fb)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := svc.SetFeedback(ctx, "r1", fb)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	// Same absolute state applied twice leaves the same flags (SW-replay safe).
	if first.Feedback.Liked != second.Feedback.Liked ||
		second.Feedback.Disliked || second.Feedback.CookAgain {
		t.Fatalf("replay changed state: %+v -> %+v", first.Feedback, second.Feedback)
	}
}

func TestSetFeedbackNotFound(t *testing.T) {
	repo := newFakeRecipeRepo()
	svc := NewRecipeService(repo)

	_, err := svc.SetFeedback(context.Background(), "missing", domain.Feedback{Liked: true})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if repo.updates != 0 {
		t.Error("update should not run when the recipe is absent")
	}
}

func TestSetFeedbackUpdateErrorPropagates(t *testing.T) {
	sentinel := errors.New("db down")
	repo := newFakeRecipeRepo(&domain.Recipe{ID: "r1"})
	repo.updateErr = sentinel
	svc := NewRecipeService(repo)

	_, err := svc.SetFeedback(context.Background(), "r1", domain.Feedback{Liked: true})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wrapped db error", err)
	}
}
