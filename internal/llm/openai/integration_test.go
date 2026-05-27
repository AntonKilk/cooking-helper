package openai_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/AntonKilk/cooking-helper/internal/llm"
	"github.com/AntonKilk/cooking-helper/internal/llm/openai"
	"github.com/AntonKilk/cooking-helper/internal/llm/prompts"
)

// TestCategorizeLive exercises the full path against the real OpenAI API:
// SDK call -> retry wrapper -> JSON decode via llm.Generate. It is skipped
// unless OPENAI_API_KEY is set, so it never runs in CI without credentials.
//
//	OPENAI_API_KEY=sk-... go test ./internal/llm/openai/ -run TestCategorizeLive -v
func TestCategorizeLive(t *testing.T) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Skip("OPENAI_API_KEY not set; skipping live OpenAI call")
	}

	prompt, err := prompts.Load("categorize_ingredient.v1.txt")
	if err != nil {
		t.Fatalf("load prompt: %v", err)
	}

	client := openai.New(key, openai.WithTimeout(20*time.Second))

	type categorized struct {
		Category string `json:"category"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	got, err := llm.Generate[categorized](ctx, client, llm.Request{
		Role:      llm.RoleCategorize,
		Prompt:    prompt + "\nIngredient: maito (milk)",
		Schema:    `{"category":"produce|meat_fish|dairy|pantry|frozen|other"}`,
		MaxTokens: 50,
		RequestID: "live-test",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if got.Category == "" {
		t.Fatalf("empty category in %+v", got)
	}
	t.Logf("categorized milk as %q", got.Category)
}
