// Package llm defines a provider-agnostic Client interface for language-model
// calls, with interchangeable provider implementations (anthropic/, openai/)
// and version-controlled prompts/. Callers select a model by Role, never by a
// provider-specific name. Handlers and services depend on the interface, never
// on an SDK directly.
package llm
