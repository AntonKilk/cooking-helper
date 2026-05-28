package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/llm"
	"github.com/AntonKilk/cooking-helper/internal/llm/prompts"
	"github.com/AntonKilk/cooking-helper/internal/repository"
	"github.com/AntonKilk/cooking-helper/internal/shopping"
)

// Generation tuning. recentLimit bounds how much history feeds the prompt;
// generationTimeout caps the whole tap-to-result path (the provider client also
// applies its own per-call timeout); maxGenTokens / maxSwapTokens cap output
// size for week vs. single-recipe generation.
const (
	recentLimit       = 10
	generationTimeout = 45 * time.Second
	maxGenTokens      = 4096
	maxSwapTokens     = 2048
	triggerDelimiter  = "---TRIGGER---"
	// maxDislikeRetries bounds semantic retries when a disliked ingredient slips
	// through. Total LLM attempts in the dislike path = 1 + maxDislikeRetries.
	maxDislikeRetries = 2
)

// schemaHint is echoed into the repair prompt when the model's reply is not valid
// JSON. It mirrors the contract in generate_week.v1.txt.
const schemaHint = `{"recipes":[{"title":string,"description":string,"cook_time_minutes":int,` +
	`"servings":int,"protein":string,"ingredients":[{"name":string,"amount":number,"unit":string,"category":string}],"steps":[string]}]}`

// swapSchemaHint mirrors swap_recipe.v1.txt's single-recipe contract.
const swapSchemaHint = `{"recipe":{"title":string,"description":string,"cook_time_minutes":int,` +
	`"servings":int,"protein":string,"ingredients":[{"name":string,"amount":number,"unit":string,"category":string}],"steps":[string]}}`

// Generation sentinel errors let the handler map each hard-constraint failure to a
// distinct localized message without leaking provider or parsing detail.
var (
	// ErrGenerationInvalid means the model did not return exactly three recipes.
	ErrGenerationInvalid = errors.New("service: generation did not return three recipes")
	// ErrDislikeViolation means a disliked ingredient survived even one retry.
	ErrDislikeViolation = errors.New("service: generated week includes a disliked ingredient")
	// ErrPortionsShort means the recipes do not portion the whole week.
	ErrPortionsShort = errors.New("service: generated week does not cover the week's portions")
	// ErrProteinVariety means fewer than two distinct protein categories appeared.
	ErrProteinVariety = errors.New("service: generated week lacks protein variety")
)

// generationRepo is the slice of the repository the generation service needs,
// kept narrow so the service is unit-testable with a fake. *repository.Store
// satisfies it. CurrentWeeklyPlan must return repository.ErrNotFound when the
// household has no active plan; ArchiveAndCreateWeek archives the previous plan
// (when previousPlanID is non-empty) and inserts the new one in one tx;
// RecipesByIDs loads the kept recipes for a swap; SwapRecipeInPlan rotates the
// plan's recipe_ids and invalidates the shopping list atomically.
type generationRepo interface {
	RecentRecipes(ctx context.Context, householdID string, limit int) ([]domain.Recipe, error)
	CurrentWeeklyPlan(ctx context.Context, householdID string) (*domain.WeeklyPlan, error)
	ArchiveAndCreateWeek(ctx context.Context, previousPlanID string, p *domain.WeeklyPlan, recipes []domain.Recipe) error
	RecipesByIDs(ctx context.Context, ids []string) ([]domain.Recipe, error)
	SwapRecipeInPlan(ctx context.Context, planID, oldRecipeID string, newRecipe *domain.Recipe, items []domain.ShoppingListItem) error
}

// shoppingBuilder consolidates a week's recipes into a categorized shopping list.
// *ShoppingBuilder satisfies it; kept as an interface so the generation service is
// unit-testable with a fake.
type shoppingBuilder interface {
	Build(ctx context.Context, recipes []domain.Recipe, pantryBasics []string) ([]domain.ShoppingListItem, error)
}

// GeneratedWeek is what GenerateWeek returns to the handler: the persisted plan,
// the persisted recipes, and a parallel slice of normalized protein tags used to
// pick each card's emoji (the tag is transient and not stored on the recipe).
type GeneratedWeek struct {
	Plan     *domain.WeeklyPlan
	Recipes  []domain.Recipe
	Proteins []string
}

