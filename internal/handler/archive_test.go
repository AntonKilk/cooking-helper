package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/service"
)

// stubArchiveLister serves canned recipes (or an error) for the archive screen
// and records the query it was asked for.
type stubArchiveLister struct {
	recipes  []domain.Recipe
	err      error
	gotQuery string
}

func (s *stubArchiveLister) SearchRecipes(_ context.Context, _, query string, _ int) ([]domain.Recipe, error) {
	s.gotQuery = query
	if s.err != nil {
		return nil, s.err
	}
	return s.recipes, nil
}

// stubCookAgain satisfies cookAgainService: CurrentPlan returns plan/planErr and
// CookAgain records its arguments and returns swapped/cookErr.
type stubCookAgain struct {
	plan      *domain.WeeklyPlan
	planErr   error
	swapped   *service.SwappedRecipe
	cookErr   error
	cookCalls int
	gotOld    string
	gotSource string
}

func (s *stubCookAgain) CurrentPlan(_ context.Context, _ string) (*domain.WeeklyPlan, error) {
	return s.plan, s.planErr
}

func (s *stubCookAgain) CookAgain(_ context.Context, _ *domain.HouseholdProfile, _ *domain.WeeklyPlan, oldRecipeID, sourceRecipeID string) (*service.SwappedRecipe, error) {
	s.cookCalls++
	s.gotOld, s.gotSource = oldRecipeID, sourceRecipeID
	if s.cookErr != nil {
		return nil, s.cookErr
	}
	return s.swapped, nil
}

func newArchiveRouter(t *testing.T, lister archiveLister, plans recipeLoader, cook cookAgainService, canCook bool) http.Handler {
	t.Helper()
	bundle := testBundle(t)
	rd := &renderer{tmpl: testTemplates(t), bundle: bundle}
	ah := &archiveHandlers{rd: rd, recipes: lister, households: stubHouseholds{}, plans: plans, cook: cook, canCook: canCook}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /archive", ah.Show)
	mux.HandleFunc("GET /archive/search", ah.Search)
	mux.HandleFunc("GET /archive/cook-again/{id}", ah.CookAgainDialog)
	mux.HandleFunc("POST /archive/cook-again/{id}", ah.CookAgain)
	return languageMiddleware(bundle, mux)
}

func archiveRecipes() []domain.Recipe {
	// Returned newest-first (the repository orders; the handler preserves order).
	return []domain.Recipe{
		{ID: "r2", Title: "Pasta Bolognese", CookTimeMinutes: 30, Feedback: &domain.Feedback{Liked: true}},
		{ID: "r1", Title: "Chicken Curry", CookTimeMinutes: 25},
	}
}

