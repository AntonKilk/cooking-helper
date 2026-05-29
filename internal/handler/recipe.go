package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/repository"
)

// recipeReader is the slice of repository.Store the recipe handler needs, kept
// narrow so the handler is testable with a stub. *repository.Store satisfies it.
type recipeReader interface {
	GetRecipe(ctx context.Context, id string) (*domain.Recipe, error)
}

// feedbackSetter applies the household's like/dislike/cook-again reaction to a
// recipe and returns the updated recipe. *service.RecipeService satisfies it.
type feedbackSetter interface {
	SetFeedback(ctx context.Context, id string, fb domain.Feedback) (*domain.Recipe, error)
}

// recipeHandlers serve the fullscreen recipe view (F-4 / CH-11) and the recipe
// feedback collection (F-5 / CH-16).
type recipeHandlers struct {
	rd       *renderer
	recipes  recipeReader
	feedback feedbackSetter
}

// recipeData is the view model for the recipe screen. NotFound renders the
// localized 404 partial; Recipe carries the populated view for the happy path.
type recipeData struct {
	Lang     string
	NotFound bool
	Recipe   *recipeView
}

// recipeView is the template-shaped projection of a domain.Recipe: only the
// fields the page renders, with sql/HTTP types kept out of the template.
type recipeView struct {
	ID          string
	Title       string
	Description string
	CookTime    int
	Servings    int
	Ingredients []ingredientView
	Steps       []string
	Feedback    feedbackView
}

// feedbackView is the template-facing projection of a recipe's feedback. It
// carries the recipe ID so the feedback fragment can post back, plus the three
// independent flags. A recipe with no feedback yet projects to all-false.
type feedbackView struct {
	RecipeID  string
	Liked     bool
	Disliked  bool
	CookAgain bool
}

// toFeedbackView projects a recipe's optional feedback into the template shape.
func toFeedbackView(r *domain.Recipe) feedbackView {
	v := feedbackView{RecipeID: r.ID}
	if r.Feedback != nil {
		v.Liked = r.Feedback.Liked
		v.Disliked = r.Feedback.Disliked
		v.CookAgain = r.Feedback.CookAgain
	}
	return v
}

// ingredientView is one line on the ingredients column. Amount is pre-formatted
// so the template never has to do numeric formatting.
type ingredientView struct {
	Name   string
	Amount string
	Unit   string
}

// Show renders the fullscreen recipe page. A blank id, or a repository
// not-found, renders the localized 404 partial; any other repository error
// degrades to a generic 500 with the detail logged, never echoed to the client.
func (h *recipeHandlers) Show(w http.ResponseWriter, r *http.Request) {
	lang := string(LanguageFromContext(r.Context()))

	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		h.rd.renderStatus(w, r, http.StatusNotFound, "recipe", recipeData{Lang: lang, NotFound: true})
		return
	}

	rec, err := h.recipes.GetRecipe(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.rd.renderStatus(w, r, http.StatusNotFound, "recipe", recipeData{Lang: lang, NotFound: true})
			return
		}
		h.rd.fail(w, r, "load recipe", err)
		return
	}

	h.rd.render(w, r, "recipe", recipeData{Lang: lang, Recipe: toRecipeView(rec)})
}

// toRecipeView projects a domain.Recipe into the template-facing shape.
func toRecipeView(r *domain.Recipe) *recipeView {
	ings := make([]ingredientView, len(r.Ingredients))
	for i, ing := range r.Ingredients {
		ings[i] = ingredientView{
			Name:   ing.Name,
			Amount: formatAmount(ing.Amount),
			Unit:   ing.Unit,
		}
	}
	return &recipeView{
		ID:          r.ID,
		Title:       r.Title,
		Description: r.Description,
		CookTime:    r.CookTimeMinutes,
		Servings:    r.Servings,
		Ingredients: ings,
		Steps:       r.Steps,
		Feedback:    toFeedbackView(r),
	}
}

// Feedback handles POST /recipe/{id}/feedback: it persists the absolute
// like/dislike/cook-again state carried in the request and re-renders the
// feedback control fragment. The flags are independent and the write is
// idempotent, so a replayed offline write is safe. A recipe that no longer
// exists is treated as a benign no-op (the write may have been queued before the
// recipe was removed) rather than an error.
func (h *recipeHandlers) Feedback(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	fb := domain.Feedback{
		Liked:     r.FormValue("liked") == "true",
		Disliked:  r.FormValue("disliked") == "true",
		CookAgain: r.FormValue("cook_again") == "true",
	}

	rec, err := h.feedback.SetFeedback(r.Context(), id, fb)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.rd.fail(w, r, "set recipe feedback", err)
		return
	}
	h.rd.renderFragment(w, r, http.StatusOK, "recipe/feedback", toFeedbackView(rec))
}

// formatAmount renders a float ingredient amount without trailing zeros so
// "250" stays "250" and "0.5" stays "0.5". Zero amounts collapse to "" so the
// template can render just the unit (e.g. "pinch").
func formatAmount(a float64) string {
	if a == 0 {
		return ""
	}
	return strconv.FormatFloat(a, 'f', -1, 64)
}
