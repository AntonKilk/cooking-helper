package handler

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	dict "github.com/AntonKilk/cooking-helper/i18n"
	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/i18n"
	"github.com/AntonKilk/cooking-helper/internal/repository"
	"github.com/AntonKilk/cooking-helper/templates"
)

// stubRecipeReader serves canned recipes for the recipe handler tests. Missing
// ids return repository.ErrNotFound (mirroring real Store.GetRecipe).
type stubRecipeReader struct {
	byID map[string]*domain.Recipe
	err  error
}

func (s stubRecipeReader) GetRecipe(_ context.Context, id string) (*domain.Recipe, error) {
	if s.err != nil {
		return nil, s.err
	}
	r, ok := s.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return r, nil
}

// testRecipes returns the fixture map used by the shared test router so the
// handler can resolve the ids used in the recipe_test.go cases.
func testRecipes() map[string]*domain.Recipe {
	return map[string]*domain.Recipe{
		"abc123": {
			ID:              "abc123",
			Title:           "Test Recipe",
			Description:     "A short description.",
			CookTimeMinutes: 25,
			Servings:        4,
			Ingredients: []domain.Ingredient{
				{Name: "flour", Amount: 250, Unit: "g"},
				{Name: "salt", Amount: 0, Unit: "pinch"},
			},
			Steps: []string{"Mix the flour and salt.", "Bake for 20 minutes."},
		},
		// A recipe that already carries feedback, so the detail view can be
		// asserted to render the active state (CH-16).
		"liked1": {
			ID:              "liked1",
			Title:           "Liked Recipe",
			CookTimeMinutes: 10,
			Servings:        2,
			Steps:           []string{"Eat."},
			Feedback:        &domain.Feedback{Liked: true},
		},
	}
}

func testBundle(t *testing.T) *i18n.Bundle {
	t.Helper()
	b, err := i18n.Load(dict.FS, domain.LanguageEN)
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	return b
}

func testTemplates(t *testing.T) *template.Template {
	t.Helper()
	tmpl, err := template.New("").Funcs(ParseFuncMap()).ParseFS(templates.FS, "*.gohtml")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	return tmpl
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	rd := &renderer{tmpl: testTemplates(t), bundle: testBundle(t)}
	hh := &homeHandlers{rd: rd}
	rh := &recipeHandlers{rd: rd, recipes: stubRecipeReader{byID: testRecipes()}, feedback: stubFeedbackSetter{byID: testRecipes()}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", hh.Home)
	mux.HandleFunc("GET /recipe/{id}", rh.Show)
	mux.HandleFunc("POST /recipe/{id}/feedback", rh.Feedback)
	mux.HandleFunc("GET /settings", rd.Settings)
	mux.HandleFunc("POST /settings/language", SetLanguage(rd.bundle))
	return languageMiddleware(rd.bundle, mux)
}

func TestHomeRendersByAcceptLanguage(t *testing.T) {
	srv := newTestRouter(t)
	cases := []struct {
		header string
		want   string // a localized string expected in the body
		lang   string
	}{
		{"fi-FI,fi;q=0.9", "Ostoslista", "fi"},
		{"ru-RU,ru;q=0.9", "Список покупок", "ru"},
		{"en-US,en;q=0.9", "Shopping list", "en"},
		{"de-DE", "Shopping list", "en"}, // unsupported → default EN
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept-Language", c.header)
		rec := httptest.NewRecorder()

		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("[%s] status = %d, want 200", c.header, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, c.want) {
			t.Errorf("[%s] body missing %q", c.header, c.want)
		}
		if !strings.Contains(body, `lang="`+c.lang+`"`) {
			t.Errorf("[%s] body missing html lang=%q", c.header, c.lang)
		}
	}
}

func TestHomeHTMXReturnsFragment(t *testing.T) {
	srv := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(strings.ToLower(body), "<!doctype") {
		t.Errorf("HTMX request returned a full page, want content fragment only:\n%s", body)
	}
	if !strings.Contains(body, "Shopping list") {
		t.Errorf("fragment missing home content:\n%s", body)
	}
}

func TestHomeFullPageHasShell(t *testing.T) {
	srv := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(strings.ToLower(body), "<!doctype") {
		t.Error("full page missing doctype")
	}
	if !strings.Contains(body, `rel="manifest"`) {
		t.Error("full page missing PWA manifest link")
	}
	if !strings.Contains(body, "serviceWorker") {
		t.Error("full page missing service worker registration")
	}
	if !strings.Contains(body, `class="app-header"`) {
		t.Error("full page missing shared header")
	}
}

func TestHomeCookieWinsOverHeader(t *testing.T) {
	srv := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.AddCookie(&http.Cookie{Name: languageCookie, Value: "fi"})
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "Ostoslista") {
		t.Error("cookie language (fi) did not win over Accept-Language header")
	}
}

func TestSetLanguageSetsCookieAndRedirects(t *testing.T) {
	srv := newTestRouter(t)
	form := url.Values{"lang": {"ru"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/language", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q, want %q", loc, "/")
	}
	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == languageCookie && c.Value == "ru" {
			found = true
		}
	}
	if !found {
		t.Error("expected lang=ru cookie to be set")
	}
}

func TestSetLanguageRejectsUnsupported(t *testing.T) {
	srv := newTestRouter(t)
	form := url.Values{"lang": {"zz"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/language", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if strings.Contains(rec.Body.String(), "zz") {
		t.Error("response should not echo the raw rejected input")
	}
}
