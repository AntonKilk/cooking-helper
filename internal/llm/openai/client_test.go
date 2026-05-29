package openai

import (
	"context"
	"errors"
	"testing"

	sdk "github.com/openai/openai-go"

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

func TestBuildParamsSetsModelMessagesAndDefaults(t *testing.T) {
	model, err := modelFor(llm.RoleCategorize)
	if err != nil {
		t.Fatalf("modelFor: %v", err)
	}
	params := buildParams(model, llm.Request{
		Role:   llm.RoleCategorize,
		System: "stable household block",
		Prompt: "categorize: milk",
	})

	if !params.MaxCompletionTokens.Valid() || params.MaxCompletionTokens.Value != defaultMaxTokens {
		t.Fatalf("max tokens = %+v, want default %d", params.MaxCompletionTokens, defaultMaxTokens)
	}
	if params.Model != model {
		t.Fatalf("model = %q, want %q", params.Model, model)
	}
	// System block first, then user prompt.
	if len(params.Messages) != 2 {
		t.Fatalf("messages = %d, want 2 (system + user)", len(params.Messages))
	}
	if params.Messages[0].OfSystem == nil {
		t.Fatalf("first message is not a system message")
	}
	if params.Messages[1].OfUser == nil {
		t.Fatalf("second message is not a user message")
	}
}

func TestBuildParamsNoSystem(t *testing.T) {
	model, err := modelFor(llm.RoleGenerate)
	if err != nil {
		t.Fatalf("modelFor: %v", err)
	}
	params := buildParams(model, llm.Request{Role: llm.RoleGenerate, Prompt: "hi", MaxTokens: 50})
	if len(params.Messages) != 1 {
		t.Fatalf("messages = %d, want 1 (user only)", len(params.Messages))
	}
	if params.Messages[0].OfUser == nil {
		t.Fatalf("only message is not a user message")
	}
	if !params.MaxCompletionTokens.Valid() || params.MaxCompletionTokens.Value != 50 {
		t.Fatalf("max tokens = %+v, want 50", params.MaxCompletionTokens)
	}
}
