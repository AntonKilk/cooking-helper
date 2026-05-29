package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
)

// stubOnboarding is a hand-written onboardingProfiles (and homeProfiles) for the
// onboarding handler tests. It records whether the mutating calls fired.
type stubOnboarding struct {
	current       *domain.HouseholdProfile
	updateCalls   int
	completeCalls int
	gotAdults     int
	gotKids       int
	gotLang       domain.Language
}

func (s *stubOnboarding) Current(_ context.Context, _ domain.Language) (*domain.HouseholdProfile, error) {
	return s.current, nil
}

func (s *stubOnboarding) UpdateProfile(_ context.Context, _ string, lang domain.Language, adults, kids int) (*domain.HouseholdProfile, error) {
	s.updateCalls++
	s.gotLang, s.gotAdults, s.gotKids = lang, adults, kids
	c := *s.current
	c.Language = lang
	c.FamilySize = domain.FamilySize{Adults: adults, Kids: kids}
	return &c, nil
}

func (s *stubOnboarding) CompleteOnboarding(_ context.Context, _ string) (*domain.HouseholdProfile, error) {
	s.completeCalls++
	c := *s.current
	c.Onboarded = true
	return &c, nil
}

func newOnboardingRouter(t *testing.T, stub *stubOnboarding) http.Handler {
	t.Helper()
	bundle := testBundle(t)
	rd := &renderer{tmpl: testTemplates(t), bundle: bundle}
	oh := &onboardingHandlers{rd: rd, bundle: bundle, svc: stub}
	hh := &homeHandlers{rd: rd, profiles: stub}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", hh.Home)
	mux.HandleFunc("GET /onboarding", oh.Show)
	mux.HandleFunc("POST /onboarding/profile", oh.SaveProfile)
	mux.HandleFunc("POST /onboarding/complete", oh.Complete)
	return languageMiddleware(bundle, mux)
}

func defaultOnboardingStub() *stubOnboarding {
	return &stubOnboarding{current: &domain.HouseholdProfile{
		ID:           "h1",
		Language:     domain.LanguageEN,
		FamilySize:   domain.FamilySize{Adults: 2, Kids: 0},
		PantryBasics: []string{"salt", "sugar"},
	}}
}

func TestOnboardingShowDefaultsToStepOne(t *testing.T) {
	srv := newOnboardingRouter(t, defaultOnboardingStub())
	req := httptest.NewRequest(http.MethodGet, "/onboarding", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="adults"`) {
		t.Errorf("step 1 should render the adults input:\n%s", body)
	}
	if !strings.Contains(body, `action="/onboarding/complete"`) {
		t.Error("every step should render a Skip control posting to /onboarding/complete")
	}
}

func TestOnboardingShowStepTwoRendersPantry(t *testing.T) {
	srv := newOnboardingRouter(t, defaultOnboardingStub())
	req := httptest.NewRequest(http.MethodGet, "/onboarding?step=2", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "salt") || !strings.Contains(body, "sugar") {
		t.Errorf("step 2 should list the pantry basics:\n%s", body)
	}
	if !strings.Contains(body, `href="/onboarding?step=3"`) {
		t.Error("step 2 should link forward to step 3")
	}
}

func TestOnboardingShowStepThreeRendersFinish(t *testing.T) {
	srv := newOnboardingRouter(t, defaultOnboardingStub())
	req := httptest.NewRequest(http.MethodGet, "/onboarding?step=3", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `action="/onboarding/complete"`) {
		t.Error("step 3 should render a Finish control posting to /onboarding/complete")
	}
}

func TestOnboardingShowClampsOutOfRangeStep(t *testing.T) {
	srv := newOnboardingRouter(t, defaultOnboardingStub())
	req := httptest.NewRequest(http.MethodGet, "/onboarding?step=99", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `name="adults"`) {
		t.Error("an out-of-range step should fall back to step 1")
	}
}

func TestOnboardingSaveProfileValidAdvancesToStepTwo(t *testing.T) {
	stub := defaultOnboardingStub()
	srv := newOnboardingRouter(t, stub)
	form := url.Values{"adults": {"3"}, "kids": {"2"}, "lang": {"fi"}}
	req := httptest.NewRequest(http.MethodPost, "/onboarding/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/onboarding?step=2" {
		t.Errorf("Location = %q, want /onboarding?step=2", loc)
	}
	if stub.updateCalls != 1 {
		t.Fatalf("UpdateProfile called %d times, want 1", stub.updateCalls)
	}
	if stub.gotAdults != 3 || stub.gotKids != 2 || stub.gotLang != domain.LanguageFI {
		t.Errorf("update got adults=%d kids=%d lang=%q, want 3/2/fi", stub.gotAdults, stub.gotKids, stub.gotLang)
	}
	var langCookie bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == languageCookie && c.Value == "fi" {
			langCookie = true
		}
	}
	if !langCookie {
		t.Error("expected lang=fi cookie to be set")
	}
}

func TestOnboardingSaveProfileOutOfRangeRejected(t *testing.T) {
	stub := defaultOnboardingStub()
	srv := newOnboardingRouter(t, stub)
	form := url.Values{"adults": {"0"}, "kids": {"0"}, "lang": {"en"}}
	req := httptest.NewRequest(http.MethodPost, "/onboarding/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if stub.updateCalls != 0 {
		t.Errorf("UpdateProfile called %d times, want 0", stub.updateCalls)
	}
	if !strings.Contains(rec.Body.String(), "Adults must be") {
		t.Error("body missing localized range error")
	}
}

func TestOnboardingCompleteSetsFlagAndRedirectsHome(t *testing.T) {
	stub := defaultOnboardingStub()
	srv := newOnboardingRouter(t, stub)
	req := httptest.NewRequest(http.MethodPost, "/onboarding/complete", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q, want /", loc)
	}
	if stub.completeCalls != 1 {
		t.Fatalf("CompleteOnboarding called %d times, want 1", stub.completeCalls)
	}
}

func TestHomeRedirectsToOnboardingWhenNotOnboarded(t *testing.T) {
	stub := defaultOnboardingStub() // Onboarded defaults to false
	srv := newOnboardingRouter(t, stub)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/onboarding" {
		t.Errorf("Location = %q, want /onboarding", loc)
	}
}

func TestHomeRendersWhenOnboarded(t *testing.T) {
	stub := defaultOnboardingStub()
	stub.current.Onboarded = true
	srv := newOnboardingRouter(t, stub)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `id="week"`) {
		t.Error("an onboarded household should see the home screen")
	}
}
