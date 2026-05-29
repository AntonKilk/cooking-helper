package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/repository"
)

// newRecipeHandler builds a recipeHandlers backed by the given reader and the
// shared test bundle/templates.
func newRecipeHandler(t *testing.T, recipes recipeReader) *recipeHandlers {
	t.Helper()
	rd := &renderer{tmpl: testTemplates(t), bundle: testBundle(t)}
	return &recipeHandlers{rd: rd, recipes: recipes}
}

func getRecipe(id string) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(http.MethodGet, "/recipe/"+id, nil)
	req.SetPathValue("id", id)
	return httptest.NewRecorder(), req
}

func TestRecipeRendersFullscreenView(t *testing.T) {
	srv := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/recipe/abc123", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Test Recipe",          // title
		"A short description.", // description
		"25 min",               // localized cook time
		"4 servings",           // localized servings
		"Ingredients",          // localized heading
		"Steps",                // localized heading
		"flour",                // ingredient name
		"250",                  // ingredient amount, formatted without trailing zeros
		"pinch",                // ingredient with no amount but a unit
		"Mix the flour",        // step text
		"Bake for 20 minutes.", // step text
		`data-recipe-id="abc123"`,
		`class="recipe-step"`,
		"Mark done",
		"Step 1",
		"Step 2",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestRecipeBlankIDNotFound(t *testing.T) {
	h := newRecipeHandler(t, stubRecipeReader{byID: testRecipes()})
	req := httptest.NewRequest(http.MethodGet, "/recipe/", nil)
	rec := httptest.NewRecorder()

	h.Show(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Recipe not found") {
		t.Errorf("body missing not-found message:\n%s", rec.Body.String())
	}
}

func TestRecipeHTMXReturnsFragment(t *testing.T) {
	srv := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/recipe/abc123", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Test Recipe") {
		t.Errorf("fragment missing recipe title:\n%s", body)
	}
	if strings.Contains(strings.ToLower(body), "<!doctype") {
		t.Errorf("HTMX request returned a full page, want fragment only:\n%s", body)
	}
}

func TestRecipeNotFoundFromRepository(t *testing.T) {
	h := newRecipeHandler(t, stubRecipeReader{byID: map[string]*domain.Recipe{}})
	rec, req := getRecipe("missing-id")

	h.Show(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Recipe not found") {
		t.Errorf("body missing localized not-found message:\n%s", body)
	}
	if strings.Contains(body, "repository:") {
		t.Errorf("internal error sentinel leaked to body:\n%s", body)
	}
}

func TestRecipeRepositoryErrorIs500(t *testing.T) {
	h := newRecipeHandler(t, stubRecipeReader{err: errors.New("db down")})
	rec, req := getRecipe("abc123")

	h.Show(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "db down") {
		t.Errorf("internal error detail leaked to body:\n%s", body)
	}
	if !strings.Contains(body, "internal server error") {
		t.Errorf("expected generic error body, got:\n%s", body)
	}
}

func TestRecipeRendersStepsMarkup(t *testing.T) {
	srv := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/recipe/abc123", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, want := range []string{
		`data-recipe-id="abc123"`,
		`data-step-index="0"`,
		`data-step-index="1"`,
		`class="recipe-step"`,
		`class="recipe-step__toggle"`,
		`aria-pressed="false"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing step-tracker hook %q:\n%s", want, body)
		}
	}
}

func TestRecipeRendersInAllThreeLanguages(t *testing.T) {
	srv := newTestRouter(t)
	cases := []struct {
		lang        string
		ingredients string
		steps       string
	}{
		{"en-US", "Ingredients", "Steps"},
		{"fi-FI", "Ainekset", "Vaiheet"},
		{"ru-RU", "Ингредиенты", "Шаги"},
	}
	for _, c := range cases {
		t.Run(c.lang, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/recipe/abc123", nil)
			req.Header.Set("Accept-Language", c.lang)
			rec := httptest.NewRecorder()

			srv.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, c.ingredients) {
				t.Errorf("[%s] body missing %q", c.lang, c.ingredients)
			}
			if !strings.Contains(body, c.steps) {
				t.Errorf("[%s] body missing %q", c.lang, c.steps)
			}
		})
	}
}

// Compile-time guard: repository.Store must satisfy our narrow reader interface
// so the router wiring stays honest if either side drifts.
var _ recipeReader = (*repository.Store)(nil)

// Compile-time guard: the test stub must satisfy the same interface.
var _ recipeReader = stubRecipeReader{}