// SwappedRecipe is what SwapRecipe returns to the handler: the plan (with its
// recipe_ids already rotated to reflect the swap), the freshly-inserted recipe,
// and the new recipe's protein tag for the card emoji.
type SwappedRecipe struct {
	Plan    *domain.WeeklyPlan
	Recipe  domain.Recipe
	Protein string
}

// GenerationService turns a household profile into a weekly plan via the LLM,
// enforcing the hard constraints, building the consolidated shopping list, and
// persisting the result atomically.
type GenerationService struct {
	client  llm.Client
	repo    generationRepo
	builder shoppingBuilder
}

// NewGenerationService returns a service backed by the given LLM client, repo,
// and shopping-list builder.
func NewGenerationService(client llm.Client, repo generationRepo, builder shoppingBuilder) *GenerationService {
	return &GenerationService{client: client, repo: repo, builder: builder}
}

// triggerData fills the variable half of the generate_week prompt.
type triggerData struct {
	Language       string
	Adults         int
	Kids           int
	TargetPortions int
	Disliked       []string
	Pantry         []string
	Recent         []string
}

// swapTriggerData fills the variable half of the swap_recipe prompt.
type swapTriggerData struct {
	Language       string
	Adults         int
	Kids           int
	TargetServings int
	Disliked       []string
	Pantry         []string
	Kept           []string
	Recent         []string
}

// GenerateWeek asks the model for three recipes, validates the hard constraints
// (exactly three, dislikes excluded with one semantic retry, portions cover the
// week, ≥2 protein categories), then persists the plan and recipes in one
// transaction. The returned recipes carry the assigned IDs.
func (g *GenerationService) GenerateWeek(ctx context.Context, h *domain.HouseholdProfile) (*GeneratedWeek, error) {
	ctx, cancel := context.WithTimeout(ctx, generationTimeout)
	defer cancel()

	system, trigger, err := g.loadPrompt(h)
	if err != nil {
		return nil, fmt.Errorf("generate week: %w", err)
	}

	week, err := g.complete(ctx, system, trigger)
	if err != nil {
		return nil, fmt.Errorf("generate week: %w", err)
	}
	if len(week.Recipes) != 3 {
		return nil, fmt.Errorf("generate week: %w", ErrGenerationInvalid)
	}

	// Dislikes are a hard constraint with defense-in-depth: post-validate every
	// LLM reply, retry up to maxDislikeRetries times with escalating prompt
	// accent, and emit a structured warn line per violation so frequency is
	// observable from the logs. Fail closed once the budget is exhausted.
	for attempt := 1; ; attempt++ {
		bad := dislikeViolations(week, h.DislikedIngredients)
		if len(bad) == 0 {
			break
		}
		slog.Warn("dislike violation",
			"attempt", attempt,
			"terms", bad,
			"household_id", h.ID,
		)
		if attempt > maxDislikeRetries {
			return nil, fmt.Errorf("generate week: %w", ErrDislikeViolation)
		}
		final := attempt == maxDislikeRetries
		retryTrigger := trigger + dislikeHint(bad, final)
		week, err = g.complete(ctx, system, retryTrigger)
		if err != nil {
			return nil, fmt.Errorf("generate week (dislike retry): %w", err)
		}
		if len(week.Recipes) != 3 {
			return nil, fmt.Errorf("generate week: %w", ErrGenerationInvalid)
		}
	}

	if !portionsCovered(week, h.FamilySize) {
		return nil, fmt.Errorf("generate week: %w", ErrPortionsShort)
	}
	if !hasProteinVariety(week) {
		return nil, fmt.Errorf("generate week: %w", ErrProteinVariety)
	}

	recipes, proteins := toDomainRecipes(week, h)
	plan := &domain.WeeklyPlan{HouseholdID: h.ID, WeekStart: mondayOf(time.Now().UTC())}

	plan.ShoppingList, err = g.builder.Build(ctx, recipes, h.PantryBasics)
	if err != nil {
		return nil, fmt.Errorf("generate week: %w", err)
	}

	previousID, err := g.currentPlanID(ctx, h.ID)
	if err != nil {
		return nil, fmt.Errorf("generate week: %w", err)
	}
	if err := g.repo.ArchiveAndCreateWeek(ctx, previousID, plan, recipes); err != nil {
		return nil, fmt.Errorf("generate week: %w", err)
	}

	return &GeneratedWeek{Plan: plan, Recipes: recipes, Proteins: proteins}, nil
}

