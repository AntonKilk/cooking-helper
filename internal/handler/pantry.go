package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/AntonKilk/cooking-helper/internal/i18n"
	"github.com/AntonKilk/cooking-helper/internal/service"
)

// pantryHandlers serve the pantry-basics screen (F-6 / CH-14): the editable list
// of "always at home" staples that are excluded from the shopping list. Reads and
// writes go through the household service; the list is persisted to
// HouseholdProfile.PantryBasics and applied at the next shopping-list build.
type pantryHandlers struct {
	rd     *renderer
	bundle *i18n.Bundle
	svc    householdProfiles
}

// pantryData is the view model for the pantry-basics screen and its list fragment.
type pantryData struct {
	Lang  string
	Items []string
	Error string // i18n key of a validation error, empty when none
}

// Show renders the pantry-basics screen populated with the current list, creating
// the household with defaults on first access.
func (ph *pantryHandlers) Show(w http.ResponseWriter, r *http.Request) {
	h, err := ph.svc.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		ph.rd.fail(w, r, "load pantry basics", err)
		return
	}
	ph.rd.render(w, r, "pantry", pantryData{
		Lang:  string(h.Language),
		Items: h.PantryBasics,
	})
}

// Add appends the submitted ingredient to the pantry-basics list and re-renders
// the list fragment. An all-whitespace item is rejected with a localized error
// and HTTP 400 without persisting.
func (ph *pantryHandlers) Add(w http.ResponseWriter, r *http.Request) {
	item := strings.TrimSpace(r.FormValue("item"))

	h, err := ph.svc.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		ph.rd.fail(w, r, "load pantry basics", err)
		return
	}

	if item == "" {
		ph.renderList(w, r, http.StatusBadRequest, string(h.Language), h.PantryBasics, "pantry.error_empty")
		return
	}

	updated, err := ph.svc.AddPantryBasic(r.Context(), h.ID, item)
	if err != nil {
		if errors.Is(err, service.ErrEmptyIngredient) {
			ph.renderList(w, r, http.StatusBadRequest, string(h.Language), h.PantryBasics, "pantry.error_empty")
			return
		}
		ph.rd.fail(w, r, "add pantry basic", err)
		return
	}
	ph.renderList(w, r, http.StatusOK, string(updated.Language), updated.PantryBasics, "")
}

// Remove drops the submitted ingredient from the pantry-basics list and re-renders
// the list fragment. Removing an absent item is a benign no-op.
func (ph *pantryHandlers) Remove(w http.ResponseWriter, r *http.Request) {
	item := strings.TrimSpace(r.FormValue("item"))

	h, err := ph.svc.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		ph.rd.fail(w, r, "load pantry basics", err)
		return
	}

	updated, err := ph.svc.RemovePantryBasic(r.Context(), h.ID, item)
	if err != nil {
		ph.rd.fail(w, r, "remove pantry basic", err)
		return
	}
	ph.renderList(w, r, http.StatusOK, string(updated.Language), updated.PantryBasics, "")
}

// renderList renders the swappable pantry/list fragment with the given status.
func (ph *pantryHandlers) renderList(w http.ResponseWriter, r *http.Request, status int, lang string, items []string, errKey string) {
	ph.rd.renderFragment(w, r, status, "pantry/list", pantryData{
		Lang:  lang,
		Items: items,
		Error: errKey,
	})
}
