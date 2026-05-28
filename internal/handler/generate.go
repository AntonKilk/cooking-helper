package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/service"
)

// weekGenerator is the slice of the generation service the handler needs, kept
// narrow so the handler is testable with a stub. *service.GenerationService
// satisfies it.
type weekGenerator interface {
	GenerateWeek(ctx context.Context, h *domain.HouseholdProfile) (*service.GeneratedWeek, error)
	CurrentPlan(ctx context.Context, householdID string) (*domain.WeeklyPlan, error)
	SwapRecipe(ctx context.Context, h *domain.HouseholdProfile, plan *domain.WeeklyPlan, oldRecipeID string) (*service.SwappedRecipe, error)
}

// recipeLoader loads recipes by ID for the swap handler to reconstruct kept
// cards. *repository.Store satisfies it.
type recipeLoader interface {
	RecipesByIDs(ctx context.Context, ids []string) ([]domain.Recipe, error)
}

// generateHandlers serve the one-tap weekly generation and per-card swap.
type generateHandlers struct {
	rd         *renderer
	households householdProfiles
	gen        weekGenerator
	recipes    recipeLoader
}

// recipeCard is the per-recipe view model for the home cards.
type recipeCard struct {
	ID          string
	Title       string
	Description string
	CookTime    int
	Emoji       string
}

// cardsData is the view model for the generated-week fragment.
type cardsData struct {
	Cards []recipeCard
}

// errorData carries the i18n key of a friendly generation error.
type errorData struct {
	MessageKey string
}

// proteinEmoji maps a protein category to a language-neutral emoji for the card.
var proteinEmoji = map[string]string{
	"poultry":    "🍗",
	"red_meat":   "🥩",
	"pork":       "🐷",
	"fish":       "🐟",
	"seafood":    "🦐",
	"vegetarian": "🥬",
	"other":      "🍽",
}

func emojiFor(protein string) string {
	if e, ok := proteinEmoji[protein]; ok {
		return e
	}
	return proteinEmoji["other"]
}

// Generate handles POST /generate: it loads the household, asks the generation
// service for a weekly plan, and renders the three recipe cards. Hard-constraint
// failures and provider errors render a friendly localized fragment (HTTP 200 so
// HTMX swaps it); internal detail is logged, never sent to the client.
func (gh *generateHandlers) Generate(w http.ResponseWriter, r *http.Request) {
	h, err := gh.households.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		gh.rd.fail(w, r, "load household for generation", err)
		return
	}

	week, err := gh.gen.GenerateWeek(r.Context(), h)
	if err != nil {
		gh.renderError(w, r, err)
		return
	}

	cards := make([]recipeCard, len(week.Recipes))
	for i, rec := range week.Recipes {
		cards[i] = recipeCard{
			ID:          rec.ID,
			Title:       rec.Title,
			Description: rec.Description,
			CookTime:    rec.CookTimeMinutes,
			Emoji:       emojiFor(week.Proteins[i]),
		}
	}
	gh.rd.renderFragment(w, r, http.StatusOK, "generate/cards", cardsData{Cards: cards})
}