// CurrentPlan returns the household's currently-active weekly plan, or nil
// when none exists. The handler calls this before issuing a swap.
func (g *GenerationService) CurrentPlan(ctx context.Context, householdID string) (*domain.WeeklyPlan, error) {
	plan, err := g.repo.CurrentWeeklyPlan(ctx, householdID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("current plan: %w", err)
	}
	return plan, nil
}

// currentPlanID returns the household's active plan ID, or "" when there is no
// previous plan to archive. Other repository errors propagate.
func (g *GenerationService) currentPlanID(ctx context.Context, householdID string) (string, error) {
	prev, err := g.repo.CurrentWeeklyPlan(ctx, householdID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return prev.ID, nil
}

// SwapRecipe replaces oldRecipeID in plan with a freshly generated recipe.
// Validation mirrors GenerateWeek: dislikes excluded (one semantic retry, then
// ErrDislikeViolation), new.Servings ≥ remaining week target (ErrPortionsShort),
// combined protein variety across the new + kept recipes ≥ 2 (ErrProteinVariety).
// On success the repository call rotates the plan's recipe_ids and invalidates
// the shopping list in one transaction; plan.RecipeIDs is updated in place.
func (g *GenerationService) SwapRecipe(ctx context.Context, h *domain.HouseholdProfile, plan *domain.WeeklyPlan, oldRecipeID string) (*SwappedRecipe, error) {
	ctx, cancel := context.WithTimeout(ctx, generationTimeout)
	defer cancel()

	idx := indexOfString(plan.RecipeIDs, oldRecipeID)
	if idx < 0 {
		return nil, fmt.Errorf("swap recipe: %w", ErrGenerationInvalid)
	}

	keptIDs := make([]string, 0, len(plan.RecipeIDs)-1)
	for i, id := range plan.RecipeIDs {
		if i == idx {
			continue
		}
		keptIDs = append(keptIDs, id)
	}
	kept, err := g.repo.RecipesByIDs(ctx, keptIDs)
	if err != nil {
		return nil, fmt.Errorf("swap recipe: %w", err)
	}

	targetServings := targetPortions(h.FamilySize)
	for _, r := range kept {
		targetServings -= r.Servings
	}
	if targetServings < 1 {
		targetServings = 1
	}

	system, trigger, err := g.loadSwapPrompt(h, kept, targetServings)
	if err != nil {
		return nil, fmt.Errorf("swap recipe: %w", err)
	}

	reply, err := g.completeSwap(ctx, system, trigger)
	if err != nil {
		return nil, fmt.Errorf("swap recipe: %w", err)
	}

	if bad := dislikeViolationsRecipe(reply.Recipe, h.DislikedIngredients); len(bad) > 0 {
		retryTrigger := trigger + swapDislikeHint(bad)
		reply, err = g.completeSwap(ctx, system, retryTrigger)
		if err != nil {
			return nil, fmt.Errorf("swap recipe (dislike retry): %w", err)
		}
		if bad := dislikeViolationsRecipe(reply.Recipe, h.DislikedIngredients); len(bad) > 0 {
			return nil, fmt.Errorf("swap recipe: %w", ErrDislikeViolation)
		}
	}

	if reply.Recipe.Servings < targetServings {
		return nil, fmt.Errorf("swap recipe: %w", ErrPortionsShort)
	}
	if !swapHasProteinVariety(reply.Recipe, kept) {
		return nil, fmt.Errorf("swap recipe: %w", ErrProteinVariety)
	}

	newRecipe, protein := mapRecipe(reply.Recipe, h)

	// Rebuild the shopping list from the full new set (kept + replacement) so it
	// stays consistent after the swap rather than being left empty.
	allRecipes := append(append(make([]domain.Recipe, 0, len(kept)+1), kept...), newRecipe)
	list, err := g.builder.Build(ctx, allRecipes, h.PantryBasics)
	if err != nil {
		return nil, fmt.Errorf("swap recipe: %w", err)
	}

	if err := g.repo.SwapRecipeInPlan(ctx, plan.ID, oldRecipeID, &newRecipe, list); err != nil {
		return nil, fmt.Errorf("swap recipe: %w", err)
	}
	plan.RecipeIDs[idx] = newRecipe.ID
	plan.ShoppingList = list

	return &SwappedRecipe{Plan: plan, Recipe: newRecipe, Protein: protein}, nil
}

// completeSwap sends one swap request and decodes the single-recipe reply.
func (g *GenerationService) completeSwap(ctx context.Context, system, trigger string) (generatedSwap, error) {
	return llm.Generate[generatedSwap](ctx, g.client, llm.Request{
		Role:      llm.RoleGenerate,
		System:    system,
		Prompt:    trigger,
		Schema:    swapSchemaHint,
		MaxTokens: maxSwapTokens,
	})
}

// loadSwapPrompt assembles the cached system block and renders the swap trigger
// from the household, the two kept recipes, and the remaining-servings target.
// Few-shot examples are NOT appended here — the swap contract is tight enough
// without them.
func (g *GenerationService) loadSwapPrompt(h *domain.HouseholdProfile, kept []domain.Recipe, targetServings int) (system, trigger string, err error) {
	base, err := prompts.Load("swap_recipe.v1.txt")
	if err != nil {
		return "", "", err
	}

	systemPart, triggerTmpl, ok := strings.Cut(base, triggerDelimiter)
	if !ok {
		return "", "", fmt.Errorf("prompt missing %q delimiter", triggerDelimiter)
	}

	recent, err := g.repo.RecentRecipes(context.Background(), h.ID, recentLimit)
	if err != nil {
		return "", "", err
	}

	tmpl, err := template.New("swapTrigger").Parse(triggerTmpl)
	if err != nil {
		return "", "", fmt.Errorf("parse swap trigger template: %w", err)
	}
	var sb strings.Builder
	data := swapTriggerData{
		Language:       string(h.Language),
		Adults:         h.FamilySize.Adults,
		Kids:           h.FamilySize.Kids,
		TargetServings: targetServings,
		Disliked:       h.DislikedIngredients,
		Pantry:         h.PantryBasics,
		Kept:           formatKept(kept),
		Recent:         formatRecent(recent),
	}
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", "", fmt.Errorf("render swap trigger template: %w", err)
	}

	return strings.TrimSpace(systemPart), strings.TrimSpace(sb.String()), nil
}

