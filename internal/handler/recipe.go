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

// recipeHandlers serve the fullscreen recipe view (F-4 / CH-11).
type recipeHandlers struct {
	rd      *renderer
	recipes recipeReader
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
	}
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
