// Package anthropic implements llm.Client against the Anthropic Go SDK. It owns
// the per-call timeout, transient-error retry, prompt caching of the stable
// system block, and token/latency logging. It never logs prompt or reply
// contents, and no SDK error type escapes the package.
package anthropic
