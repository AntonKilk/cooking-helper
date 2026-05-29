package service_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/llm/openai"
	"github.com/AntonKilk/cooking-helper/internal/service"
)

// memCache is a throwaway in-memory CategoryCache for the live builder test.
type memCache struct {
	m map[string]domain.IngredientCategory
}

func (c *memCache) CategoriesByNames(_ context.Context, names []string) (map[string]domain.IngredientCategory, error) {
	out := make(map[string]domain.IngredientCategory)
	for _, n := range names {
		if v, ok := c.m[n]; ok {
			out[n] = v
		}
	}
	return out, nil
}

func (c *memCache) SaveCategory(_ context.Context, name string, cat domain.IngredientCategory) error {
	c.m[name] = cat
	return nil
}

// TestShoppingBuilderLive exercises the full builder path against the real OpenAI
// API: consolidate -> dictionary -> LLM categorize (RoleCategorize) for the
// dictionary misses. It is skipped unless OPENAI_API_KEY is set.
//
//	OPENAI_API_KEY=sk-... go test ./internal/service/ -run TestShoppingBuilderLive -v
func TestShoppingBuilderLive(t *testing.T) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Skip("OPENAI_API_KEY not set; skipping live shopping-builder call")
	}

	client := openai.New(key, openai.WithTimeout(20*time.Second))
	builder := service.NewShoppingBuilder(client, &memCache{m: map[string]domain.IngredientCategory{}})

	recipes := []domain.Recipe{
		{Ingredients: []domain.Ingredient{
			{Name: "chicken breast", Amount: 500, Unit: "g"}, // dictionary -> meat_fish
			{Name: "carrot", Amount: 250, Unit: "g"},         // dictionary -> produce
			{Name: "carrot", Amount: 100, Unit: "g"},         // merges to 350 g
			{Name: "halloumi", Amount: 200, Unit: "g"},       // dictionary miss -> LLM
			{Name: "rkl soy sauce", Amount: 2, Unit: "rkl"},  // miss -> LLM
		}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	items, err := builder.Build(ctx, recipes, []string{"salt"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	for _, it := range items {
		if it.Category == "" {
			t.Errorf("ingredient %q left uncategorized", it.Name)
		}
		t.Logf("%-16s %6.1f %-4s -> %s", it.Name, it.Amount, it.Unit, it.Category)
	}

	// Spot-check the consolidation invariant survives the live path.
	for _, it := range items {
		if it.Name == "carrot" && (it.Amount != 350 || it.Unit != "g") {
			t.Errorf("carrot consolidation = %v %s, want 350 g", it.Amount, it.Unit)
		}
	}
}
