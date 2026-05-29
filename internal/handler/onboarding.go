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

// onboardingProfiles is the slice of the household service the onboarding wizard
// needs. Keeping it narrow lets the handlers be tested with a stub.
type onboardingProfiles interface {
	Current(ctx context.Context, defaultLang domain.Language) (*domain.HouseholdProfile, error)
	UpdateProfile(ctx context.Context, id string, lang domain.Language, adults, kids int) (*domain.HouseholdProfile, error)
	CompleteOnboarding(ctx context.Context, id string) (*domain.HouseholdProfile, error)
}

// onboardingHandlers serve the first-run onboarding wizard (CH-19): a short
// 3-step intro shown until the household completes or skips it.
type onboardingHandlers struct {
	rd     *renderer
	bundle *i18n.Bundle
	svc    onboardingProfiles
}

// onboardingData is the view model for the onboarding wizard. Items and the
// adults/kids/lang fields back the embedded profile form (step 1) and pantry list
// (step 2); Error carries an i18n key when a step-1 submission is invalid.
type onboardingData struct {
	Lang   string
	Step   int
	Adults int
	Kids   int
	Items  []string
	Error  string
}

// onboardingSteps is the number of wizard steps.
const onboardingSteps = 3

// Show renders the wizard step requested via ?step= (1-onboardingSteps),
// defaulting to step 1 for a missing or out-of-range value.
func (oh *onboardingHandlers) Show(w http.ResponseWriter, r *http.Request) {
	h, err := oh.svc.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		oh.rd.fail(w, r, "load onboarding", err)
		return
	}

	step := 1
	if n, convErr := strconv.Atoi(r.URL.Query().Get("step")); convErr == nil && n >= 1 && n <= onboardingSteps {
		step = n
	}

	oh.rd.render(w, r, "onboarding", onboardingData{
		Lang:   string(h.Language),
		Step:   step,
		Adults: h.FamilySize.Adults,
		Kids:   h.FamilySize.Kids,
		Items:  h.PantryBasics,
	})
}

// SaveProfile validates and persists the step-1 family composition and language,
// syncs the language cookie, and advances to step 2. Invalid input re-renders
// step 1 with a localized error and HTTP 400 without persisting.
func (oh *onboardingHandlers) SaveProfile(w http.ResponseWriter, r *http.Request) {
	adults, adultsErr := strconv.Atoi(r.FormValue("adults"))
	kids, kidsErr := strconv.Atoi(r.FormValue("kids"))
	lang := domain.Language(r.FormValue("lang"))

	if adultsErr != nil || kidsErr != nil || !oh.bundle.Has(lang) ||
		adults < minAdults || adults > maxAdults || kids < minKids || kids > maxKids {
		oh.renderInvalid(w, r, lang, adults, kids)
		return
	}

	h, err := oh.svc.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		oh.rd.fail(w, r, "load onboarding", err)
		return
	}

	if _, err := oh.svc.UpdateProfile(r.Context(), h.ID, lang, adults, kids); err != nil {
		if errors.Is(err, service.ErrInvalidFamilySize) {
			oh.renderInvalid(w, r, lang, adults, kids)
			return
		}
		oh.rd.fail(w, r, "update onboarding profile", err)
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
	http.Redirect(w, r, "/onboarding?step=2", http.StatusSeeOther)
}

// Complete marks onboarding as done and returns to the home screen. It backs both
// the per-step Skip control and the final Finish button.
func (oh *onboardingHandlers) Complete(w http.ResponseWriter, r *http.Request) {
	h, err := oh.svc.Current(r.Context(), LanguageFromContext(r.Context()))
	if err != nil {
		oh.rd.fail(w, r, "load onboarding", err)
		return
	}

	if _, err := oh.svc.CompleteOnboarding(r.Context(), h.ID); err != nil {
		oh.rd.fail(w, r, "complete onboarding", err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// renderInvalid re-renders step 1 echoing the submitted values with a range error
// and HTTP 400.
func (oh *onboardingHandlers) renderInvalid(w http.ResponseWriter, r *http.Request, lang domain.Language, adults, kids int) {
	oh.rd.renderStatus(w, r, http.StatusBadRequest, "onboarding", onboardingData{
		Lang:   string(lang),
		Step:   1,
		Adults: adults,
		Kids:   kids,
		Error:  "profile.error_range",
	})
}
