package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/llm"
)

// fakeLLM returns canned completions in sequence, one per Complete call, so a test
// can script a first reply and a retry reply.
type fakeLLM struct {
	replies []string
	calls   int
}

func (f *fakeLLM) Complete(_ context.Context, _ llm.Request) (llm.Completion, error) {
	i := f.calls
	f.calls++
	if i >= len(f.replies) {
		i = len(f.replies) - 1
	}
	return llm.Completion{Text: f.replies[i]}, nil
}

// fakeGenRepo records the persisted week and serves canned history.
type fakeGenRepo struct {
	recent   []domain.Recipe
	saved    *domain.WeeklyPlan
	savedRec []domain.Recipe
}

func (r *fakeGenRepo) RecentRecipes(_ context.Context, _ string, _ int) ([]domain.Recipe, error) {
	return r.recent, nil
}

func (r *fakeGenRepo) CreateWeekWithRecipes(_ context.Context, p *domain.WeeklyPlan, recipes []domain.Recipe) error {
	ids := make([]string, len(recipes))
	for i := range recipes {
		recipes[i].ID = "rec-" + string(rune('a'+i))
		ids[i] = recipes[i].ID
	}
	p.ID = "plan-1"
	p.RecipeIDs = ids
	r.saved = p
	r.savedRec = recipes
	return nil
}

// recipeJSON builds one DTO recipe.
func recipeJSON(title, protein string, servings int, ingredients ...string) map[string]any {
	ings := make([]map[string]any, len(ingredients))
	for i, name := range ingredients {
		ings[i] = map[string]any{"name": name, "amount": 1, "unit": "kpl", "category": "other"}
	}
	return map[string]any{
		"title": title, "description": "desc", "cook_time_minutes": 25,
		"servings": servings, "protein": protein, "ingredients": ings, "steps": []string{"step 1"},
	}
}

func weekJSON(recipes ...map[string]any) string {
	b, _ := json.Marshal(map[string]any{"recipes": recipes})
	return string(b)
}

func testHousehold() *domain.HouseholdProfile {
	return &domain.HouseholdProfile{
		ID:         "hh-1",
		Language:   domain.LanguageEN,
		FamilySize: domain.FamilySize{Adults: 2, Kids: 0}, // target = 7*2 = 14 portions
	}
}

func validWeek() string {
	return weekJSON(
		recipeJSON("Chicken Pasta", "poultry", 5, "chicken", "pasta"),
		recipeJSON("Beef Tacos", "red_meat", 5, "beef", "tortilla"),
		recipeJSON("Salmon Bowl", "fish", 5, "salmon", "rice"),
	) // 15 servings ≥ 14; 3 protein categories
}

func TestGenerateWeekHappyPath(t *testing.T) {
	repo := &fakeGenRepo{}
	svc := NewGenerationService(&fakeLLM{replies: []string{validWeek()}}, repo)

	got, err := svc.GenerateWeek(context.Background(), testHousehold())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(got.Recipes) != 3 {
		t.Fatalf("recipes = %d, want 3", len(got.Recipes))
	}
	if got.Plan == nil || got.Plan.ID == "" {
		t.Fatal("plan not persisted")
	}
	if repo.saved == nil || len(repo.savedRec) != 3 {
		t.Fatal("week was not persisted via CreateWeekWithRecipes")
	}
	if got.Recipes[0].Source != domain.SourceLLM || got.Recipes[0].Language != domain.LanguageEN {
		t.Fatalf("recipe metadata wrong: %+v", got.Recipes[0])
	}
	if want := []string{"poultry", "red_meat", "fish"}; !equalStrings(got.Proteins, want) {
		t.Fatalf("proteins = %v, want %v", got.Proteins, want)
	}
}

func TestGenerateWeekDislikeRetrySucceeds(t *testing.T) {
	repo := &fakeGenRepo{}
	bad := weekJSON(
		recipeJSON("Mushroom Pasta", "vegetarian", 5, "mushroom", "pasta"),
		recipeJSON("Beef Tacos", "red_meat", 5, "beef", "tortilla"),
		recipeJSON("Salmon Bowl", "fish", 5, "salmon", "rice"),
	)
	svc := NewGenerationService(&fakeLLM{replies: []string{bad, validWeek()}}, repo)

	h := testHousehold()
	h.DislikedIngredients = []string{"mushroom"}

	got, err := svc.GenerateWeek(context.Background(), h)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if got.Recipes[0].Title != "Chicken Pasta" {
		t.Fatalf("retry result not used: %+v", got.Recipes[0])
	}
}

