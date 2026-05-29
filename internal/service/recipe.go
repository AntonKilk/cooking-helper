package service

import (
	"context"
	"fmt"
	"time"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

// recipeRepo is the subset of repository.Store the recipe service depends on,
// kept narrow so the service can be unit-tested with a fake. *repository.Store
// satisfies it.
type recipeRepo interface {
	GetRecipe(ctx context.Context, id string) (*domain.Recipe, error)
	UpdateRecipe(ctx context.Context, r *domain.Recipe) error
}

// RecipeService orchestrates recipe reads and writes that carry domain logic,
// currently the household's like / dislike / cook-again feedback (F-5).
type RecipeService struct {
	repo recipeRepo
}

// NewRecipeService returns a service backed by the given repository.
func NewRecipeService(repo recipeRepo) *RecipeService {
	return &RecipeService{repo: repo}
}

// SetFeedback applies the absolute like/dislike/cook-again state to a recipe and
// returns the updated recipe. The three flags are independent. Writing the
// absolute desired state (rather than toggling) keeps the operation idempotent,
// so a Service-Worker replay of an offline write is a safe no-op.
//
// When all three flags are false the recipe's feedback is cleared (nil), which
// the repository persists as all-NULL columns — "no opinion" is a valid, freely
// changeable state. Otherwise CreatedAt is stamped with the current time so the
// timestamp reflects the most recent reaction (CH-17 orders feedback by recency).
func (s *RecipeService) SetFeedback(ctx context.Context, recipeID string, fb domain.Feedback) (*domain.Recipe, error) {
	rec, err := s.repo.GetRecipe(ctx, recipeID)
	if err != nil {
		return nil, fmt.Errorf("set feedback: %w", err)
	}

	if !fb.Liked && !fb.Disliked && !fb.CookAgain {
		rec.Feedback = nil
	} else {
		fb.CreatedAt = time.Now().UTC()
		rec.Feedback = &fb
	}

	if err := s.repo.UpdateRecipe(ctx, rec); err != nil {
		return nil, fmt.Errorf("set feedback: %w", err)
	}
	return rec, nil
}
