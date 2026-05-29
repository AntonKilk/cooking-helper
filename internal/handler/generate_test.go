package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/service"
)

// stubHouseholds satisfies householdProfiles for the generate handler tests.
type stubHouseholds struct{}

func (stubHouseholds) Current(_ context.Context, lang domain.Language) (*domain.HouseholdProfile, error) {
	return &domain.HouseholdProfile{ID: "hh-1", Language: lang, FamilySize: domain.FamilySize{Adults: 2}}, nil
}

func (stubHouseholds) UpdateProfile(_ context.Context, _ string, _ domain.Language, _, _ int) (*domain.HouseholdProfile, error) {
	return nil, nil
}

func (stubHouseholds) AddPantryBasic(_ context.Context, _, _ string) (*domain.HouseholdProfile, error) {
	return nil, nil
}

func (stubHouseholds) RemovePantryBasic(_ context.Context, _, _ string) (*domain.HouseholdProfile, error) {
	return nil, nil
}

// stubGenerator returns canned data for the handler tests. The same stub serves
// both GenerateWeek and the swap path: SwapRecipe returns swapped/err and
// CurrentPlan returns plan/planErr.
type stubGenerator struct {
	week     *service.GeneratedWeek
	err      error
	plan     *domain.WeeklyPlan
	planErr  error
	swapped  *service.SwappedRecipe
	swapErr  error
	swapCall struct {
		old string
	}
}

func (s *stubGenerator) GenerateWeek(_ context.Context, _ *domain.HouseholdProfile) (*service.GeneratedWeek, error) {
	return s.week, s.err
}

func (s *stubGenerator) CurrentPlan(_ context.Context, _ string) (*domain.WeeklyPlan, error) {
	return s.plan, s.planErr
}

func (s *stubGenerator) SwapRecipe(_ context.Context, _ *domain.HouseholdProfile, _ *domain.WeeklyPlan, oldID string) (*service.SwappedRecipe, error) {
	s.swapCall.old = oldID
	return s.swapped, s.swapErr
}

// stubRecipeLoader serves canned kept recipes for the swap handler.
type stubRecipeLoader struct {
	byID map[string]domain.Recipe
	err  error
}

