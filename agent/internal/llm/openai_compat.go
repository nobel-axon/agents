package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// Retry configuration
const (
	maxRetries     = 5
	initialBackoff = 2 * time.Second
	maxBackoff     = 60 * time.Second
	backoffFactor  = 2.0
)

// OpenAICompatProvider implements Provider for OpenAI-compatible APIs (GLM, Kimi, OpenRouter).
type OpenAICompatProvider struct {
	name         string
	config       Config
	client       *http.Client
	extraHeaders map[string]string
}

// ChatMessage represents a message in the chat format.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest represents an OpenAI-compatible chat completion request.
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// ChatResponse represents an OpenAI-compatible chat completion response.
type ChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message      ChatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// NewOpenAICompatProvider creates a new OpenAI-compatible provider.
func NewOpenAICompatProvider(name string, config Config) *OpenAICompatProvider {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 30
	}
	return &OpenAICompatProvider{
		name:   name,
		config: config,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

// Name returns the provider name.
func (p *OpenAICompatProvider) Name() string {
	return p.name
}

// Generate sends a prompt and returns the response.
func (p *OpenAICompatProvider) Generate(ctx context.Context, prompt string) (string, error) {
	return p.GenerateWithSystem(ctx, "", prompt)
}

// GenerateWithSystem sends a prompt with a custom system message.
// Includes retry logic with exponential backoff for transient errors.
func (p *OpenAICompatProvider) GenerateWithSystem(ctx context.Context, system, prompt string) (string, error) {
	messages := []ChatMessage{}
	if system != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: system})
	}
	messages = append(messages, ChatMessage{Role: "user", Content: prompt})

	reqBody := ChatRequest{
		Model:       p.config.Model,
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   1024,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Add jitter to backoff (±25%)
			jitter := time.Duration(float64(backoff) * (0.75 + rand.Float64()*0.5))
			log.Printf("[%s] Retry attempt %d/%d after %v (error: %v)", p.name, attempt, maxRetries, jitter, lastErr)

			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(jitter):
			}
		}

		result, err := p.doRequest(ctx, jsonBody)
		if err == nil {
			if attempt > 0 {
				log.Printf("[%s] Request succeeded after %d retries", p.name, attempt)
			}
			return result, nil
		}

		// Check if error is retryable
		if !isRetryableError(err) {
			return "", err
		}

		lastErr = err

		// Exponential backoff with cap
		backoff = time.Duration(float64(backoff) * backoffFactor)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return "", fmt.Errorf("max retries exceeded: %w", lastErr)
}

// doRequest performs a single API request.
func (p *OpenAICompatProvider) doRequest(ctx context.Context, jsonBody []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	for k, v := range p.extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	// Server errors are retryable
	if resp.StatusCode >= 500 {
		return "", &retryableError{fmt.Errorf("server error %d: %s", resp.StatusCode, string(body))}
	}

	// Rate limit errors are retryable
	if resp.StatusCode == 429 {
		return "", &retryableError{fmt.Errorf("rate limited: %s", string(body))}
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if chatResp.Error != nil {
		errMsg := chatResp.Error.Message
		// Check for overload/rate limit messages
		if isOverloadedMessage(errMsg) {
			return "", &retryableError{fmt.Errorf("API error: %s", errMsg)}
		}
		return "", fmt.Errorf("API error: %s", errMsg)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// retryableError wraps an error to mark it as retryable.
type retryableError struct {
	err error
}

func (e *retryableError) Error() string {
	return e.err.Error()
}

func (e *retryableError) Unwrap() error {
	return e.err
}

// isRetryableError checks if an error should trigger a retry.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Check if it's explicitly marked as retryable
	var re *retryableError
	if ok := errors.As(err, &re); ok {
		return true
	}
	// Also check for common transient error patterns
	errStr := err.Error()
	return isOverloadedMessage(errStr)
}

// isOverloadedMessage checks if an error message indicates overload/rate limiting.
func isOverloadedMessage(msg string) bool {
	lowerMsg := strings.ToLower(msg)
	return strings.Contains(lowerMsg, "overload") ||
		strings.Contains(lowerMsg, "rate limit") ||
		strings.Contains(lowerMsg, "too many requests") ||
		strings.Contains(lowerMsg, "try again later") ||
		strings.Contains(lowerMsg, "capacity") ||
		strings.Contains(lowerMsg, "busy")
}

// NewGLMProvider creates a GLM provider.
func NewGLMProvider(apiKey string) *OpenAICompatProvider {
	return NewOpenAICompatProvider("glm", Config{
		APIKey:  apiKey,
		BaseURL: "https://open.bigmodel.cn/api/paas/v4/chat/completions",
		Model:   "glm-4-flash",
		Timeout: 30,
	})
}

// NewKimiProvider creates a Kimi provider.
func NewKimiProvider(apiKey string) *OpenAICompatProvider {
	return NewOpenAICompatProvider("kimi", Config{
		APIKey:  apiKey,
		BaseURL: "https://api.moonshot.ai/v1/chat/completions",
		Model:   "moonshot-v1-8k",
		Timeout: 60,
	})
}

// NewOpenRouterProvider creates an OpenRouter provider.
// If model is empty, defaults to moonshotai/kimi-k2.
func NewOpenRouterProvider(apiKey, model string) *OpenAICompatProvider {
	if model == "" {
		model = "moonshotai/kimi-k2"
	}
	p := NewOpenAICompatProvider("openrouter", Config{
		APIKey:  apiKey,
		BaseURL: "https://openrouter.ai/api/v1/chat/completions",
		Model:   model,
		Timeout: 90,
	})
	p.extraHeaders = map[string]string{
		"HTTP-Referer": "https://axon-arena.com",
		"X-Title":      "Axon Arena",
	}
	return p
}