// indexOfString returns the index of v in xs or -1 when absent.
func indexOfString(xs []string, v string) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return -1
}

// complete renders nothing further — it sends one request and decodes the reply.
func (g *GenerationService) complete(ctx context.Context, system, trigger string) (generatedWeek, error) {
	return llm.Generate[generatedWeek](ctx, g.client, llm.Request{
		Role:      llm.RoleGenerate,
		System:    system,
		Prompt:    trigger,
		Schema:    schemaHint,
		MaxTokens: maxGenTokens,
	})
}

// loadPrompt assembles the cached system block (instructions + few-shot examples)
// and renders the variable trigger from the household profile and recent history.
func (g *GenerationService) loadPrompt(h *domain.HouseholdProfile) (system, trigger string, err error) {
	base, err := prompts.Load("generate_week.v1.txt")
	if err != nil {
		return "", "", err
	}
	examples, err := prompts.Load("recipe_examples.v1.txt")
	if err != nil {
		return "", "", err
	}

	systemPart, triggerTmpl, ok := strings.Cut(base, triggerDelimiter)
	if !ok {
		return "", "", fmt.Errorf("prompt missing %q delimiter", triggerDelimiter)
	}

	recent, err := g.repo.RecentRecipes(context.Background(), h.ID, recentLimit)
	if err != nil {
		return "", "", err
	}

	tmpl, err := template.New("trigger").Parse(triggerTmpl)
	if err != nil {
		return "", "", fmt.Errorf("parse trigger template: %w", err)
	}
	var sb strings.Builder
	data := triggerData{
		Language:       string(h.Language),
		Adults:         h.FamilySize.Adults,
		Kids:           h.FamilySize.Kids,
		TargetPortions: targetPortions(h.FamilySize),
		Disliked:       h.DislikedIngredients,
		Pantry:         h.PantryBasics,
		Recent:         formatRecent(recent),
	}
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", "", fmt.Errorf("render trigger template: %w", err)
	}

	system = strings.TrimSpace(systemPart) + "\n\n" + strings.TrimSpace(examples)
	return system, strings.TrimSpace(sb.String()), nil
}

// targetPortions is the number of servings the three recipes must cover together:
// seven days for every person in the household.
func targetPortions(f domain.FamilySize) int { return 7 * (f.Adults + f.Kids) }

