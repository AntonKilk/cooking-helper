package openai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	sdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/AntonKilk/cooking-helper/internal/llm"
)

const (
	defaultTimeout   = 30 * time.Second
	defaultMaxTokens = 1024
)

// Recommended cheap models for each role, mirroring the Sonnet/Haiku split.
// Verify the exact IDs against GET /v1/models for the target account.
const (
	// ModelGenerate handles week generation and swaps (variety and nuance).
	ModelGenerate llm.Model = "gpt-5.4-mini"
	// ModelCategorize handles cheap, high-volume work like ingredient categorization.
	ModelCategorize llm.Model = "gpt-5.4-nano"
)

// completionAPI is the slice of the SDK this client uses, kept as an interface
// so the build wiring is explicit and the surface is documented in one place.
type completionAPI interface {
	New(ctx context.Context, body sdk.ChatCompletionNewParams, opts ...option.RequestOption) (*sdk.ChatCompletion, error)
}

// Client is the OpenAI-backed implementation of llm.Client.
type Client struct {
	completions completionAPI
	timeout     time.Duration
	logger      *slog.Logger
}

// Option configures a Client.
type Option func(*Client)

// WithTimeout overrides the per-call timeout (default 30s).
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// WithLogger sets the logger used for token/latency lines (default slog.Default).
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) { c.logger = l }
}

// New returns a Client. The API key is read from the environment by the caller
// and passed in — it is never read or hardcoded here.
func New(apiKey string, opts ...Option) *Client {
	api := sdk.NewClient(option.WithAPIKey(apiKey))
	c := &Client{
		completions: &api.Chat.Completions,
		timeout:     defaultTimeout,
		logger:      slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Complete sends one request to OpenAI. Each attempt runs under its own timeout;
// transient failures are retried per llm.Retry.
func (c *Client) Complete(ctx context.Context, req llm.Request) (llm.Completion, error) {
	params := buildParams(req)

	var resp *sdk.ChatCompletion
	start := time.Now()
	err := llm.Retry(ctx, func() error {
		callCtx, cancel := context.WithTimeout(ctx, c.timeout)
		defer cancel()

		r, err := c.completions.New(callCtx, params)
		if err != nil {
			return classify(err)
		}
		resp = r
		return nil
	})
	if err != nil {
		return llm.Completion{}, fmt.Errorf("openai complete: %w", err)
	}

	comp := llm.Completion{
		Text: extractText(resp),
		Usage: llm.Usage{
			InputTokens:  int(resp.Usage.PromptTokens),
			OutputTokens: int(resp.Usage.CompletionTokens),
		},
	}

	c.logger.Info("llm complete",
		"model", string(req.Model),
		"input_tokens", comp.Usage.InputTokens,
		"output_tokens", comp.Usage.OutputTokens,
		"latency_ms", time.Since(start).Milliseconds(),
		"request_id", req.RequestID,
	)
	return comp, nil
}

func buildParams(req llm.Request) sdk.ChatCompletionNewParams {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	messages := make([]sdk.ChatCompletionMessageParamUnion, 0, 2)
	if req.System != "" {
		messages = append(messages, sdk.SystemMessage(req.System))
	}
	messages = append(messages, sdk.UserMessage(req.Prompt))

	return sdk.ChatCompletionNewParams{
		Model:               string(req.Model),
		MaxCompletionTokens: sdk.Int(int64(maxTokens)),
		Messages:            messages,
	}
}

func extractText(resp *sdk.ChatCompletion) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Message.Content
}

// classify maps an SDK error to an llm sentinel so callers and the retry helper
// never see provider types. Network/5xx/429 are transient (retryable); other
// 4xx are permanent; deadline exhaustion is ErrTimeout.
func classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.Join(llm.ErrTimeout, err)
	}

	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode >= 500 || apiErr.StatusCode == 429 {
			return errors.Join(llm.ErrTransient, err)
		}
		return err
	}

	// No HTTP status — treat as a network-level failure, which is retryable.
	return errors.Join(llm.ErrTransient, err)
}