func (s stubRecipeLoader) RecipesByIDs(_ context.Context, ids []string) ([]domain.Recipe, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([]domain.Recipe, 0, len(ids))
	for _, id := range ids {
		if r, ok := s.byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func newGenerateHandler(t *testing.T, gen weekGenerator) *generateHandlers {
	t.Helper()
	rd := &renderer{tmpl: testTemplates(t), bundle: testBundle(t)}
	return &generateHandlers{rd: rd, households: stubHouseholds{}, gen: gen, recipes: stubRecipeLoader{}}
}

func newGenerateHandlerWith(t *testing.T, gen weekGenerator, recipes recipeLoader) *generateHandlers {
	t.Helper()
	rd := &renderer{tmpl: testTemplates(t), bundle: testBundle(t)}
	return &generateHandlers{rd: rd, households: stubHouseholds{}, gen: gen, recipes: recipes}
}

func postGenerate() (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(http.MethodPost, "/generate", nil)
	req.Header.Set("HX-Request", "true")
	return httptest.NewRecorder(), req
}

func TestGenerateRendersCards(t *testing.T) {
	week := &service.GeneratedWeek{
		Recipes: []domain.Recipe{
			{ID: "r1", Title: "Chicken Pasta", Description: "Creamy", CookTimeMinutes: 25},
			{ID: "r2", Title: "Beef Tacos", Description: "Spicy", CookTimeMinutes: 20},
			{ID: "r3", Title: "Salmon Bowl", Description: "Fresh", CookTimeMinutes: 30},
		},
		Proteins: []string{"poultry", "red_meat", "fish"},
	}
	gh := newGenerateHandler(t, &stubGenerator{week: week})
	rec, req := postGenerate()

	gh.Generate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Chicken Pasta", "Beef Tacos", "Salmon Bowl",
		"🍗", "🥩", "🐟", "25 min",
		"Replace", "Regenerate all",
		`hx-post="/generate/swap/r1"`,
		`hx-post="/generate/swap/r2"`,
		`hx-post="/generate"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(strings.ToLower(body), "<!doctype") {
		t.Errorf("expected a fragment, got a full page:\n%s", body)
	}
}

func postSwap(recipeID string) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(http.MethodPost, "/generate/swap/"+recipeID, nil)
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("recipeID", recipeID)
	return httptest.NewRecorder(), req
}

func TestSwapRendersCards(t *testing.T) {
	plan := &domain.WeeklyPlan{
		ID:        "plan-1",
		RecipeIDs: []string{"r1", "r-new", "r3"}, // post-swap order
	}
	swapped := &service.SwappedRecipe{
		Plan: plan,
		Recipe: domain.Recipe{
			ID: "r-new", Title: "Beef Tacos", Description: "Spicy", CookTimeMinutes: 20,
			Ingredients: []domain.Ingredient{{Name: "beef"}, {Name: "tortilla"}},
		},
		Protein: "red_meat",
	}
	gen := &stubGenerator{plan: plan, swapped: swapped}
	recipes := stubRecipeLoader{byID: map[string]domain.Recipe{
		"r1": {ID: "r1", Title: "Chicken Pasta", Description: "Creamy", CookTimeMinutes: 25,
			Ingredients: []domain.Ingredient{{Name: "chicken"}, {Name: "pasta"}}},
		"r3": {ID: "r3", Title: "Salmon Bowl", Description: "Fresh", CookTimeMinutes: 30,
			Ingredients: []domain.Ingredient{{Name: "salmon"}, {Name: "rice"}}},
	}}
	gh := newGenerateHandlerWith(t, gen, recipes)
	rec, req := postSwap("r2")

	gh.Swap(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gen.swapCall.old != "r2" {
		t.Fatalf("SwapRecipe got oldID = %q, want r2", gen.swapCall.old)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Chicken Pasta", "Beef Tacos", "Salmon Bowl",
		"🍗", "🥩", "🐟", // emojis inferred for kept + reported for new
		"Replace", "Regenerate all",
		`hx-post="/generate/swap/r-new"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestSwapRendersFriendlyErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"dislike", service.ErrDislikeViolation, "disliked ingredient"},
		{"portions", service.ErrPortionsShort, "whole week"},
		{"variety", service.ErrProteinVariety, "varied enough"},
		{"generic", service.ErrGenerationInvalid, "Something went wrong"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan := &domain.WeeklyPlan{ID: "plan-1", RecipeIDs: []string{"r1", "r2", "r3"}}
			gen := &stubGenerator{plan: plan, swapErr: c.err}
			gh := newGenerateHandler(t, gen)
			rec, req := postSwap("r2")

			gh.Swap(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (HTMX swaps the fragment)", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, c.want) {
				t.Errorf("body missing %q:\n%s", c.want, body)
			}
			if strings.Contains(body, "service:") {
				t.Errorf("error fragment leaked internal detail:\n%s", body)
			}
		})
	}
}

func TestSwapNoActivePlanRendersError(t *testing.T) {
	gen := &stubGenerator{plan: nil} // no current plan
	gh := newGenerateHandler(t, gen)
	rec, req := postSwap("r2")

	gh.Swap(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Something went wrong") {
		t.Errorf("body missing generic error message:\n%s", body)
	}
}

func TestSwapMissingRecipeIDRendersError(t *testing.T) {
	gh := newGenerateHandler(t, &stubGenerator{})
	rec, req := postSwap("")

	gh.Swap(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Something went wrong") {
		t.Errorf("body missing generic error:\n%s", rec.Body.String())
	}
}

func TestGenerateRendersFriendlyErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string // a fragment of the localized EN message
	}{
		{"dislike", service.ErrDislikeViolation, "disliked ingredient"},
		{"portions", service.ErrPortionsShort, "whole week"},
		{"variety", service.ErrProteinVariety, "varied enough"},
		{"generic", service.ErrGenerationInvalid, "Something went wrong"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gh := newGenerateHandler(t, &stubGenerator{err: c.err})
			rec, req := postGenerate()

			gh.Generate(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (HTMX swaps the fragment)", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, c.want) {
				t.Errorf("body missing %q:\n%s", c.want, body)
			}
			// Internal error detail must never leak to the client.
			if strings.Contains(body, "service:") {
				t.Errorf("error fragment leaked internal detail:\n%s", body)
			}
		})
	}
}
