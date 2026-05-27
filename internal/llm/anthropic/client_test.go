package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/AntonKilk/cooking-helper/internal/llm"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		transient bool
		timeout   bool
	}{
		{"server error", &sdk.Error{StatusCode: 500}, true, false},
		{"rate limited", &sdk.Error{StatusCode: 429}, true, false},
		{"bad request", &sdk.Error{StatusCode: 400}, false, false},
		{"unauthorized", &sdk.Error{StatusCode: 401}, false, false},
		{"deadline", context.DeadlineExceeded, false, true},
		{"network", errors.New("connection reset"), true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classify(c.err)
			if errors.Is(got, llm.ErrTransient) != c.transient {
				t.Fatalf("transient = %v, want %v (err=%v)", !c.transient, c.transient, got)
			}
			if errors.Is(got, llm.ErrTimeout) != c.timeout {
				t.Fatalf("timeout = %v, want %v (err=%v)", !c.timeout, c.timeout, got)
			}
		})
	}
}

func TestClassifyNil(t *testing.T) {
	if err := classify(nil); err != nil {
		t.Fatalf("classify(nil) = %v, want nil", err)
	}
}

func TestBuildParamsSetsCacheControlAndDefaults(t *testing.T) {
	model, err := modelFor(llm.RoleCategorize)
	if err != nil {
		t.Fatalf("modelFor: %v", err)
	}
	params := buildParams(model, llm.Request{
		Role:   llm.RoleCategorize,
		System: "stable household block",
		Prompt: "categorize: milk",
	})

	if params.MaxTokens != defaultMaxTokens {
		t.Fatalf("max tokens = %d, want default %d", params.MaxTokens, defaultMaxTokens)
	}
	if string(params.Model) != model {
		t.Fatalf("model = %q, want %q", params.Model, model)
	}
	if len(params.System) != 1 {
		t.Fatalf("system blocks = %d, want 1", len(params.System))
	}
	if params.System[0].Text != "stable household block" {
		t.Fatalf("system text = %q", params.System[0].Text)
	}

	// The system block must serialize with an ephemeral cache breakpoint so the
	// stable prompt prefix is cached across calls.
	raw, err := json.Marshal(params.System[0])
	if err != nil {
		t.Fatalf("marshal system block: %v", err)
	}
	if !strings.Contains(string(raw), `"cache_control"`) || !strings.Contains(string(raw), `"ephemeral"`) {
		t.Fatalf("system block missing ephemeral cache_control: %s", raw)
	}
}

func TestBuildParamsNoSystem(t *testing.T) {
	model, err := modelFor(llm.RoleGenerate)
	if err != nil {
		t.Fatalf("modelFor: %v", err)
	}
	params := buildParams(model, llm.Request{Role: llm.RoleGenerate, Prompt: "hi", MaxTokens: 50})
	if len(params.System) != 0 {
		t.Fatalf("system blocks = %d, want 0", len(params.System))
	}
	if params.MaxTokens != 50 {
		t.Fatalf("max tokens = %d, want 50", params.MaxTokens)
	}
}
