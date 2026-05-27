package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/llm"
	"github.com/AntonKilk/cooking-helper/internal/llm/prompts"
)

// Generation tuning. recentLimit bounds how much history feeds the prompt;
// generationTimeout caps the whole tap-to-result path (the provider client also
// applies its own per-call timeout); maxGenTokens must fit three full recipes.
const (
	recentLimit       = 10
	generationTimeout = 45 * time.Second
	maxGenTokens      = 4096
	triggerDelimiter  = "---TRIGGER---"
)

// schemaHint is echoed into the repair prompt when the model's reply is not valid
// JSON. It mirrors the contract in generate_week.v1.txt.
const schemaHint = `{"recipes":[{"title":string,"description":string,"cook_time_minutes":int,` +
	`"servings":int,"protein":string,"ingredients":[{"name":string,"amount":number,"unit":string,"category":string}],"steps":[string]}]}`

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
// satisfies it.
type generationRepo interface {
	RecentRecipes(ctx context.Context, householdID string, limit int) ([]domain.Recipe, error)
	CreateWeekWithRecipes(ctx context.Context, p *domain.WeeklyPlan, recipes []domain.Recipe) error
}

// GeneratedWeek is what GenerateWeek returns to the handler: the persisted plan,
// the persisted recipes, and a parallel slice of normalized protein tags used to
// pick each card's emoji (the tag is transient and not stored on the recipe).
type GeneratedWeek struct {
	Plan     *domain.WeeklyPlan
	Recipes  []domain.Recipe
	Proteins []string
}

// GenerationService turns a household profile into a weekly plan via the LLM,
// enforcing the hard constraints and persisting the result atomically.
type GenerationService struct {
	client llm.Client
	repo   generationRepo
}

// NewGenerationService returns a service backed by the given LLM client and repo.
func NewGenerationService(client llm.Client, repo generationRepo) *GenerationService {
	return &GenerationService{client: client, repo: repo}
}

// triggerData fills the variable half of the prompt.
type triggerData struct {
	Language       string
	Adults         int
	Kids           int
	TargetPortions int
	Disliked       []string
	Pantry         []string
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

	// Dislikes are a hard constraint: on violation, retry once with the offending
	// terms named, then fail closed.
	if bad := dislikeViolations(week, h.DislikedIngredients); len(bad) > 0 {
		retryTrigger := trigger + dislikeHint(bad)
		week, err = g.complete(ctx, system, retryTrigger)
		if err != nil {
			return nil, fmt.Errorf("generate week (dislike retry): %w", err)
		}
		if len(week.Recipes) != 3 {
			return nil, fmt.Errorf("generate week: %w", ErrGenerationInvalid)
		}
		if bad := dislikeViolations(week, h.DislikedIngredients); len(bad) > 0 {
			return nil, fmt.Errorf("generate week: %w", ErrDislikeViolation)
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
	if err := g.repo.CreateWeekWithRecipes(ctx, plan, recipes); err != nil {
		return nil, fmt.Errorf("generate week: %w", err)
	}

	return &GeneratedWeek{Plan: plan, Recipes: recipes, Proteins: proteins}, nil
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

// dislikeViolations returns the disliked terms that appear in any ingredient name
// across the week (case-insensitive substring match). Empty terms are ignored.
func dislikeViolations(week generatedWeek, disliked []string) []string {
	var hits []string
	for _, term := range disliked {
		t := strings.ToLower(strings.TrimSpace(term))
		if t == "" {
			continue
		}
		for _, r := range week.Recipes {
			for _, ing := range r.Ingredients {
				if strings.Contains(strings.ToLower(ing.Name), t) {
					hits = append(hits, term)
					goto next
				}
			}
		}
	next:
	}
	return hits
}

// dislikeHint augments the trigger with the offending terms for the retry.
func dislikeHint(bad []string) string {
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
		ings := make([]domain.Ingredient, len(gr.Ingredients))
		for j, gi := range gr.Ingredients {
			ings[j] = domain.Ingredient{
				Name:     gi.Name,
				Amount:   gi.Amount,
				Unit:     gi.Unit,
				Category: normalizeCategory(gi.Category),
			}
		}
		recipes[i] = domain.Recipe{
			HouseholdID:     h.ID,
			Language:        h.Language,
			Title:           gr.Title,
			Description:     gr.Description,
			CookTimeMinutes: gr.CookTimeMinutes,
			Servings:        gr.Servings,
			Ingredients:     ings,
			Steps:           gr.Steps,
			Source:          domain.SourceLLM,
		}
		proteins[i] = strings.ToLower(strings.TrimSpace(gr.Protein))
	}
	return recipes, proteins
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
