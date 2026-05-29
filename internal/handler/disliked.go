package handler

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/service"
)

// dislikedProfiles is the slice of the household service the disliked-ingredients
// handlers need, kept narrow so they are testable with a stub. *HouseholdService
// satisfies it.
type dislikedProfiles interface {
	Current(ctx context.Context, defaultLang domain.Language) (*domain.HouseholdProfile, error)
	AddDisliked(ctx context.Context, id, term string) (*domain.HouseholdProfile, error)
	RemoveDisliked(ctx context.Context, id, term string) (*domain.HouseholdProfile, error)
}

// recipeHistory supplies recent recipes whose ingredient names seed the add-field
// autosuggest. *repository.Store satisfies it.
type recipeHistory interface {
	RecentRecipes(ctx context.Context, householdID string, limit int) ([]domain.Recipe, error)
}

// suggestionLimit bounds how many recent recipes feed the autosuggest datalist.
const suggestionLimit = 20

// dislikedHandlers serve the disliked-ingredients management screen (F-7 / CH-15):
// the current list plus add and remove actions. The list is persisted on the
// household profile and consumed as a hard constraint by every generation.
type dislikedHandlers struct {
	rd       *renderer
	profiles dislikedProfiles
	history  recipeHistory
}

// dislikedData is the view model for the disliked screen and its list fragment.
type dislikedData struct {
	Lang        string
	Items       []string
	Suggestions []string
	Error       string // i18n key of a validation error, empty when none
}

// Show renders the disliked-ingredients screen populated with the current list and
// an autosuggest datalist sourced from recent recipe ingredients. A failure to load
// history degrades to no suggestions rather than failing the page.
func (dh *dislikedHandlers) Show(w http.ResponseWriter, r *http.Request) {
	h, err := dh.profiles.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		dh.rd.fail(w, r, "load household for disliked", err)
		return
	}

	var suggestions []string
	if recipes, err := dh.history.RecentRecipes(r.Context(), h.ID, suggestionLimit); err == nil {
		suggestions = distinctIngredientSuggestions(recipes, h.DislikedIngredients)
	}

	dh.rd.render(w, r, "disliked", dislikedData{
		Lang:        string(LanguageFromContext(r.Context())),
		Items:       h.DislikedIngredients,
		Suggestions: suggestions,
	})
}

// Add handles POST /settings/disliked: it appends the submitted ingredient and
// re-renders the list fragment. A blank ingredient re-renders the screen with a
// localized error and HTTP 400 without persisting.
func (dh *dislikedHandlers) Add(w http.ResponseWriter, r *http.Request) {
	h, err := dh.profiles.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		dh.rd.fail(w, r, "load household for disliked add", err)
		return
	}

	updated, err := dh.profiles.AddDisliked(r.Context(), h.ID, r.FormValue("ingredient"))
	if err != nil {
		if errors.Is(err, service.ErrEmptyIngredient) {
			dh.rd.renderStatus(w, r, http.StatusBadRequest, "disliked", dislikedData{
				Lang:  string(LanguageFromContext(r.Context())),
				Items: h.DislikedIngredients,
				Error: "disliked.error_empty",
			})
			return
		}
		dh.rd.fail(w, r, "add disliked", err)
		return
	}

	dh.renderList(w, r, updated.DislikedIngredients)
}

// Remove handles POST /settings/disliked/remove: it drops the submitted ingredient
// (carried as a form value, since terms may contain spaces) and re-renders the
// list fragment.
func (dh *dislikedHandlers) Remove(w http.ResponseWriter, r *http.Request) {
	h, err := dh.profiles.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		dh.rd.fail(w, r, "load household for disliked remove", err)
		return
	}

	updated, err := dh.profiles.RemoveDisliked(r.Context(), h.ID, r.FormValue("ingredient"))
	if err != nil {
		dh.rd.fail(w, r, "remove disliked", err)
		return
	}

	dh.renderList(w, r, updated.DislikedIngredients)
}

// renderList renders the disliked/list fragment for the given items.
func (dh *dislikedHandlers) renderList(w http.ResponseWriter, r *http.Request, items []string) {
	dh.rd.renderFragment(w, r, http.StatusOK, "disliked/list", dislikedData{
		Lang:  string(LanguageFromContext(r.Context())),
		Items: items,
	})
}

// distinctIngredientSuggestions collects ingredient names from recipes, dropping
// duplicates (case-insensitive) and any name already on the disliked list, and
// returns them sorted for a stable datalist.
func distinctIngredientSuggestions(recipes []domain.Recipe, disliked []string) []string {
	excluded := make(map[string]struct{}, len(disliked))
	for _, d := range disliked {
		excluded[strings.ToLower(strings.TrimSpace(d))] = struct{}{}
	}

	seen := make(map[string]struct{})
	var names []string
	for _, recipe := range recipes {
		for _, ing := range recipe.Ingredients {
			name := strings.TrimSpace(ing.Name)
			key := strings.ToLower(name)
			if name == "" {
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			if _, skip := excluded[key]; skip {
				continue
			}
			seen[key] = struct{}{}
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