func TestArchiveShowListsNewestFirstWithIconsAndCook(t *testing.T) {
	lister := &stubArchiveLister{recipes: archiveRecipes()}
	srv := newArchiveRouter(t, lister, stubRecipeLoader{}, &stubCookAgain{}, true)

	req := httptest.NewRequest(http.MethodGet, "/archive", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	iP := strings.Index(body, "Pasta Bolognese")
	iC := strings.Index(body, "Chicken Curry")
	if iP < 0 || iC < 0 {
		t.Fatalf("body missing recipe titles:\n%s", body)
	}
	if iP > iC {
		t.Error("recipes not rendered newest-first (Pasta should precede Chicken)")
	}
	if !strings.Contains(body, "👍") {
		t.Error("liked recipe missing its feedback icon")
	}
	if !strings.Contains(body, `hx-get="/archive/cook-again/r2"`) {
		t.Errorf("cook-again control missing when canCook is true:\n%s", body)
	}
}

func TestArchiveShowHidesCookWhenDisabled(t *testing.T) {
	lister := &stubArchiveLister{recipes: archiveRecipes()}
	srv := newArchiveRouter(t, lister, stubRecipeLoader{}, nil, false)

	req := httptest.NewRequest(http.MethodGet, "/archive", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "/archive/cook-again/") {
		t.Error("cook-again control rendered while canCook is false")
	}
}

func TestArchiveShowDegradesOnReadError(t *testing.T) {
	lister := &stubArchiveLister{err: errors.New("db down")}
	srv := newArchiveRouter(t, lister, stubRecipeLoader{}, &stubCookAgain{}, true)

	req := httptest.NewRequest(http.MethodGet, "/archive", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (graceful degradation, not 500)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "load the archive") {
		t.Errorf("body missing degradation banner:\n%s", rec.Body.String())
	}
}

func TestArchiveSearchReturnsListFragment(t *testing.T) {
	lister := &stubArchiveLister{recipes: archiveRecipes()}
	srv := newArchiveRouter(t, lister, stubRecipeLoader{}, &stubCookAgain{}, true)

	req := httptest.NewRequest(http.MethodGet, "/archive/search?q=pasta", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if lister.gotQuery != "pasta" {
		t.Errorf("query forwarded = %q, want pasta", lister.gotQuery)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="archive-list"`) {
		t.Errorf("response is not the list fragment:\n%s", body)
	}
}

func TestArchiveCookAgainDialogNoPlan(t *testing.T) {
	cook := &stubCookAgain{plan: nil}
	srv := newArchiveRouter(t, &stubArchiveLister{}, stubRecipeLoader{}, cook, true)

	req := httptest.NewRequest(http.MethodGet, "/archive/cook-again/r1", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No active week") {
		t.Errorf("body missing no-active-week state:\n%s", rec.Body.String())
	}
}

func TestArchiveCookAgainDialogListsCurrentRecipes(t *testing.T) {
	cook := &stubCookAgain{plan: &domain.WeeklyPlan{ID: "p1", RecipeIDs: []string{"a", "b", "c"}}}
	plans := stubRecipeLoader{byID: map[string]domain.Recipe{
		"a": {ID: "a", Title: "Keep A"},
		"b": {ID: "b", Title: "Keep B"},
		"c": {ID: "c", Title: "Keep C"},
	}}
	srv := newArchiveRouter(t, &stubArchiveLister{}, plans, cook, true)

	req := httptest.NewRequest(http.MethodGet, "/archive/cook-again/src", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Replace which recipe?", "Keep A", "Keep B", "Keep C", `name="old"`, `hx-post="/archive/cook-again/src"`} {
		if !strings.Contains(body, want) {
			t.Errorf("dialog body missing %q:\n%s", want, body)
		}
	}
}

func TestArchiveCookAgainSuccess(t *testing.T) {
	cook := &stubCookAgain{
		plan:    &domain.WeeklyPlan{ID: "p1", RecipeIDs: []string{"old", "b", "c"}},
		swapped: &service.SwappedRecipe{Recipe: domain.Recipe{ID: "new", Title: "Pasta Bolognese"}},
	}
	srv := newArchiveRouter(t, &stubArchiveLister{}, stubRecipeLoader{}, cook, true)

	form := url.Values{"old": {"old"}}
	req := httptest.NewRequest(http.MethodPost, "/archive/cook-again/src", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if cook.cookCalls != 1 || cook.gotOld != "old" || cook.gotSource != "src" {
		t.Fatalf("CookAgain called %d times with (old=%q, source=%q), want 1 (old, src)", cook.cookCalls, cook.gotOld, cook.gotSource)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Added to this week") || !strings.Contains(body, "Pasta Bolognese") {
		t.Errorf("body missing confirmation with added title:\n%s", body)
	}
}

func TestArchiveCookAgainServiceErrorRendersFriendly(t *testing.T) {
	cook := &stubCookAgain{
		plan:    &domain.WeeklyPlan{ID: "p1", RecipeIDs: []string{"old", "b", "c"}},
		cookErr: errors.New("boom"),
	}
	srv := newArchiveRouter(t, &stubArchiveLister{}, stubRecipeLoader{}, cook, true)

	form := url.Values{"old": {"old"}}
	req := httptest.NewRequest(http.MethodPost, "/archive/cook-again/src", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (HTMX-swappable error)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "add the recipe to this week") {
		t.Errorf("body missing localized error:\n%s", rec.Body.String())
	}
}

func TestArchiveCookAgainNoPlanRendersDialog(t *testing.T) {
	cook := &stubCookAgain{plan: nil}
	srv := newArchiveRouter(t, &stubArchiveLister{}, stubRecipeLoader{}, cook, true)

	form := url.Values{"old": {"old"}}
	req := httptest.NewRequest(http.MethodPost, "/archive/cook-again/src", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if cook.cookCalls != 0 {
		t.Error("CookAgain should not run without an active plan")
	}
	if !strings.Contains(rec.Body.String(), "No active week") {
		t.Errorf("body missing no-active-week state:\n%s", rec.Body.String())
	}
}
