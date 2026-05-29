package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/llm"
	"github.com/AntonKilk/cooking-helper/internal/repository"
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

// fakeGenRepo records the persisted week, the previous plan ID passed to
// archive-and-create, and the most recent swap call, and serves canned data
// (history, current plan, kept recipes) back to the service.
type fakeGenRepo struct {
	recent          []domain.Recipe
	recentLimitSeen int
	saved           *domain.WeeklyPlan
	savedRec        []domain.Recipe
	archivedPrev    string
	currentPlan     *domain.WeeklyPlan
	keptByID        map[string]domain.Recipe
	swapPlanID      string
	swapOldID       string
	swapNewRecipe   *domain.Recipe
	swapItems       []domain.ShoppingListItem
}

func (r *fakeGenRepo) RecentRecipes(_ context.Context, _ string, limit int) ([]domain.Recipe, error) {
	r.recentLimitSeen = limit
	return r.recent, nil
}

func (r *fakeGenRepo) CurrentWeeklyPlan(_ context.Context, _ string) (*domain.WeeklyPlan, error) {
	if r.currentPlan == nil {
		return nil, repository.ErrNotFound
	}
	return r.currentPlan, nil
}

func (r *fakeGenRepo) ArchiveAndCreateWeek(_ context.Context, previousPlanID string, p *domain.WeeklyPlan, recipes []domain.Recipe) error {
	r.archivedPrev = previousPlanID
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

func (r *fakeGenRepo) RecipesByIDs(_ context.Context, ids []string) ([]domain.Recipe, error) {
	out := make([]domain.Recipe, 0, len(ids))
	for _, id := range ids {
		rec, ok := r.keptByID[id]
		if !ok {
			return nil, repository.ErrNotFound
		}
		out = append(out, rec)
	}
	return out, nil
}

func (r *fakeGenRepo) SwapRecipeInPlan(_ context.Context, planID, oldRecipeID string, newRecipe *domain.Recipe, items []domain.ShoppingListItem) error {
	r.swapPlanID = planID
	r.swapOldID = oldRecipeID
	if newRecipe.ID == "" {
		newRecipe.ID = "rec-swap"
	}
	r.swapNewRecipe = newRecipe
	r.swapItems = items
	return nil
}

// fakeBuilder is a scriptable shopping-list builder. It records its last input so
// swap/generation tests can assert which recipes + pantry were consolidated, and
// returns canned items (defaulting to an empty list).
type fakeBuilder struct {
	items      []domain.ShoppingListItem
	err        error
	gotRecipes []domain.Recipe
	gotPantry  []string
	calls      int
}

func (b *fakeBuilder) Build(_ context.Context, recipes []domain.Recipe, pantry []string) ([]domain.ShoppingListItem, error) {
	b.calls++
	b.gotRecipes = recipes
	b.gotPantry = pantry
	if b.err != nil {
		return nil, b.err
	}
	return b.items, nil
}

// newTestGenService wires a generation service with a default (empty) shopping
// builder, so the existing tests need not care about list building.
func newTestGenService(client llm.Client, repo generationRepo) *GenerationService {
	return NewGenerationService(client, repo, &fakeBuilder{})
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
	svc := newTestGenService(&fakeLLM{replies: []string{validWeek()}}, repo)

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
	svc := newTestGenService(&fakeLLM{replies: []string{bad, validWeek()}}, repo)

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

func TestGenerateWeekDislikeRetrySucceedsOnSecondRetry(t *testing.T) {
	repo := &fakeGenRepo{}
	badFirst := weekJSON(
		recipeJSON("Mushroom Pasta", "vegetarian", 5, "mushroom", "pasta"),
		recipeJSON("Beef Tacos", "red_meat", 5, "beef", "tortilla"),
		recipeJSON("Salmon Bowl", "fish", 5, "salmon", "rice"),
	)
	badSecond := weekJSON(
		recipeJSON("Creamy Mushrooms", "vegetarian", 5, "Fresh Mushrooms", "cream"),
		recipeJSON("Beef Tacos", "red_meat", 5, "beef", "tortilla"),
		recipeJSON("Salmon Bowl", "fish", 5, "salmon", "rice"),
	)
	svc := newTestGenService(&fakeLLM{replies: []string{badFirst, badSecond, validWeek()}}, repo)

	h := testHousehold()
	h.DislikedIngredients = []string{"mushroom"}

	got, err := svc.GenerateWeek(context.Background(), h)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if got.Recipes[0].Title != "Chicken Pasta" {
		t.Fatalf("retry result not used: %+v", got.Recipes[0])
	}
	if repo.saved == nil {
		t.Fatal("must persist after successful retry")
	}
}

func TestGenerateWeekDislikePersistsFails(t *testing.T) {
	repo := &fakeGenRepo{}
	bad := weekJSON(
		recipeJSON("Mushroom Pasta", "vegetarian", 5, "Fresh Mushrooms", "pasta"),
		recipeJSON("Beef Tacos", "red_meat", 5, "beef", "tortilla"),
		recipeJSON("Salmon Bowl", "fish", 5, "salmon", "rice"),
	)
	// All three attempts (initial + maxDislikeRetries) violate.
	svc := newTestGenService(&fakeLLM{replies: []string{bad, bad, bad}}, repo)

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

func TestGenerateWeekDislikeInflection(t *testing.T) {
	cases := []struct {
		name             string
		disliked         string
		badIngredient    string
		offendingRecipe  string
		offendingProtein string
	}{
		{
			name:             "russian inflection",
			disliked:         "гриб",
			badIngredient:    "грибы",
			offendingRecipe:  "Грибной соус",
			offendingProtein: "vegetarian",
		},
		{
			name:             "finnish stem mutation",
			disliked:         "sieni",
			badIngredient:    "sienet",
			offendingRecipe:  "Sienikastike",
			offendingProtein: "vegetarian",
		},
		{
			name:             "english diacritic",
			disliked:         "creme fraiche",
			badIngredient:    "Crème Fraîche",
			offendingRecipe:  "Pasta in Cream",
			offendingProtein: "vegetarian",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeGenRepo{}
			bad := weekJSON(
				recipeJSON(c.offendingRecipe, c.offendingProtein, 5, c.badIngredient, "pasta"),
				recipeJSON("Beef Tacos", "red_meat", 5, "beef", "tortilla"),
				recipeJSON("Salmon Bowl", "fish", 5, "salmon", "rice"),
			)
			svc := newTestGenService(&fakeLLM{replies: []string{bad, validWeek()}}, repo)

			h := testHousehold()
			h.DislikedIngredients = []string{c.disliked}

			got, err := svc.GenerateWeek(context.Background(), h)
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			if got.Recipes[0].Title != "Chicken Pasta" {
				t.Fatalf("retry not triggered for %s; title = %q", c.name, got.Recipes[0].Title)
			}
		})
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
			svc := newTestGenService(&fakeLLM{replies: []string{c.reply}}, repo)
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
	svc := newTestGenService(llmClient, repo)

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

func TestGenerateWeekRendersDislikedHistory(t *testing.T) {
	repo := &fakeGenRepo{
		recent: []domain.Recipe{
			{Title: "Liver Casserole", Feedback: &domain.Feedback{Disliked: true}},
		},
	}
	llmClient := &capturingLLM{reply: validWeek()}
	svc := newTestGenService(llmClient, repo)

	if _, err := svc.GenerateWeek(context.Background(), testHousehold()); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(llmClient.lastPrompt, "Liver Casserole") {
		t.Errorf("trigger missing disliked recipe title:\n%s", llmClient.lastPrompt)
	}
	if !strings.Contains(llmClient.lastPrompt, "do not make this again") {
		t.Errorf("trigger missing disliked do-not-repeat instruction:\n%s", llmClient.lastPrompt)
	}
}

func TestGenerateWeekDefaultRecentLimit(t *testing.T) {
	repo := &fakeGenRepo{}
	svc := newTestGenService(&fakeLLM{replies: []string{validWeek()}}, repo)

	if _, err := svc.GenerateWeek(context.Background(), testHousehold()); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if repo.recentLimitSeen != defaultRecentLimit {
		t.Fatalf("recentLimitSeen = %d, want default %d", repo.recentLimitSeen, defaultRecentLimit)
	}
}

func TestGenerateWeekUsesConfiguredRecentLimit(t *testing.T) {
	repo := &fakeGenRepo{}
	svc := NewGenerationService(&fakeLLM{replies: []string{validWeek()}}, repo, &fakeBuilder{}, WithRecentLimit(20))

	if _, err := svc.GenerateWeek(context.Background(), testHousehold()); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if repo.recentLimitSeen != 20 {
		t.Fatalf("recentLimitSeen = %d, want 20", repo.recentLimitSeen)
	}
}

func TestWithRecentLimitIgnoresNonPositive(t *testing.T) {
	repo := &fakeGenRepo{}
	svc := NewGenerationService(&fakeLLM{replies: []string{validWeek()}}, repo, &fakeBuilder{}, WithRecentLimit(0))

	if _, err := svc.GenerateWeek(context.Background(), testHousehold()); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if repo.recentLimitSeen != defaultRecentLimit {
		t.Fatalf("recentLimitSeen = %d, want default %d (non-positive ignored)", repo.recentLimitSeen, defaultRecentLimit)
	}
}

// capturingLLM records each request so prompt assembly can be asserted; the
// last* fields hold the most recent call's values for legacy assertions.
type capturingLLM struct {
	reply      string
	replies    []string
	calls      int
	lastSystem string
	lastPrompt string
}

func (c *capturingLLM) Complete(_ context.Context, req llm.Request) (llm.Completion, error) {
	c.lastSystem = req.System
	c.lastPrompt = req.Prompt
	if len(c.replies) > 0 {
		i := c.calls
		c.calls++
		if i >= len(c.replies) {
			i = len(c.replies) - 1
		}
		return llm.Completion{Text: c.replies[i]}, nil
	}
	c.calls++
	return llm.Completion{Text: c.reply}, nil
}

func TestGenerateWeekArchivesPreviousPlan(t *testing.T) {
	repo := &fakeGenRepo{
		currentPlan: &domain.WeeklyPlan{ID: "prev-plan-99"},
	}
	svc := newTestGenService(&fakeLLM{replies: []string{validWeek()}}, repo)

	if _, err := svc.GenerateWeek(context.Background(), testHousehold()); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if repo.archivedPrev != "prev-plan-99" {
		t.Fatalf("archivedPrev = %q, want %q", repo.archivedPrev, "prev-plan-99")
	}
}

func TestGenerateWeekNoPreviousPlan(t *testing.T) {
	repo := &fakeGenRepo{} // currentPlan = nil → repository.ErrNotFound
	svc := newTestGenService(&fakeLLM{replies: []string{validWeek()}}, repo)

	if _, err := svc.GenerateWeek(context.Background(), testHousehold()); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if repo.archivedPrev != "" {
		t.Fatalf("archivedPrev = %q, want empty (no previous plan)", repo.archivedPrev)
	}
}

func TestCurrentPlanNoActiveReturnsNil(t *testing.T) {
	svc := newTestGenService(&fakeLLM{replies: []string{""}}, &fakeGenRepo{})
	got, err := svc.CurrentPlan(context.Background(), "hh-1")
	if err != nil {
		t.Fatalf("current plan: %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

func swapKeptHousehold(t *testing.T) (*domain.HouseholdProfile, *domain.WeeklyPlan, *fakeGenRepo) {
	t.Helper()
	h := testHousehold()
	kept := map[string]domain.Recipe{
		"keep-A": {
			ID: "keep-A", Title: "Chicken Pasta", Servings: 5,
			Ingredients: []domain.Ingredient{{Name: "chicken"}, {Name: "pasta"}},
			Description: "Creamy chicken pasta",
		},
		"keep-C": {
			ID: "keep-C", Title: "Salmon Bowl", Servings: 5,
			Ingredients: []domain.Ingredient{{Name: "salmon"}, {Name: "rice"}},
			Description: "Fresh salmon rice bowl",
		},
	}
	plan := &domain.WeeklyPlan{
		ID:          "plan-99",
		HouseholdID: h.ID,
		RecipeIDs:   []string{"keep-A", "old-B", "keep-C"},
	}
	repo := &fakeGenRepo{keptByID: kept}
	return h, plan, repo
}

// singleRecipeJSON builds a swap-reply JSON wrapper.
func singleRecipeJSON(title, protein string, servings int, ingredients ...string) string {
	r := recipeJSON(title, protein, servings, ingredients...)
	b, _ := json.Marshal(map[string]any{"recipe": r})
	return string(b)
}

func TestSwapRecipeHappyPath(t *testing.T) {
	h, plan, repo := swapKeptHousehold(t)
	reply := singleRecipeJSON("Beef Tacos", "red_meat", 4, "beef", "tortilla") // target = 14 - (5+5) = 4
	svc := newTestGenService(&fakeLLM{replies: []string{reply}}, repo)

	got, err := svc.SwapRecipe(context.Background(), h, plan, "old-B")
	if err != nil {
		t.Fatalf("swap: %v", err)
	}
	if got.Recipe.Title != "Beef Tacos" {
		t.Fatalf("title = %q, want Beef Tacos", got.Recipe.Title)
	}
	if got.Protein != "red_meat" {
		t.Fatalf("protein = %q, want red_meat", got.Protein)
	}
	if repo.swapPlanID != "plan-99" || repo.swapOldID != "old-B" {
		t.Fatalf("swap call = (plan=%q, old=%q), want (plan-99, old-B)", repo.swapPlanID, repo.swapOldID)
	}
	if plan.RecipeIDs[1] != got.Recipe.ID {
		t.Fatalf("plan.RecipeIDs[1] = %q, want %q (in-place rotation)", plan.RecipeIDs[1], got.Recipe.ID)
	}
}

func TestSwapRecipeUnknownOldID(t *testing.T) {
	h, plan, repo := swapKeptHousehold(t)
	svc := newTestGenService(&fakeLLM{replies: []string{""}}, repo)

	if _, err := svc.SwapRecipe(context.Background(), h, plan, "nope"); !errors.Is(err, ErrGenerationInvalid) {
		t.Fatalf("err = %v, want ErrGenerationInvalid", err)
	}
	if repo.swapPlanID != "" {
		t.Fatal("must not call repo.SwapRecipeInPlan when oldID is unknown")
	}
}

func TestSwapRecipeDislikeRetrySucceeds(t *testing.T) {
	h, plan, repo := swapKeptHousehold(t)
	h.DislikedIngredients = []string{"mushroom"}
	bad := singleRecipeJSON("Mushroom Bowl", "vegetarian", 4, "Fresh Mushrooms", "rice")
	good := singleRecipeJSON("Beef Tacos", "red_meat", 4, "beef", "tortilla")
	llmClient := &capturingLLM{replies: []string{bad, good}}
	svc := newTestGenService(llmClient, repo)

	got, err := svc.SwapRecipe(context.Background(), h, plan, "old-B")
	if err != nil {
		t.Fatalf("swap: %v", err)
	}
	if got.Recipe.Title != "Beef Tacos" {
		t.Fatalf("retry result not used: %+v", got.Recipe)
	}
	if !strings.Contains(llmClient.lastPrompt, "FORBIDDEN") {
		t.Errorf("retry trigger missing FORBIDDEN hint:\n%s", llmClient.lastPrompt)
	}
}

func TestSwapRecipeDislikePersistsFails(t *testing.T) {
	h, plan, repo := swapKeptHousehold(t)
	h.DislikedIngredients = []string{"MUSHROOM"}
	bad := singleRecipeJSON("Mushroom Bowl", "vegetarian", 4, "Fresh Mushrooms", "rice")
	svc := newTestGenService(&fakeLLM{replies: []string{bad, bad}}, repo)

	if _, err := svc.SwapRecipe(context.Background(), h, plan, "old-B"); !errors.Is(err, ErrDislikeViolation) {
		t.Fatalf("err = %v, want ErrDislikeViolation", err)
	}
	if repo.swapPlanID != "" {
		t.Fatal("must not persist on dislike violation")
	}
}

func TestSwapRecipePortionsShort(t *testing.T) {
	h, plan, repo := swapKeptHousehold(t)
	// target = 14 - 10 = 4 ; reply returns 2 — too few.
	reply := singleRecipeJSON("Tiny", "red_meat", 2, "beef")
	svc := newTestGenService(&fakeLLM{replies: []string{reply}}, repo)

	if _, err := svc.SwapRecipe(context.Background(), h, plan, "old-B"); !errors.Is(err, ErrPortionsShort) {
		t.Fatalf("err = %v, want ErrPortionsShort", err)
	}
}

func TestSwapRecipeProteinVariety(t *testing.T) {
	// Force kept recipes whose inferred protein is "poultry" for both, and
	// have the swap reply also report "poultry" — combined = 1 distinct.
	h := testHousehold()
	kept := map[string]domain.Recipe{
		"keep-A": {ID: "keep-A", Title: "Chicken Pasta", Servings: 5, Ingredients: []domain.Ingredient{{Name: "chicken"}}},
		"keep-C": {ID: "keep-C", Title: "Chicken Stew", Servings: 5, Ingredients: []domain.Ingredient{{Name: "chicken"}}},
	}
	plan := &domain.WeeklyPlan{ID: "plan-77", HouseholdID: h.ID, RecipeIDs: []string{"keep-A", "old-B", "keep-C"}}
	repo := &fakeGenRepo{keptByID: kept}
	reply := singleRecipeJSON("Chicken Curry", "poultry", 4, "chicken")
	svc := newTestGenService(&fakeLLM{replies: []string{reply}}, repo)

	if _, err := svc.SwapRecipe(context.Background(), h, plan, "old-B"); !errors.Is(err, ErrProteinVariety) {
		t.Fatalf("err = %v, want ErrProteinVariety", err)
	}
}

func TestSwapRecipeKeptContextInPrompt(t *testing.T) {
	h, plan, repo := swapKeptHousehold(t)
	llmClient := &capturingLLM{reply: singleRecipeJSON("Beef Tacos", "red_meat", 4, "beef", "tortilla")}
	svc := newTestGenService(llmClient, repo)

	if _, err := svc.SwapRecipe(context.Background(), h, plan, "old-B"); err != nil {
		t.Fatalf("swap: %v", err)
	}
	for _, want := range []string{"Chicken Pasta", "Salmon Bowl", "Replace ONE recipe"} {
		if !strings.Contains(llmClient.lastSystem+llmClient.lastPrompt, want) {
			t.Errorf("swap prompt missing %q\nSYSTEM:\n%s\nTRIGGER:\n%s",
				want, llmClient.lastSystem, llmClient.lastPrompt)
		}
	}
	// Few-shot Finnish examples must NOT be in the swap system block.
	if strings.Contains(llmClient.lastSystem, "ruokaboksi") {
		t.Errorf("swap system block should not include generate-week few-shot examples")
	}
	// Target servings = 14 - (5+5) = 4 must appear in the trigger.
	if !strings.Contains(llmClient.lastPrompt, "at least 4") {
		t.Errorf("trigger missing target servings:\n%s", llmClient.lastPrompt)
	}
}

func TestGenerateWeekAttachesShoppingList(t *testing.T) {
	repo := &fakeGenRepo{}
	builder := &fakeBuilder{items: []domain.ShoppingListItem{
		{Name: "chicken", Amount: 500, Unit: "g", Category: domain.CategoryMeatFish},
		{Name: "pasta", Amount: 400, Unit: "g", Category: domain.CategoryPantry},
	}}
	svc := NewGenerationService(&fakeLLM{replies: []string{validWeek()}}, repo, builder)

	h := testHousehold()
	h.PantryBasics = []string{"salt"}

	got, err := svc.GenerateWeek(context.Background(), h)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if builder.calls != 1 {
		t.Fatalf("builder calls = %d, want 1", builder.calls)
	}
	if len(builder.gotRecipes) != 3 {
		t.Fatalf("builder got %d recipes, want 3", len(builder.gotRecipes))
	}
	if !equalStrings(builder.gotPantry, []string{"salt"}) {
		t.Fatalf("builder pantry = %v, want [salt]", builder.gotPantry)
	}
	if len(got.Plan.ShoppingList) != 2 || got.Plan.ShoppingList[0].Name != "chicken" {
		t.Fatalf("plan shopping list not attached: %+v", got.Plan.ShoppingList)
	}
	if repo.saved == nil || len(repo.saved.ShoppingList) != 2 {
		t.Fatal("shopping list was not persisted with the plan")
	}
}

func TestGenerateWeekBuilderErrorFails(t *testing.T) {
	repo := &fakeGenRepo{}
	builder := &fakeBuilder{err: errors.New("categorize boom")}
	svc := NewGenerationService(&fakeLLM{replies: []string{validWeek()}}, repo, builder)

	if _, err := svc.GenerateWeek(context.Background(), testHousehold()); err == nil {
		t.Fatal("expected error when builder fails")
	}
	if repo.saved != nil {
		t.Fatal("must not persist when shopping-list build fails")
	}
}

func TestSwapRecipeRebuildsShoppingList(t *testing.T) {
	h, plan, repo := swapKeptHousehold(t)
	h.PantryBasics = []string{"oil"}
	builder := &fakeBuilder{items: []domain.ShoppingListItem{
		{Name: "beef", Amount: 1, Unit: "pcs", Category: domain.CategoryMeatFish},
	}}
	reply := singleRecipeJSON("Beef Tacos", "red_meat", 4, "beef", "tortilla")
	svc := NewGenerationService(&fakeLLM{replies: []string{reply}}, repo, builder)

	got, err := svc.SwapRecipe(context.Background(), h, plan, "old-B")
	if err != nil {
		t.Fatalf("swap: %v", err)
	}
	// Builder must consolidate the full new set: the two kept recipes + replacement.
	if len(builder.gotRecipes) != 3 {
		t.Fatalf("builder got %d recipes, want 3 (2 kept + new)", len(builder.gotRecipes))
	}
	if !equalStrings(builder.gotPantry, []string{"oil"}) {
		t.Fatalf("builder pantry = %v, want [oil]", builder.gotPantry)
	}
	// The rebuilt list must reach the repository and the returned plan.
	if len(repo.swapItems) != 1 || repo.swapItems[0].Name != "beef" {
		t.Fatalf("repo swap items = %+v, want rebuilt list", repo.swapItems)
	}
	if len(got.Plan.ShoppingList) != 1 || got.Plan.ShoppingList[0].Name != "beef" {
		t.Fatalf("returned plan shopping list = %+v, want rebuilt list", got.Plan.ShoppingList)
	}
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
