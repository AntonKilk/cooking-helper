package service

import (
	"context"
	"errors"
	"testing"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/llm"
	"github.com/AntonKilk/cooking-helper/internal/shopping"
)

// fakeCache is an in-memory CategoryCache that records reads/writes and can be
// primed with pre-cached categories or made to fail its read.
type fakeCache struct {
	stored  map[string]domain.IngredientCategory
	saved   map[string]domain.IngredientCategory
	readErr error
	reads   int
}

func newFakeCache() *fakeCache {
	return &fakeCache{
		stored: map[string]domain.IngredientCategory{},
		saved:  map[string]domain.IngredientCategory{},
	}
}

func (c *fakeCache) CategoriesByNames(_ context.Context, names []string) (map[string]domain.IngredientCategory, error) {
	c.reads++
	if c.readErr != nil {
		return nil, c.readErr
	}
	out := make(map[string]domain.IngredientCategory)
	for _, n := range names {
		if v, ok := c.stored[n]; ok {
			out[n] = v
		}
	}
	return out, nil
}

func (c *fakeCache) SaveCategory(_ context.Context, name string, cat domain.IngredientCategory) error {
	c.saved[name] = cat
	c.stored[name] = cat
	return nil
}

// funcLLM answers each Complete from a supplied function, so a categorize test can
// vary the reply (or error) per ingredient by inspecting the prompt.
type funcLLM struct {
	fn    func(req llm.Request) (llm.Completion, error)
	calls int
}

func (f *funcLLM) Complete(_ context.Context, req llm.Request) (llm.Completion, error) {
	f.calls++
	return f.fn(req)
}

func recipeWith(ings ...domain.Ingredient) domain.Recipe {
	return domain.Recipe{Ingredients: ings}
}

