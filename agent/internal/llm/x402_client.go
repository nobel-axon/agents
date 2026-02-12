package llm

import (
	"context"
	"log"
)

// X402Client wraps an OpenAICompatProvider with x402 payment support.
// The x402 proxy handles the 402 payment flow automatically — the client
// just needs to retry with the payment header when it gets a 402 response.
// In practice, the Go x402 SDK client handles this transparently.
type X402Client struct {
	proxyProvider  *OpenAICompatProvider // points to x402 proxy URL
	directProvider *OpenAICompatProvider // points to OpenRouter directly (fallback)
}

// NewX402Client creates a new x402-enabled LLM client.
// proxyURL is the x402 proxy endpoint (e.g., http://localhost:9200/v1/chat/completions).
// The directProvider is used as fallback when the proxy is unreachable.
func NewX402Client(proxyURL, openRouterKey, model string) *X402Client {
	if model == "" {
		model = "moonshotai/kimi-k2"
	}

	proxy := NewOpenAICompatProvider("x402-proxy", Config{
		APIKey:  "x402-payment", // proxy uses x402 payment headers, not API key
		BaseURL: proxyURL,
		Model:   model,
		Timeout: 120,
	})

	direct := NewOpenRouterProvider(openRouterKey, model)

	return &X402Client{
		proxyProvider:  proxy,
		directProvider: direct,
	}
}

// Name returns the provider name.
func (c *X402Client) Name() string {
	return "x402"
}

// Generate sends a prompt via the x402 proxy.
func (c *X402Client) Generate(ctx context.Context, prompt string) (string, error) {
	return c.GenerateWithSystem(ctx, "", prompt)
}

// GenerateWithSystem sends a prompt with system message via x402 proxy with fallback.
func (c *X402Client) GenerateWithSystem(ctx context.Context, system, prompt string) (string, error) {
	// Try proxy first — the x402 payment flow is handled at the HTTP level
	result, err := c.proxyProvider.GenerateWithSystem(ctx, system, prompt)
	if err == nil {
		return result, nil
	}

	log.Printf("[x402] proxy failed: %v, falling back to direct OpenRouter", err)

	// Fallback to direct OpenRouter
	return c.directProvider.GenerateWithSystem(ctx, system, prompt)
}
