package handler

import "net/http"

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

// homeHandlers serve the home screen. canGenerate reflects whether an LLM client
// is wired, which decides if the "generate week" button is active.
type homeHandlers struct {
	rd          *renderer
	canGenerate bool
}

// homeData is the view model for the home page.
type homeData struct {
	Lang        string
	CanGenerate bool
}

// Home renders the localized home page: the generate-week action plus a link to
// the shopping list. The button is inert when generation is not configured.
func (hh *homeHandlers) Home(w http.ResponseWriter, r *http.Request) {
	data := homeData{
		Lang:        string(LanguageFromContext(r.Context())),
		CanGenerate: hh.canGenerate,
	}
	hh.rd.render(w, r, "home", data)
}
