package handler

import "net/http"

// settingsData is the view model for the settings hub.
type settingsData struct {
	Lang string
}

// Settings renders the settings hub: a link to the household profile and the
// language switcher.
func (rd *renderer) Settings(w http.ResponseWriter, r *http.Request) {
	rd.render(w, r, "settings", settingsData{
		Lang: string(LanguageFromContext(r.Context())),
	})
}