func TestShoppingBuilderDictionaryOnlySkipsLLM(t *testing.T) {
	cache := newFakeCache()
	llmStub := &funcLLM{fn: func(llm.Request) (llm.Completion, error) {
		t.Fatal("LLM must not be called when the dictionary covers every ingredient")
		return llm.Completion{}, nil
	}}
	b := NewShoppingBuilder(llmStub, cache)

	items, err := b.Build(context.Background(), []domain.Recipe{
		recipeWith(
			domain.Ingredient{Name: "chicken breast", Amount: 500, Unit: "g"},
			domain.Ingredient{Name: "carrot", Amount: 2, Unit: "шт"},
		),
	}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if llmStub.calls != 0 || cache.reads != 0 {
		t.Fatalf("expected no LLM/cache traffic, got llm=%d cache=%d", llmStub.calls, cache.reads)
	}
	for _, it := range items {
		if it.Category == "" {
			t.Fatalf("item %q left uncategorized", it.Name)
		}
	}
}

func TestShoppingBuilderUsesCacheBeforeLLM(t *testing.T) {
	cache := newFakeCache()
	cache.stored[shopping.Normalize("quinoa")] = domain.CategoryPantry
	llmStub := &funcLLM{fn: func(llm.Request) (llm.Completion, error) {
		t.Fatal("LLM must not be called for a cache hit")
		return llm.Completion{}, nil
	}}
	b := NewShoppingBuilder(llmStub, cache)

	items, err := b.Build(context.Background(), []domain.Recipe{
		recipeWith(domain.Ingredient{Name: "quinoa", Amount: 200, Unit: "g"}),
	}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cache.reads != 1 {
		t.Fatalf("cache reads = %d, want 1", cache.reads)
	}
	got := items[0]
	if got.Name != "quinoa" || got.Category != domain.CategoryPantry {
		t.Fatalf("quinoa = %+v, want pantry from cache", got)
	}
}

func TestShoppingBuilderLLMFallbackCaches(t *testing.T) {
	cache := newFakeCache()
	llmStub := &funcLLM{fn: func(llm.Request) (llm.Completion, error) {
		return llm.Completion{Text: `{"category":"pantry"}`}, nil
	}}
	b := NewShoppingBuilder(llmStub, cache)

	items, err := b.Build(context.Background(), []domain.Recipe{
		recipeWith(domain.Ingredient{Name: "quinoa", Amount: 200, Unit: "g"}),
	}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if llmStub.calls != 1 {
		t.Fatalf("llm calls = %d, want 1", llmStub.calls)
	}
	if items[0].Category != domain.CategoryPantry {
		t.Fatalf("quinoa category = %q, want pantry from LLM", items[0].Category)
	}
	if cache.saved[shopping.Normalize("quinoa")] != domain.CategoryPantry {
		t.Fatalf("LLM result not cached: %+v", cache.saved)
	}
}

func TestShoppingBuilderLLMErrorDefaultsToOther(t *testing.T) {
	cache := newFakeCache()
	llmStub := &funcLLM{fn: func(llm.Request) (llm.Completion, error) {
		return llm.Completion{}, errors.New("provider down")
	}}
	b := NewShoppingBuilder(llmStub, cache)

	items, err := b.Build(context.Background(), []domain.Recipe{
		recipeWith(domain.Ingredient{Name: "quinoa", Amount: 200, Unit: "g"}),
	}, nil)
	if err != nil {
		t.Fatalf("build must not fail on categorize error: %v", err)
	}
	if items[0].Category != domain.CategoryOther {
		t.Fatalf("quinoa category = %q, want other (LLM failed)", items[0].Category)
	}
	// A transient failure must NOT be cached, so a later week can retry.
	if len(cache.saved) != 0 {
		t.Fatalf("failed categorization should not be cached: %+v", cache.saved)
	}
}

func TestShoppingBuilderCacheReadErrorDegrades(t *testing.T) {
	cache := newFakeCache()
	cache.readErr = errors.New("db unavailable")
	llmStub := &funcLLM{fn: func(llm.Request) (llm.Completion, error) {
		return llm.Completion{Text: `{"category":"pantry"}`}, nil
	}}
	b := NewShoppingBuilder(llmStub, cache)

	items, err := b.Build(context.Background(), []domain.Recipe{
		recipeWith(domain.Ingredient{Name: "quinoa", Amount: 200, Unit: "g"}),
	}, nil)
	if err != nil {
		t.Fatalf("build must survive a cache read error: %v", err)
	}
	if items[0].Category != domain.CategoryPantry {
		t.Fatalf("quinoa category = %q, want pantry via LLM after cache miss", items[0].Category)
	}
}

// TestShoppingBuilderCategorizationAccuracy is the CH-12 AC: ≥95% of ingredients
// across five representative weeks land on the correct store category. The
// dictionary places the bulk; an LLM stub (mirroring a correct model) handles the
// dictionary misses, exercising the full consolidate→cache→LLM pipeline.
func TestShoppingBuilderCategorizationAccuracy(t *testing.T) {
	// Five representative weeks (RU / FI / EN). The dictionary places most names;
	// the trailing pair are deliberate dictionary misses that exercise the LLM
	// path. Expected category by ingredient name.
	corpus := []struct {
		name string
		cat  domain.IngredientCategory
	}{
		// Week 1 — EN roast dinner
		{"chicken breast", domain.CategoryMeatFish},
		{"potatoes", domain.CategoryProduce},
		{"carrots", domain.CategoryProduce},
		{"butter", domain.CategoryDairy},
		{"plain flour", domain.CategoryPantry},
		{"frozen peas", domain.CategoryFrozen},
		// Week 2 — FI weeknight
		{"jauheliha", domain.CategoryMeatFish},
		{"sipuli", domain.CategoryProduce},
		{"tomaatti", domain.CategoryProduce},
		{"juusto", domain.CategoryDairy},
		{"riisi", domain.CategoryPantry},
		{"oliiviöljy", domain.CategoryPantry},
		// Week 3 — RU comfort food
		{"куриное филе", domain.CategoryMeatFish},
		{"морковь", domain.CategoryProduce},
		{"картофель", domain.CategoryProduce},
		{"молоко", domain.CategoryDairy},
		{"мука пшеничная", domain.CategoryPantry},
		{"соль", domain.CategoryPantry},
		// Week 4 — fish + dairy mix
		{"salmon fillet", domain.CategoryMeatFish},
		{"lohi", domain.CategoryMeatFish},
		{"лосось", domain.CategoryMeatFish},
		{"kerma", domain.CategoryDairy},
		{"сметана", domain.CategoryDairy},
		{"spinach", domain.CategoryProduce},
		// Week 5 — pantry-heavy bake
		{"sugar", domain.CategoryPantry},
		{"sokeri", domain.CategoryPantry},
		{"baking powder", domain.CategoryPantry},
		{"eggs", domain.CategoryDairy},
		{"munat", domain.CategoryDairy},
		{"dark chocolate", domain.CategoryPantry},
		// Dictionary misses the model is expected to place correctly.
		{"quinoa", domain.CategoryPantry},
		{"red lentils", domain.CategoryPantry},
	}

	want := map[string]domain.IngredientCategory{}
	var recipes []domain.Recipe
	for _, item := range corpus {
		want[shopping.Normalize(item.name)] = item.cat
		recipes = append(recipes, recipeWith(domain.Ingredient{Name: item.name, Amount: 1, Unit: "g"}))
	}

	llmStub := &funcLLM{fn: func(req llm.Request) (llm.Completion, error) {
		// The stub answers with the expected category for whichever ingredient the
		// prompt is asking about.
		for norm, cat := range want {
			if shopping.ContainsTerm(req.Prompt, norm) {
				return llm.Completion{Text: `{"category":"` + string(cat) + `"}`}, nil
			}
		}
		return llm.Completion{Text: `{"category":"other"}`}, nil
	}}

	b := NewShoppingBuilder(llmStub, newFakeCache())
	items, err := b.Build(context.Background(), recipes, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	correct := 0
	for _, it := range items {
		if it.Category == want[shopping.Normalize(it.Name)] {
			correct++
		} else {
			t.Logf("misplaced %q: got %q, want %q", it.Name, it.Category, want[shopping.Normalize(it.Name)])
		}
	}
	total := len(items)
	if correct*100 < total*95 {
		t.Fatalf("categorization accuracy = %d/%d (<95%%)", correct, total)
	}
	t.Logf("categorization accuracy: %d/%d", correct, total)
}
