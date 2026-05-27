// Package openai implements llm.Client against the official OpenAI Go SDK using
// the Chat Completions API. Like the anthropic implementation it owns the
// per-call timeout, transient-error retry, and token/latency logging, and never
// logs prompt or reply contents; no SDK error type escapes the package.
//
// OpenAI performs prompt caching automatically for long, stable prompt prefixes
// (the llm.Request.System block), so there is no explicit cache breakpoint to
// set as there is with Anthropic.
package openai
