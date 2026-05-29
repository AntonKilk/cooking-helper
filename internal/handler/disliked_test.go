package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/service"
)

// stubDisliked is a hand-written dislikedProfiles for handler tests. It records
// add/remove calls and applies them to the in-memory profile so re-renders show
// the updated list.
type stubDisliked struct {
	profile    *domain.HouseholdProfile
	addCalls   int
	addTerm    string
	addErr     error
	removeTerm string
	removeCall int
}

func (s *stubDisliked) Current(_ context.Context, _ domain.Language) (*domain.HouseholdProfile, error) {
	return s.profile, nil
}

func (s *stubDisliked) AddDisliked(_ context.Context, _, term string) (*domain.HouseholdProfile, error) {
	s.addCalls++
	s.addTerm = term
	if s.addErr != nil {
		return nil, s.addErr
	}
	s.profile.DislikedIngredients = append(s.profile.DislikedIngredients, strings.TrimSpace(term))
	return s.profile, nil
}

func (s *stubDisliked) RemoveDisliked(_ context.Context, _, term string) (*domain.HouseholdProfile, error) {
	s.removeCall++
	s.removeTerm = term
	kept := s.profile.DislikedIngredients[:0:0]
	for _, d := range s.profile.DislikedIngredients {
		if !strings.EqualFold(d, strings.TrimSpace(term)) {
			kept = append(kept, d)
		}
	}
	s.profile.DislikedIngredients = kept
	return s.profile, nil
}

// stubHistory returns a fixed recipe set for autosuggest tests.
type stubHistory struct {
	recipes []domain.Recipe
}

func (s stubHistory) RecentRecipes(_ context.Context, _ string, _ int) ([]domain.Recipe, error) {
	return s.recipes, nil
}

func newDislikedRouter(t *testing.T, profiles dislikedProfiles, history recipeHistory) http.Handler {
	t.Helper()
	bundle := testBundle(t)
	rd := &renderer{tmpl: testTemplates(t), bundle: bundle}
	dh := &dislikedHandlers{rd: rd, profiles: profiles, history: history}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /settings/disliked", dh.Show)
	mux.HandleFunc("POST /settings/disliked", dh.Add)
	mux.HandleFunc("POST /settings/disliked/remove", dh.Remove)
	return languageMiddleware(bundle, mux)
}

func defaultDislikedStub() *stubDisliked {
	return &stubDisliked{profile: &domain.HouseholdProfile{
		ID:                  "h1",
		Language:            domain.LanguageEN,
		DislikedIngredients: []string{"Olives"},
	}}
}

func defaultHistory() stubHistory {
	return stubHistory{recipes: []domain.Recipe{
		{Ingredients: []domain.Ingredient{{Name: "Mushrooms"}, {Name: "Olives"}}},
		{Ingredients: []domain.Ingredient{{Name: "Garlic"}}},
	}}
}

func TestDislikedShowRendersListAndSuggestions(t *testing.T) {
	srv := newDislikedRouter(t, defaultDislikedStub(), defaultHistory())
	req := httptest.NewRequest(http.MethodGet, "/settings/disliked", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Olives") {
		t.Errorf("body missing existing disliked term Olives:\n%s", body)
	}
	if !strings.Contains(body, "<datalist") {
		t.Error("body missing autosuggest datalist")
	}
	if !strings.Contains(body, `<option value="Mushrooms">`) {
		t.Error("body missing history suggestion Mushrooms")
	}
	if strings.Contains(body, `<option value="Olives">`) {
		t.Error("already-disliked Olives should not appear as a suggestion")
	}
}

func TestDislikedAddPersistsAndRendersList(t *testing.T) {
	stub := defaultDislikedStub()
	srv := newDislikedRouter(t, stub, defaultHistory())
	form := url.Values{"ingredient": {"Mushrooms"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/disliked", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if stub.addCalls != 1 || stub.addTerm != "Mushrooms" {
		t.Fatalf("AddDisliked called %d times with %q, want 1 with Mushrooms", stub.addCalls, stub.addTerm)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="disliked-list"`) {
		t.Error("response is not the list fragment")
	}
	if !strings.Contains(body, "Mushrooms") {
		t.Error("re-rendered list missing the added term")
	}
}

func TestDislikedAddBlankRejected(t *testing.T) {
	stub := defaultDislikedStub()
	stub.addErr = service.ErrEmptyIngredient
	srv := newDislikedRouter(t, stub, defaultHistory())
	form := url.Values{"ingredient": {"   "}}
	req := httptest.NewRequest(http.MethodPost, "/settings/disliked", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Enter an ingredient name") {
		t.Errorf("body missing localized empty error:\n%s", rec.Body.String())
	}
}

func TestDislikedRemoveRendersList(t *testing.T) {
	stub := defaultDislikedStub()
	srv := newDislikedRouter(t, stub, defaultHistory())
	form := url.Values{"ingredient": {"Olives"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/disliked/remove", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if stub.removeCall != 1 || stub.removeTerm != "Olives" {
		t.Fatalf("RemoveDisliked called %d times with %q, want 1 with Olives", stub.removeCall, stub.removeTerm)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="disliked-list"`) {
		t.Error("response is not the list fragment")
	}
	if strings.Contains(body, `class="disliked-item__name">Olives`) {
		t.Error("removed term Olives still present in list")
	}
}
