package handler

import (
	"context"
	"net/http"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

// categoryKeys are the store-category translation keys, in the order they are
// shown on the shopping list (PRD §15 Appendix).
var categoryKeys = []string{
	"category.produce",
	"category.meat_fish",
	"category.dairy",
	"category.pantry",
	"category.frozen",
	"category.other",
}

// homeProfiles is the narrow household reader the home screen needs to gate the
// first-run onboarding wizard. *service.HouseholdService satisfies it.
type homeProfiles interface {
	Current(ctx context.Context, defaultLang domain.Language) (*domain.HouseholdProfile, error)
}

// homeHandlers serve the home screen. canGenerate reflects whether an LLM client
// is wired, which decides if the "generate week" button is active. profiles gates
// the first-run onboarding redirect; when nil (e.g. in isolated tests) the gate is
// skipped.
type homeHandlers struct {
	rd          *renderer
	canGenerate bool
	profiles    homeProfiles
}

// homeData is the view model for the home page.
type homeData struct {
	Lang        string
	CanGenerate bool
}

// Home renders the localized home page: the generate-week action plus a link to
// the shopping list. The button is inert when generation is not configured.
func (hh *homeHandlers) Home(w http.ResponseWriter, r *http.Request) {
	if hh.profiles != nil {
		h, err := hh.profiles.Current(r.Context(), LanguageFromContext(r.Context()))
		if err != nil {
			hh.rd.fail(w, r, "load home profile", err)
			return
		}
		if !h.Onboarded {
			http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
			return
		}
	}

	data := homeData{
		Lang:        string(LanguageFromContext(r.Context())),
		CanGenerate: hh.canGenerate,
	}
	hh.rd.render(w, r, "home", data)
}
