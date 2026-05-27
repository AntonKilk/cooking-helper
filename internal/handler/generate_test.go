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

// stubGenerator returns a canned week or error for the handler tests.
type stubGenerator struct {
	week *service.GeneratedWeek
	err  error
}

func (s stubGenerator) GenerateWeek(_ context.Context, _ *domain.HouseholdProfile) (*service.GeneratedWeek, error) {
	return s.week, s.err
}

func newGenerateHandler(t *testing.T, gen weekGenerator) *generateHandlers {
	t.Helper()
	rd := &renderer{tmpl: testTemplates(t), bundle: testBundle(t)}
	return &generateHandlers{rd: rd, households: stubHouseholds{}, gen: gen}
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
	gh := newGenerateHandler(t, stubGenerator{week: week})
	rec, req := postGenerate()

	gh.Generate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Chicken Pasta", "Beef Tacos", "Salmon Bowl", "🍗", "🥩", "🐟", "25 min"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(strings.ToLower(body), "<!doctype") {
		t.Errorf("expected a fragment, got a full page:\n%s", body)
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
			gh := newGenerateHandler(t, stubGenerator{err: c.err})
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
