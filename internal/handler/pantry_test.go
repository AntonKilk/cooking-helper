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

// stubPantry is a hand-written householdProfiles for pantry handler tests. It
// records add/remove invocations so tests can assert the service was (or was not)
// called and mutates its in-memory list to mirror the real service.
type stubPantry struct {
	current    *domain.HouseholdProfile
	addCalls   int
	gotAddItem string
	rmCalls    int
	gotRmItem  string
}

func (s *stubPantry) Current(_ context.Context, _ domain.Language) (*domain.HouseholdProfile, error) {
	return s.current, nil
}

func (s *stubPantry) UpdateProfile(_ context.Context, _ string, _ domain.Language, _, _ int) (*domain.HouseholdProfile, error) {
	return s.current, nil
}

func (s *stubPantry) AddPantryBasic(_ context.Context, _, item string) (*domain.HouseholdProfile, error) {
	s.addCalls++
	s.gotAddItem = item
	s.current.PantryBasics = append(s.current.PantryBasics, item)
	return s.current, nil
}

func (s *stubPantry) RemovePantryBasic(_ context.Context, _, item string) (*domain.HouseholdProfile, error) {
	s.rmCalls++
	s.gotRmItem = item
	kept := s.current.PantryBasics[:0:0]
	for _, existing := range s.current.PantryBasics {
		if existing != item {
			kept = append(kept, existing)
		}
	}
	s.current.PantryBasics = kept
	return s.current, nil
}

func newPantryRouter(t *testing.T, stub *stubPantry) http.Handler {
	t.Helper()
	bundle := testBundle(t)
	rd := &renderer{tmpl: testTemplates(t), bundle: bundle}
	pan := &pantryHandlers{rd: rd, bundle: bundle, svc: stub}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /settings/pantry", pan.Show)
	mux.HandleFunc("POST /settings/pantry/add", pan.Add)
	mux.HandleFunc("POST /settings/pantry/remove", pan.Remove)
	return languageMiddleware(bundle, mux)
}

func pantryStub(items ...string) *stubPantry {
	return &stubPantry{current: &domain.HouseholdProfile{
		ID:           "h1",
		Language:     domain.LanguageEN,
		PantryBasics: items,
	}}
}

func TestPantryShowRendersItems(t *testing.T) {
	srv := newPantryRouter(t, pantryStub("salt", "sugar"))
	req := httptest.NewRequest(http.MethodGet, "/settings/pantry", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "salt") || !strings.Contains(body, "sugar") {
		t.Errorf("body missing current items:\n%s", body)
	}
}

func TestPantryAddAppendsItem(t *testing.T) {
	stub := pantryStub("salt")
	srv := newPantryRouter(t, stub)
	form := url.Values{"item": {"olive oil"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/pantry/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if stub.addCalls != 1 || stub.gotAddItem != "olive oil" {
		t.Fatalf("add called %d times with %q, want 1 with %q", stub.addCalls, stub.gotAddItem, "olive oil")
	}
	if !strings.Contains(rec.Body.String(), "olive oil") {
		t.Errorf("fragment missing new item:\n%s", rec.Body.String())
	}
}

func TestPantryAddEmptyRejected(t *testing.T) {
	stub := pantryStub("salt")
	srv := newPantryRouter(t, stub)
	form := url.Values{"item": {"   "}}
	req := httptest.NewRequest(http.MethodPost, "/settings/pantry/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if stub.addCalls != 0 {
		t.Errorf("AddPantryBasic called %d times, want 0", stub.addCalls)
	}
	if !strings.Contains(rec.Body.String(), "Enter an ingredient") {
		t.Errorf("body missing localized empty error:\n%s", rec.Body.String())
	}
}

func TestPantryRemoveDropsItem(t *testing.T) {
	stub := pantryStub("salt", "sugar")
	srv := newPantryRouter(t, stub)
	form := url.Values{"item": {"salt"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/pantry/remove", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if stub.rmCalls != 1 || stub.gotRmItem != "salt" {
		t.Fatalf("remove called %d times with %q, want 1 with %q", stub.rmCalls, stub.gotRmItem, "salt")
	}
	body := rec.Body.String()
	if strings.Contains(body, ">salt<") {
		t.Errorf("fragment still contains removed item:\n%s", body)
	}
	if !strings.Contains(body, "sugar") {
		t.Errorf("fragment dropped surviving item:\n%s", body)
	}
}
