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

// stubProfiles is a hand-written householdProfiles for handler tests. It records
// the last UpdateProfile call so tests can assert it was (or was not) invoked.
type stubProfiles struct {
	current     *domain.HouseholdProfile
	updateCalls int
	gotLang     domain.Language
	gotAdults   int
	gotKids     int
}

func (s *stubProfiles) Current(_ context.Context, _ domain.Language) (*domain.HouseholdProfile, error) {
	return s.current, nil
}

func (s *stubProfiles) UpdateProfile(_ context.Context, _ string, lang domain.Language, adults, kids int) (*domain.HouseholdProfile, error) {
	s.updateCalls++
	s.gotLang, s.gotAdults, s.gotKids = lang, adults, kids
	c := *s.current
	c.Language = lang
	c.FamilySize = domain.FamilySize{Adults: adults, Kids: kids}
	return &c, nil
}

func (s *stubProfiles) AddPantryBasic(_ context.Context, _, item string) (*domain.HouseholdProfile, error) {
	s.current.PantryBasics = append(s.current.PantryBasics, item)
	return s.current, nil
}

func (s *stubProfiles) RemovePantryBasic(_ context.Context, _, item string) (*domain.HouseholdProfile, error) {
	kept := s.current.PantryBasics[:0:0]
	for _, existing := range s.current.PantryBasics {
		if existing != item {
			kept = append(kept, existing)
		}
	}
	s.current.PantryBasics = kept
	return s.current, nil
}

func newProfileRouter(t *testing.T, stub *stubProfiles) http.Handler {
	t.Helper()
	bundle := testBundle(t)
	rd := &renderer{tmpl: testTemplates(t), bundle: bundle}
	ph := &profileHandlers{rd: rd, bundle: bundle, svc: stub}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /settings/profile", ph.Show)
	mux.HandleFunc("POST /settings/profile", ph.Save)
	return languageMiddleware(bundle, mux)
}

func defaultStub() *stubProfiles {
	return &stubProfiles{current: &domain.HouseholdProfile{
		ID:         "h1",
		Language:   domain.LanguageEN,
		FamilySize: domain.FamilySize{Adults: 2, Kids: 1},
	}}
}

func TestProfileShowRendersCurrentValues(t *testing.T) {
	srv := newProfileRouter(t, defaultStub())
	req := httptest.NewRequest(http.MethodGet, "/settings/profile", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="adults" min="1" max="6" value="2"`) {
		t.Errorf("body missing adults input with value 2:\n%s", body)
	}
	if !strings.Contains(body, `name="kids" min="0" max="6" value="1"`) {
		t.Errorf("body missing kids input with value 1:\n%s", body)
	}
}

func TestProfileSaveValidPersistsAndRedirects(t *testing.T) {
	stub := defaultStub()
	srv := newProfileRouter(t, stub)
	form := url.Values{"adults": {"3"}, "kids": {"2"}, "lang": {"fi"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/settings/profile" {
		t.Errorf("Location = %q, want /settings/profile", loc)
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

func TestProfileSaveOutOfRangeRejected(t *testing.T) {
	cases := []struct {
		name         string
		adults, kids string
	}{
		{"adults too low", "0", "0"},
		{"adults too high", "7", "0"},
		{"kids too high", "2", "7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := defaultStub()
			srv := newProfileRouter(t, stub)
			form := url.Values{"adults": {c.adults}, "kids": {c.kids}, "lang": {"en"}}
			req := httptest.NewRequest(http.MethodPost, "/settings/profile", strings.NewReader(form.Encode()))
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
		})
	}
}
