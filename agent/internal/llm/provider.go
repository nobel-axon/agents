// Package llm provides LLM provider implementations for axon-agent.
package llm

import (
	"context"
)

// Provider defines the interface for LLM providers.
type Provider interface {
	// Name returns the provider name (e.g., "glm", "kimi", "claude").
	Name() string
	// Generate sends a prompt and returns the response.
	Generate(ctx context.Context, prompt string) (string, error)
	// GenerateWithSystem sends a prompt with a custom system message.
	GenerateWithSystem(ctx context.Context, system, prompt string) (string, error)
}

// Config holds configuration for an LLM provider.
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout int // seconds
}

// ProviderConfigs maps provider names to their configurations.
type ProviderConfigs struct {
	GLM    Config
	Kimi   Config
	Claude Config
}
