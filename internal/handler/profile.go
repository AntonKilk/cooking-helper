package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/i18n"
	"github.com/AntonKilk/cooking-helper/internal/service"
)

// householdProfiles is the slice of the household service the profile handlers
// need. Depending on the interface keeps the handlers testable with a stub.
type householdProfiles interface {
	Current(ctx context.Context, defaultLang domain.Language) (*domain.HouseholdProfile, error)
	UpdateProfile(ctx context.Context, id string, lang domain.Language, adults, kids int) (*domain.HouseholdProfile, error)
	AddPantryBasic(ctx context.Context, id, item string) (*domain.HouseholdProfile, error)
	RemovePantryBasic(ctx context.Context, id, item string) (*domain.HouseholdProfile, error)
}

// profileHandlers serve the household profile screen.
type profileHandlers struct {
	rd     *renderer
	bundle *i18n.Bundle
	svc    householdProfiles
}

// profileData is the view model for the profile template.
type profileData struct {
	Lang   string
	Adults int
	Kids   int
	Error  string // i18n key of a validation error, empty when none
}

// Show renders the profile form populated with the current household values,
// creating the household with defaults on first access.
func (ph *profileHandlers) Show(w http.ResponseWriter, r *http.Request) {
	h, err := ph.svc.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		ph.rd.fail(w, r, "load profile", err)
		return
	}
	ph.rd.render(w, r, "profile", profileData{
		Lang:   string(h.Language),
		Adults: h.FamilySize.Adults,
		Kids:   h.FamilySize.Kids,
	})
}

// Save validates the submitted family composition and language, persists them,
// syncs the language cookie, and redirects back to the profile. Invalid input is
// rejected with a localized error and HTTP 400 without persisting.
func (ph *profileHandlers) Save(w http.ResponseWriter, r *http.Request) {
	adults, adultsErr := strconv.Atoi(r.FormValue("adults"))
	kids, kidsErr := strconv.Atoi(r.FormValue("kids"))
	lang := domain.Language(r.FormValue("lang"))

	if adultsErr != nil || kidsErr != nil || !ph.bundle.Has(lang) ||
		adults < minAdults || adults > maxAdults || kids < minKids || kids > maxKids {
		ph.renderInvalid(w, r, lang, adults, kids)
		return
	}

	h, err := ph.svc.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		ph.rd.fail(w, r, "load profile", err)
		return
	}

	if _, err := ph.svc.UpdateProfile(r.Context(), h.ID, lang, adults, kids); err != nil {
		if errors.Is(err, service.ErrInvalidFamilySize) {
			ph.renderInvalid(w, r, lang, adults, kids)
			return
		}
		ph.rd.fail(w, r, "update profile", err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     languageCookie,
		Value:    string(lang),
		Path:     "/",
		MaxAge:   languageCookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/settings/profile", http.StatusSeeOther)
}

// minAdults..maxKids mirror the service bounds; the handler validates at the HTTP
// boundary so it can return a localized message instead of a bare error.
const (
	minAdults = 1
	maxAdults = 6
	minKids   = 0
	maxKids   = 6
)

// renderInvalid re-renders the form echoing the submitted values with a range
// error and HTTP 400.
func (ph *profileHandlers) renderInvalid(w http.ResponseWriter, r *http.Request, lang domain.Language, adults, kids int) {
	ph.rd.renderStatus(w, r, http.StatusBadRequest, "profile", profileData{
		Lang:   string(lang),
		Adults: adults,
		Kids:   kids,
		Error:  "profile.error_range",
	})
}