func TestGenerateWeekDislikePersistsFails(t *testing.T) {
	repo := &fakeGenRepo{}
	bad := weekJSON(
		recipeJSON("Mushroom Pasta", "vegetarian", 5, "Fresh Mushrooms", "pasta"),
		recipeJSON("Beef Tacos", "red_meat", 5, "beef", "tortilla"),
		recipeJSON("Salmon Bowl", "fish", 5, "salmon", "rice"),
	)
	// Both attempts violate (case-insensitive substring "mushroom").
	svc := NewGenerationService(&fakeLLM{replies: []string{bad, bad}}, repo)

	h := testHousehold()
	h.DislikedIngredients = []string{"MUSHROOM"}

	_, err := svc.GenerateWeek(context.Background(), h)
	if !errors.Is(err, ErrDislikeViolation) {
		t.Fatalf("err = %v, want ErrDislikeViolation", err)
	}
	if repo.saved != nil {
		t.Fatal("must not persist on dislike violation")
	}
}

func TestGenerateWeekValidationErrors(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		want  error
	}{
		{
			name: "not three recipes",
			reply: weekJSON(
				recipeJSON("Only One", "fish", 14, "salmon"),
			),
			want: ErrGenerationInvalid,
		},
		{
			name: "portions short",
			reply: weekJSON(
				recipeJSON("A", "poultry", 2, "chicken"),
				recipeJSON("B", "red_meat", 2, "beef"),
				recipeJSON("C", "fish", 2, "salmon"),
			), // 6 < 14
			want: ErrPortionsShort,
		},
		{
			name: "single protein",
			reply: weekJSON(
				recipeJSON("A", "poultry", 5, "chicken"),
				recipeJSON("B", "poultry", 5, "chicken"),
				recipeJSON("C", "poultry", 5, "chicken"),
			),
			want: ErrProteinVariety,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeGenRepo{}
			svc := NewGenerationService(&fakeLLM{replies: []string{c.reply}}, repo)
			_, err := svc.GenerateWeek(context.Background(), testHousehold())
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if repo.saved != nil {
				t.Fatal("must not persist on validation failure")
			}
		})
	}
}

func TestGenerateWeekIncludesHistoryInPrompt(t *testing.T) {
	repo := &fakeGenRepo{
		recent: []domain.Recipe{
			{Title: "Old Stew", Feedback: &domain.Feedback{Liked: true, CookAgain: true}},
		},
	}
	llmClient := &capturingLLM{reply: validWeek()}
	svc := NewGenerationService(llmClient, repo)

	h := testHousehold()
	h.DislikedIngredients = []string{"olives"}
	h.PantryBasics = []string{"salt"}

	if _, err := svc.GenerateWeek(context.Background(), h); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(llmClient.lastPrompt, "Old Stew") {
		t.Errorf("trigger missing recent history:\n%s", llmClient.lastPrompt)
	}
	if !strings.Contains(llmClient.lastPrompt, "liked, cook again") {
		t.Errorf("trigger missing feedback tag:\n%s", llmClient.lastPrompt)
	}
	if !strings.Contains(llmClient.lastPrompt, "olives") {
		t.Errorf("trigger missing disliked list:\n%s", llmClient.lastPrompt)
	}
	if !strings.Contains(llmClient.lastPrompt, "14") {
		t.Errorf("trigger missing target portions:\n%s", llmClient.lastPrompt)
	}
	if !strings.Contains(llmClient.lastSystem, "OUTPUT CONTRACT") {
		t.Errorf("system block missing instructions:\n%s", llmClient.lastSystem)
	}
	if !strings.Contains(llmClient.lastSystem, "ruokaboksi") {
		t.Errorf("system block missing few-shot examples")
	}
}

// capturingLLM records the last request so prompt assembly can be asserted.
type capturingLLM struct {
	reply      string
	lastSystem string
	lastPrompt string
}

func (c *capturingLLM) Complete(_ context.Context, req llm.Request) (llm.Completion, error) {
	c.lastSystem = req.System
	c.lastPrompt = req.Prompt
	return llm.Completion{Text: c.reply}, nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