// Swap handles POST /generate/swap/{recipeID}: replaces one recipe in the
// household's current plan, re-renders the 3-card fragment. The kept cards are
// reconstructed from the rotated plan.RecipeIDs plus the freshly inserted
// recipe; emoji for kept cards comes from a keyword inference because CH-8 did
// not store protein on Recipe. Hard-constraint failures and provider errors
// render the same localized error partial as Generate.
func (gh *generateHandlers) Swap(w http.ResponseWriter, r *http.Request) {
	oldID := strings.TrimSpace(r.PathValue("recipeID"))
	if oldID == "" {
		gh.renderError(w, r, errors.New("swap: missing recipe id"))
		return
	}

	h, err := gh.households.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		gh.rd.fail(w, r, "load household for swap", err)
		return
	}

	plan, err := gh.gen.CurrentPlan(r.Context(), h.ID)
	if err != nil {
		gh.rd.fail(w, r, "load current plan for swap", err)
		return
	}
	if plan == nil {
		gh.renderError(w, r, errors.New("swap: no active plan"))
		return
	}

	swapped, err := gh.gen.SwapRecipe(r.Context(), h, plan, oldID)
	if err != nil {
		gh.renderError(w, r, err)
		return
	}

	// Rebuild the 3 cards in their plan order: keep the new recipe in place,
	// reload the kept ones for their titles/descriptions/cook times.
	keptIDs := make([]string, 0, len(swapped.Plan.RecipeIDs)-1)
	for _, id := range swapped.Plan.RecipeIDs {
		if id != swapped.Recipe.ID {
			keptIDs = append(keptIDs, id)
		}
	}
	kept, err := gh.recipes.RecipesByIDs(r.Context(), keptIDs)
	if err != nil {
		gh.rd.fail(w, r, "load kept recipes after swap", err)
		return
	}
	byID := make(map[string]domain.Recipe, len(kept))
	for _, rec := range kept {
		byID[rec.ID] = rec
	}

	cards := make([]recipeCard, len(swapped.Plan.RecipeIDs))
	for i, id := range swapped.Plan.RecipeIDs {
		if id == swapped.Recipe.ID {
			cards[i] = recipeCard{
				ID:          swapped.Recipe.ID,
				Title:       swapped.Recipe.Title,
				Description: swapped.Recipe.Description,
				CookTime:    swapped.Recipe.CookTimeMinutes,
				Emoji:       emojiFor(swapped.Protein),
			}
			continue
		}
		rec := byID[id]
		cards[i] = recipeCard{
			ID:          rec.ID,
			Title:       rec.Title,
			Description: rec.Description,
			CookTime:    rec.CookTimeMinutes,
			Emoji:       emojiFor(inferProteinFromRecipe(rec)),
		}
	}
	gh.rd.renderFragment(w, r, http.StatusOK, "generate/cards", cardsData{Cards: cards})
}

// inferProteinFromRecipe makes a best-effort protein-bucket guess from a
// stored recipe's ingredient names so the kept-card emoji stays meaningful
// after a swap. Returns "" when no known keyword is found — the caller falls
// through to the "other 🍽" emoji.
func inferProteinFromRecipe(r domain.Recipe) string {
	for _, ing := range r.Ingredients {
		name := strings.ToLower(ing.Name)
		switch {
		case containsAny(name, "chicken", "kana", "курица", "куриц",
			"turkey", "kalkkuna", "индейк"):
			return "poultry"
		case containsAny(name, "beef", "nauda", "naudan", "говядин",
			"lamb", "lammas", "lampaan", "ягнят", "баранин"):
			return "red_meat"
		case containsAny(name, "pork", "sika", "porsaan", "сви"):
			return "pork"
		case containsAny(name, "salmon", "lohi", "лосось", "сёмг", "семг",
			"tuna", "tonnikala", "тунец",
			"cod", "turska", "треск"):
			return "fish"
		case containsAny(name, "shrimp", "katkarapu", "креветк"):
			return "seafood"
		case containsAny(name, "tofu", "tofua", "тофу"):
			return "vegetarian"
		}
	}
	return ""
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// renderError logs the real error and renders the localized error partial. Known
// hard-constraint failures get a specific message; anything else is generic.
func (gh *generateHandlers) renderError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Warn("week generation failed",
		"err", err,
		"request_id", RequestIDFromContext(r.Context()),
	)
	gh.rd.renderFragment(w, r, http.StatusOK, "generate/error", errorData{MessageKey: errorMessageKey(err)})
}

// errorMessageKey maps a generation error to the i18n key shown to the user.
func errorMessageKey(err error) string {
	switch {
	case errors.Is(err, service.ErrDislikeViolation):
		return "generate.error_dislikes"
	case errors.Is(err, service.ErrPortionsShort):
		return "generate.error_portions"
	case errors.Is(err, service.ErrProteinVariety):
		return "generate.error_variety"
	default:
		return "generate.error"
	}
}
