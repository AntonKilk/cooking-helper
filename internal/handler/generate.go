package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/service"
)

// weekGenerator is the slice of the generation service the handler needs, kept
// narrow so the handler is testable with a stub. *service.GenerationService
// satisfies it.
type weekGenerator interface {
	GenerateWeek(ctx context.Context, h *domain.HouseholdProfile) (*service.GeneratedWeek, error)
}

// generateHandlers serve the one-tap weekly generation.
type generateHandlers struct {
	rd         *renderer
	households householdProfiles
	gen        weekGenerator
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
