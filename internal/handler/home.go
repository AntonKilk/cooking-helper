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

// homeData is the view model for the demo/home page.
type homeData struct {
	Lang         string
	CategoryKeys []string
}

// Home renders the localized demo page: the store categories plus a language
// switcher. It exercises the i18n wiring end to end.
func (rd *renderer) Home(w http.ResponseWriter, r *http.Request) {
	data := homeData{
		Lang:         string(LanguageFromContext(r.Context())),
		CategoryKeys: categoryKeys,
	}
	rd.render(w, r, "home", data)
}
