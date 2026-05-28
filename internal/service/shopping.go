package service

import (
	"context"
	"log/slog"
	"strings"

	"github.com/AntonKilk/cooking-helper/internal/domain"
	"github.com/AntonKilk/cooking-helper/internal/llm"
	"github.com/AntonKilk/cooking-helper/internal/llm/prompts"
	"github.com/AntonKilk/cooking-helper/internal/shopping"
)

// categorizeMaxTokens bounds the categorize reply — a single category word in a
// tiny JSON object — generously enough to survive one JSON-repair retry.
const categorizeMaxTokens = 64

// categorizeSchemaHint mirrors categorize_ingredient.v1.txt's contract; it is
// echoed back into the repair prompt when a reply is not valid JSON.
const categorizeSchemaHint = `{"category":string}`

// categorizePlaceholder is the slot the categorize prompt template exposes for
// the ingredient name. It is filled by a literal string replace (not text/template)
// so the prompt file stays human-readable with its `{{ingredient}}` marker.
const categorizePlaceholder = "{{ingredient}}"

// CategoryCache caches an ingredient's store category by its normalized name so
// the LLM categorizer is never asked twice for the same ingredient. The cache is
// global (name-keyed): a carrot is produce for every household.
type CategoryCache interface {
	CategoriesByNames(ctx context.Context, names []string) (map[string]domain.IngredientCategory, error)
	SaveCategory(ctx context.Context, nameNormalized string, c domain.IngredientCategory) error
}

// ShoppingBuilder turns a set of recipes into a consolidated, store-categorized
// shopping list. Consolidation and the first-pass dictionary categorization are
// pure (internal/shopping); ingredients the dictionary cannot place are resolved
// against the DB cache and, on a miss, a cheap LLM call (RoleCategorize), with the
// resolved category written back to the cache. Categorization never fails the
// build: any cache or LLM error degrades the affected line to "other".
type ShoppingBuilder struct {
	client llm.Client
	cache  CategoryCache
}

// NewShoppingBuilder returns a builder backed by the given LLM client and cache.
func NewShoppingBuilder(client llm.Client, cache CategoryCache) *ShoppingBuilder {
	return &ShoppingBuilder{client: client, cache: cache}
}

// categoryReply decodes the categorize_ingredient prompt's JSON reply.
type categoryReply struct {
	Category string `json:"category"`
}

// Build consolidates the recipes' ingredients, drops pantry basics, and assigns
// every resulting line a store category.
func (b *ShoppingBuilder) Build(ctx context.Context, recipes []domain.Recipe, pantryBasics []string) ([]domain.ShoppingListItem, error) {
	items := shopping.Consolidate(recipes, pantryBasics)

	// Gather the names the dictionary could not place, de-duplicated by their
	// normalized form (the cache key).
	unresolved := make(map[string]string) // normalized -> display name
	for i := range items {
		if items[i].Category == "" {
			unresolved[shopping.Normalize(items[i].Name)] = items[i].Name
		}
	}
	if len(unresolved) == 0 {
		return items, nil
	}

	resolved := b.resolveCategories(ctx, unresolved)

	for i := range items {
		if items[i].Category != "" {
			continue
		}
		if c, ok := resolved[shopping.Normalize(items[i].Name)]; ok {
			items[i].Category = c
		} else {
			items[i].Category = domain.CategoryOther
		}
	}
	return items, nil
}

// resolveCategories fills a category for every unresolved name: the DB cache
// first, then one LLM call per remaining name with the result cached. Any error
// degrades the affected name to CategoryOther (recorded in the returned map)
// rather than failing the whole build.
func (b *ShoppingBuilder) resolveCategories(ctx context.Context, unresolved map[string]string) map[string]domain.IngredientCategory {
	out := make(map[string]domain.IngredientCategory, len(unresolved))

	norms := make([]string, 0, len(unresolved))
	for norm := range unresolved {
		norms = append(norms, norm)
	}

	cached, err := b.cache.CategoriesByNames(ctx, norms)
	if err != nil {
		slog.Warn("shopping category cache read failed", "err", err, "count", len(norms))
		cached = map[string]domain.IngredientCategory{}
	}

	rawPrompt, err := prompts.Load("categorize_ingredient.v1.txt")
	if err != nil {
		// Without the prompt we cannot ask the LLM; fall back to cache-only and
		// default the rest to "other".
		slog.Error("load categorize prompt", "err", err)
		rawPrompt = ""
	}

	for norm, display := range unresolved {
		if c, ok := cached[norm]; ok {
			out[norm] = c
			continue
		}
		if rawPrompt == "" {
			out[norm] = domain.CategoryOther
			continue
		}
		c, ok := b.categorizeOne(ctx, rawPrompt, display)
		if !ok {
			out[norm] = domain.CategoryOther
			continue
		}
		out[norm] = c
		if err := b.cache.SaveCategory(ctx, norm, c); err != nil {
			slog.Warn("shopping category cache write failed", "err", err)
		}
	}
	return out
}

// categorizeOne asks the LLM for a single ingredient's store category. ok is
// false only on a transport/parse failure (caller defaults to "other" without
// caching, so a later week can retry). A parseable-but-unknown category is
// coerced to "other" and returned with ok=true (the model answered).
func (b *ShoppingBuilder) categorizeOne(ctx context.Context, rawPrompt, ingredient string) (domain.IngredientCategory, bool) {
	prompt := strings.ReplaceAll(rawPrompt, categorizePlaceholder, ingredient)
	reply, err := llm.Generate[categoryReply](ctx, b.client, llm.Request{
		Role:      llm.RoleCategorize,
		Prompt:    prompt,
		Schema:    categorizeSchemaHint,
		MaxTokens: categorizeMaxTokens,
	})
	if err != nil {
		slog.Warn("categorize ingredient failed", "err", err)
		return "", false
	}
	return normalizeCategory(reply.Category), true
}