// dislikeViolations returns the disliked terms that appear in any ingredient
// name across the week, using inflection-tolerant matching from internal/shopping.
// Empty terms are ignored.
func dislikeViolations(week generatedWeek, disliked []string) []string {
	var hits []string
	for _, term := range disliked {
		if strings.TrimSpace(term) == "" {
			continue
		}
		for _, r := range week.Recipes {
			matched := false
			for _, ing := range r.Ingredients {
				if shopping.ContainsTerm(ing.Name, term) {
					hits = append(hits, term)
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
	}
	return hits
}

// dislikeHint augments the trigger with the offending terms for the retry. When
// final is true (the last retry the budget allows), the wording is escalated.
func dislikeHint(bad []string, final bool) string {
	if final {
		return "\n\nTHIS IS THE FINAL ATTEMPT. The previous replies used these FORBIDDEN ingredients: " +
			strings.Join(bad, ", ") + ". You MUST exclude them entirely — not as a main ingredient, " +
			"not as a garnish, not as a sauce component. If you cannot exclude them while satisfying " +
			"the other constraints, choose three recipes built around entirely different proteins and produce."
	}
	return "\n\nThe previous attempt used these FORBIDDEN ingredients: " +
		strings.Join(bad, ", ") + ". Regenerate all three recipes with these completely excluded."
}

// portionsCovered reports whether the recipes' servings sum to at least the week's
// target portions.
func portionsCovered(week generatedWeek, f domain.FamilySize) bool {
	total := 0
	for _, r := range week.Recipes {
		total += r.Servings
	}
	return total >= targetPortions(f)
}

// hasProteinVariety reports whether at least two distinct, non-empty protein tags
// appear across the week.
func hasProteinVariety(week generatedWeek) bool {
	seen := make(map[string]struct{})
	for _, r := range week.Recipes {
		p := strings.ToLower(strings.TrimSpace(r.Protein))
		if p != "" {
			seen[p] = struct{}{}
		}
	}
	return len(seen) >= 2
}

// formatRecent turns recent recipes into prompt lines with a compact feedback tag,
// e.g. "Creamy Pasta [liked, cook again]".
func formatRecent(recipes []domain.Recipe) []string {
	lines := make([]string, 0, len(recipes))
	for _, r := range recipes {
		line := r.Title
		if tag := feedbackTag(r.Feedback); tag != "" {
			line += " [" + tag + "]"
		}
		lines = append(lines, line)
	}
	return lines
}

func feedbackTag(f *domain.Feedback) string {
	if f == nil {
		return ""
	}
	var parts []string
	if f.Liked {
		parts = append(parts, "liked")
	}
	if f.Disliked {
		parts = append(parts, "disliked")
	}
	if f.CookAgain {
		parts = append(parts, "cook again")
	}
	return strings.Join(parts, ", ")
}

// toDomainRecipes maps the LLM DTO into persistable recipes (tagged llm, in the
// household language) and returns the parallel protein tags for the cards.
func toDomainRecipes(week generatedWeek, h *domain.HouseholdProfile) ([]domain.Recipe, []string) {
	recipes := make([]domain.Recipe, len(week.Recipes))
	proteins := make([]string, len(week.Recipes))
	for i, gr := range week.Recipes {
		recipes[i], proteins[i] = mapRecipe(gr, h)
	}
	return recipes, proteins
}

// mapRecipe converts one LLM-DTO recipe into a persistable domain Recipe plus
// its normalized protein tag (transient, used only for the card emoji).
func mapRecipe(gr generatedRecipe, h *domain.HouseholdProfile) (domain.Recipe, string) {
	ings := make([]domain.Ingredient, len(gr.Ingredients))
	for j, gi := range gr.Ingredients {
		ings[j] = domain.Ingredient{
			Name:     gi.Name,
			Amount:   gi.Amount,
			Unit:     gi.Unit,
			Category: normalizeCategory(gi.Category),
		}
	}
	return domain.Recipe{
		HouseholdID:     h.ID,
		Language:        h.Language,
		Title:           gr.Title,
		Description:     gr.Description,
		CookTimeMinutes: gr.CookTimeMinutes,
		Servings:        gr.Servings,
		Ingredients:     ings,
		Steps:           gr.Steps,
		Source:          domain.SourceLLM,
	}, strings.ToLower(strings.TrimSpace(gr.Protein))
}

// dislikeViolationsRecipe returns the disliked terms that appear in any
// ingredient name of one recipe (case-insensitive substring match).
func dislikeViolationsRecipe(r generatedRecipe, disliked []string) []string {
	var hits []string
	for _, term := range disliked {
		t := strings.ToLower(strings.TrimSpace(term))
		if t == "" {
			continue
		}
		for _, ing := range r.Ingredients {
			if strings.Contains(strings.ToLower(ing.Name), t) {
				hits = append(hits, term)
				break
			}
		}
	}
	return hits
}

// swapDislikeHint augments the swap trigger with the offending terms for retry.
func swapDislikeHint(bad []string) string {
	return "\n\nThe previous attempt used these FORBIDDEN ingredients: " +
		strings.Join(bad, ", ") + ". Generate a new replacement recipe with these completely excluded."
}

// swapHasProteinVariety reports whether the new recipe plus the kept recipes
// span ≥ 2 distinct non-empty protein categories. The new recipe contributes
// its own reported protein; kept recipes contribute an inferred bucket because
// CH-8 did not store protein on Recipe.
func swapHasProteinVariety(newRecipe generatedRecipe, kept []domain.Recipe) bool {
	seen := make(map[string]struct{})
	if p := strings.ToLower(strings.TrimSpace(newRecipe.Protein)); p != "" {
		seen[p] = struct{}{}
	}
	for _, r := range kept {
		if p := inferProtein(r); p != "" {
			seen[p] = struct{}{}
		}
	}
	return len(seen) >= 2
}

// inferProtein guesses a kept recipe's protein bucket from its ingredient
// names. Bilingual (EN/FI/RU) keyword match — good enough for swap variety
// validation when no per-recipe protein column exists yet (revisit when one
// does). Returns "" when no known keyword is found.
func inferProtein(r domain.Recipe) string {
	for _, ing := range r.Ingredients {
		name := strings.ToLower(ing.Name)
		switch {
		case containsAny(name, "chicken", "kana", "курица", "куриц"):
			return "poultry"
		case containsAny(name, "turkey", "kalkkuna", "индейк"):
			return "poultry"
		case containsAny(name, "beef", "nauda", "naudan", "говядин"):
			return "red_meat"
		case containsAny(name, "lamb", "lammas", "lampaan", "ягнят", "баранин"):
			return "red_meat"
		case containsAny(name, "pork", "sika", "porsaan", "сви"):
			return "pork"
		case containsAny(name, "salmon", "lohi", "лосось", "сёмг", "семг"):
			return "fish"
		case containsAny(name, "tuna", "tonnikala", "тунец"):
			return "fish"
		case containsAny(name, "cod", "turska", "треск"):
			return "fish"
		case containsAny(name, "shrimp", "katkarapu", "креветк"):
			return "seafood"
		case containsAny(name, "tofu", "tofua", "тофу"):
			return "vegetarian"
		}
	}
	return ""
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// formatKept renders the two kept recipes as compact one-line summaries for the
// swap prompt: "Title | protein | top-3 ingredient names | description".
func formatKept(kept []domain.Recipe) []string {
	out := make([]string, 0, len(kept))
	for _, r := range kept {
		ingNames := make([]string, 0, 3)
		for i, ing := range r.Ingredients {
			if i >= 3 {
				break
			}
			ingNames = append(ingNames, ing.Name)
		}
		protein := inferProtein(r)
		if protein == "" {
			protein = "unknown"
		}
		desc := r.Description
		if desc == "" {
			desc = "no description"
		}
		out = append(out, fmt.Sprintf("%s | %s | %s | %s",
			r.Title, protein, strings.Join(ingNames, ", "), desc))
	}
	return out
}

// normalizeCategory coerces an LLM-supplied category to a known store category,
// defaulting unrecognized values to "other" (LLM output is untrusted input).
func normalizeCategory(s string) domain.IngredientCategory {
	switch c := domain.IngredientCategory(strings.ToLower(strings.TrimSpace(s))); c {
	case domain.CategoryProduce, domain.CategoryMeatFish, domain.CategoryDairy,
		domain.CategoryPantry, domain.CategoryFrozen, domain.CategoryOther:
		return c
	default:
		return domain.CategoryOther
	}
}

// mondayOf returns the Monday (UTC, midnight) of the week containing t.
func mondayOf(t time.Time) time.Time {
	t = t.UTC()
	offset := (int(t.Weekday()) + 6) % 7 // Monday=0 ... Sunday=6
	d := t.AddDate(0, 0, -offset)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}
