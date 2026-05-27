package anthropic

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/AntonKilk/cooking-helper/internal/llm"
)

const (
	defaultTimeout   = 30 * time.Second
	defaultMaxTokens = 1024
)

// messageAPI is the slice of the SDK this client uses, kept as an interface so
// the build wiring is explicit and the type is documented in one place.
type messageAPI interface {
	New(ctx context.Context, body sdk.MessageNewParams, opts ...option.RequestOption) (*sdk.Message, error)
}

// Client is the Anthropic-backed implementation of llm.Client.
type Client struct {
	messages messageAPI
	timeout  time.Duration
	logger   *slog.Logger
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
		messages: &api.Messages,
		timeout:  defaultTimeout,
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Complete sends one request to Anthropic. Each attempt runs under its own
// timeout; transient failures are retried per llm.Retry.
func (c *Client) Complete(ctx context.Context, req llm.Request) (llm.Completion, error) {
	params := buildParams(req)

	var msg *sdk.Message
	start := time.Now()
	err := llm.Retry(ctx, func() error {
		callCtx, cancel := context.WithTimeout(ctx, c.timeout)
		defer cancel()

		m, err := c.messages.New(callCtx, params)
		if err != nil {
			return classify(err)
		}
		msg = m
		return nil
	})
	if err != nil {
		return llm.Completion{}, fmt.Errorf("anthropic complete: %w", err)
	}

	comp := llm.Completion{
		Text: extractText(msg),
		Usage: llm.Usage{
			InputTokens:  int(msg.Usage.InputTokens),
			OutputTokens: int(msg.Usage.OutputTokens),
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

func buildParams(req llm.Request) sdk.MessageNewParams {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	params := sdk.MessageNewParams{
		Model:     sdk.Model(req.Model),
		MaxTokens: int64(maxTokens),
		Messages: []sdk.MessageParam{
			sdk.NewUserMessage(sdk.NewTextBlock(req.Prompt)),
		},
	}

	// Mark the stable block as a prompt-cache breakpoint so repeated calls only
	// pay for the variable trigger.
	if req.System != "" {
		params.System = []sdk.TextBlockParam{{
			Text:         req.System,
			CacheControl: sdk.NewCacheControlEphemeralParam(),
		}}
	}
	return params
}

func extractText(msg *sdk.Message) string {
	var b strings.Builder
	for _, block := range msg.Content {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	return b.String()
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
