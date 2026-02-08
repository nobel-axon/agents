package llm

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// FailoverClient wraps multiple providers with automatic failover.
type FailoverClient struct {
	agentID   string
	primary   Provider
	failovers []Provider
	current   Provider // currently active provider
	maxRetries int
}

// NewFailoverClient creates a new failover client.
func NewFailoverClient(agentID string, primary Provider, failovers []Provider) *FailoverClient {
	return &FailoverClient{
		agentID:    agentID,
		primary:    primary,
		failovers:  failovers,
		current:    primary,
		maxRetries: 2,
	}
}

// Name returns the current active provider name.
func (c *FailoverClient) Name() string {
	return c.current.Name()
}

// Generate sends a prompt and returns the response with automatic failover.
func (c *FailoverClient) Generate(ctx context.Context, prompt string) (string, error) {
	return c.GenerateWithSystem(ctx, "", prompt)
}

// GenerateWithSystem sends a prompt with system message and handles failover.
func (c *FailoverClient) GenerateWithSystem(ctx context.Context, system, prompt string) (string, error) {
	providers := append([]Provider{c.primary}, c.failovers...)
	var lastErr error

	for _, provider := range providers {
		for retry := 0; retry <= c.maxRetries; retry++ {
			result, err := provider.GenerateWithSystem(ctx, system, prompt)
			if err == nil {
				c.current = provider
				return result, nil
			}

			lastErr = err

			// Only retry on server errors or timeouts
			if !isRetryable(err) {
				log.Printf("[%s] %s non-retryable error: %v", c.agentID, provider.Name(), err)
				break
			}

			log.Printf("[%s] %s retry %d: %v", c.agentID, provider.Name(), retry+1, err)

			if retry < c.maxRetries {
				time.Sleep(time.Duration(retry+1) * 500 * time.Millisecond)
			}
		}

		log.Printf("[%s] failing over from %s", c.agentID, provider.Name())
	}

	return "", fmt.Errorf("all providers failed: %w", lastErr)
}

// isRetryable checks if an error warrants a retry.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "server error 5") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "connection refused")
}

// CreateFailoverClient creates a failover client based on agent configuration.
func CreateFailoverClient(agentID, primaryLLM, glmKey, kimiKey, claudeKey string) *FailoverClient {
	var primary Provider
	var failovers []Provider

	glm := NewGLMProvider(glmKey)
	kimi := NewKimiProvider(kimiKey)
	claude := NewClaudeProvider(claudeKey)

	switch primaryLLM {
	case "glm":
		primary = glm
		failovers = []Provider{kimi, claude}
	case "kimi":
		primary = kimi
		failovers = []Provider{claude, glm}
	case "claude":
		primary = claude
		failovers = []Provider{glm, kimi}
	default:
		// Default to GLM
		primary = glm
		failovers = []Provider{kimi, claude}
	}

	return NewFailoverClient(agentID, primary, failovers)
}
