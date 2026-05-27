package handler

import (
	"net/http"
	"strings"
)

// recipeData is the view model for the recipe screen.
type recipeData struct {
	Lang     string
	ID       string
	NotFound bool
}

// Recipe renders a single recipe screen. Persistence is not wired yet, so this
// is a stub that echoes the requested id; a blank id renders a localized
// not-found page with HTTP 404.
func (rd *renderer) Recipe(w http.ResponseWriter, r *http.Request) {
	lang := string(LanguageFromContext(r.Context()))
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		rd.renderStatus(w, r, http.StatusNotFound, "recipe", recipeData{Lang: lang, NotFound: true})
		return
	}
	rd.render(w, r, "recipe", recipeData{Lang: lang, ID: id})
}
