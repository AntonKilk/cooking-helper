package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Model identifies which Claude model serves a request. Sonnet handles
// generation and swaps (variety and nuance); Haiku handles cheaper,
// high-volume work like ingredient categorization.
type Model string

const (
	ModelSonnet Model = "claude-sonnet-4-6"
	ModelHaiku  Model = "claude-haiku-4-5-20251001"
)

// Sentinel errors let callers branch on failure mode without depending on the
// underlying SDK — no provider error type ever escapes this package.
var (
	// ErrInvalidJSON means the model's reply could not be parsed as the
	// requested type even after one repair attempt.
	ErrInvalidJSON = errors.New("llm: response was not valid JSON")
	// ErrTransient marks a retryable provider failure (network or 5xx/429).
	ErrTransient = errors.New("llm: transient provider error")
	// ErrTimeout means a call exceeded its deadline.
	ErrTimeout = errors.New("llm: request timed out")
)

// Request is one model call. System holds the stable, cacheable block
// (household profile, disliked ingredients, pantry, recent feedback); Prompt
// holds the variable trigger. Schema, when set, is echoed back into the repair
// hint if the first reply is not valid JSON.
type Request struct {
	Model     Model
	System    string
	Prompt    string
	Schema    string
	MaxTokens int
	RequestID string
}

// Usage reports token counts for budget monitoring.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Completion is the raw model reply plus its token usage.
type Completion struct {
	Text  string
	Usage Usage
}

// Client is the provider-agnostic entry point. Implementations own transport
// concerns (timeout, retry, logging); they do not parse the reply.
type Client interface {
	Complete(ctx context.Context, req Request) (Completion, error)
}

// Generate runs req through c and decodes the reply into T. If the first reply
// is not valid JSON, it retries once with a clarifying hint appended to the
// trigger prompt; a second failure returns ErrInvalidJSON. Transport errors
// from the client are returned without a repair attempt.
func Generate[T any](ctx context.Context, c Client, req Request) (T, error) {
	var zero T

	comp, err := c.Complete(ctx, req)
	if err != nil {
		return zero, fmt.Errorf("llm generate: %w", err)
	}

	out, err := decode[T](comp.Text)
	if err == nil {
		return out, nil
	}

	repair := req
	repair.Prompt = req.Prompt + repairHint(req.Schema, err)

	comp, err = c.Complete(ctx, repair)
	if err != nil {
		return zero, fmt.Errorf("llm generate (repair): %w", err)
	}

	out, err = decode[T](comp.Text)
	if err != nil {
		return zero, fmt.Errorf("llm generate: %w", ErrInvalidJSON)
	}
	return out, nil
}

func decode[T any](text string) (T, error) {
	var out T
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return out, err
	}
	return out, nil
}

func repairHint(schema string, err error) string {
	hint := fmt.Sprintf("\n\nYour previous reply could not be parsed as JSON (%v). "+
		"Reply with ONLY a single valid JSON value, with no surrounding text or markdown fences.", err)
	if schema != "" {
		hint += " It must match this schema: " + schema
	}
	return hint
}
