package question

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockLLMProvider is a mock implementation of llm.Provider for testing.
type mockLLMProvider struct {
	name      string
	responses []string
	errors    []error
	callIndex int
}

func (m *mockLLMProvider) Name() string {
	return m.name
}

func (m *mockLLMProvider) Generate(ctx context.Context, prompt string) (string, error) {
	if m.callIndex >= len(m.responses) {
		return "", errors.New("no more mock responses")
	}
	resp := m.responses[m.callIndex]
	var err error
	if m.callIndex < len(m.errors) {
		err = m.errors[m.callIndex]
	}
	m.callIndex++
	return resp, err
}

func (m *mockLLMProvider) GenerateWithSystem(ctx context.Context, system, prompt string) (string, error) {
	return m.Generate(ctx, prompt)
}

func TestParseJSONResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantQ    string
		wantA    string
		wantErr  bool
	}{
		{
			name:    "direct JSON",
			input:   `{"question": "What is 2+2?", "answer": "4", "variants": [], "format": "number"}`,
			wantQ:   "What is 2+2?",
			wantA:   "4",
			wantErr: false,
		},
		{
			name: "markdown wrapped",
			input: "```json\n" + `{"question": "What is the capital of France?", "answer": "Paris", "variants": ["paris"], "format": "name"}` + "\n```",
			wantQ:   "What is the capital of France?",
			wantA:   "Paris",
			wantErr: false,
		},
		{
			name: "markdown no language",
			input: "```\n" + `{"question": "Test?", "answer": "Yes", "variants": [], "format": "text"}` + "\n```",
			wantQ:   "Test?",
			wantA:   "Yes",
			wantErr: false,
		},
		{
			name:    "with prefix text",
			input:   `Here is your question: {"question": "What year?", "answer": "2024", "variants": [], "format": "number"}`,
			wantQ:   "What year?",
			wantA:   "2024",
			wantErr: false,
		},
		{
			name:    "with trailing text",
			input:   `{"question": "Who invented?", "answer": "Tesla", "variants": ["nikola tesla"], "format": "name"} Let me know if you need more.`,
			wantQ:   "Who invented?",
			wantA:   "Tesla",
			wantErr: false,
		},
		{
			name:    "no JSON",
			input:   "I cannot generate a question for you.",
			wantErr: true,
		},
		{
			name:    "empty response",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   `{"question": "incomplete`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := parseJSONResponse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Question != tt.wantQ {
				t.Errorf("question = %q, want %q", resp.Question, tt.wantQ)
			}
			if resp.Answer != tt.wantA {
				t.Errorf("answer = %q, want %q", resp.Answer, tt.wantA)
			}
		})
	}
}

func TestValidateLLMResponse(t *testing.T) {
	tests := []struct {
		name    string
		resp    *LLMQuestionResponse
		wantErr bool
	}{
		{
			name: "valid response",
			resp: &LLMQuestionResponse{
				Question: "What is 2+2?",
				Answer:   "4",
				Format:   "number",
			},
			wantErr: false,
		},
		{
			name: "empty question",
			resp: &LLMQuestionResponse{
				Question: "",
				Answer:   "4",
				Format:   "number",
			},
			wantErr: true,
		},
		{
			name: "empty answer",
			resp: &LLMQuestionResponse{
				Question: "What is 2+2?",
				Answer:   "",
				Format:   "number",
			},
			wantErr: true,
		},
		{
			name: "empty format",
			resp: &LLMQuestionResponse{
				Question: "What is 2+2?",
				Answer:   "4",
				Format:   "",
			},
			wantErr: true,
		},
		{
			name: "invalid format",
			resp: &LLMQuestionResponse{
				Question: "What is 2+2?",
				Answer:   "4",
				Format:   "unknown",
			},
			wantErr: true,
		},
		{
			name: "all valid formats - number",
			resp: &LLMQuestionResponse{
				Question: "Q?",
				Answer:   "A",
				Format:   "number",
			},
			wantErr: false,
		},
		{
			name: "all valid formats - hex",
			resp: &LLMQuestionResponse{
				Question: "Q?",
				Answer:   "A",
				Format:   "hex",
			},
			wantErr: false,
		},
		{
			name: "all valid formats - address",
			resp: &LLMQuestionResponse{
				Question: "Q?",
				Answer:   "A",
				Format:   "address",
			},
			wantErr: false,
		},
		{
			name: "all valid formats - name",
			resp: &LLMQuestionResponse{
				Question: "Q?",
				Answer:   "A",
				Format:   "name",
			},
			wantErr: false,
		},
		{
			name: "all valid formats - text",
			resp: &LLMQuestionResponse{
				Question: "Q?",
				Answer:   "A",
				Format:   "text",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLLMResponse(tt.resp)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCategoryToFormat(t *testing.T) {
	tests := []struct {
		category string
		want     string
	}{
		{"math", "number"},
		{"Mathematics", "number"},
		{"crypto", "text"},
		{"cryptocurrency", "text"},
		{"blockchain", "text"},
		{"Blockchain Technology", "text"},
		{"history", "name"},
		{"World History", "name"},
		{"science", "text"},
		{"geography", "name"},
		{"unknown", "text"},
		{"", "text"},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			got := categoryToFormat(tt.category)
			if got != tt.want {
				t.Errorf("categoryToFormat(%q) = %q, want %q", tt.category, got, tt.want)
			}
		})
	}
}

func TestGenerateQuestionWithLLM_Success(t *testing.T) {
	// Mock LLM that returns a valid question, then answers its own question correctly
	mock := &mockLLMProvider{
		name: "test",
		responses: []string{
			// Generation response
			`{"question": "What is 2+2?", "answer": "4", "variants": ["four"], "format": "number"}`,
			// Self-validation response (must match answer)
			"4",
		},
	}

	g := NewGenerator(WithLLMClient(mock))

	resp, err := g.GenerateQuestionWithLLM(context.Background(), "match-1", "math", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Question != "What is 2+2?" {
		t.Errorf("question = %q, want %q", resp.Question, "What is 2+2?")
	}
	if resp.FormatHint != "number" {
		t.Errorf("formatHint = %q, want %q", resp.FormatHint, "number")
	}
	if resp.AnswerHash == "" {
		t.Error("expected non-empty answer hash")
	}

	// Verify answer was stored
	stored, ok := g.GetAnswer("match-1")
	if !ok {
		t.Fatal("expected stored answer")
	}
	if stored.Answer != "4" {
		t.Errorf("stored answer = %q, want %q", stored.Answer, "4")
	}
}

func TestGenerateQuestionWithLLM_SelfValidationFails(t *testing.T) {
	// Mock LLM that returns a valid question, but answers differently
	mock := &mockLLMProvider{
		name: "test",
		responses: []string{
			// Generation response
			`{"question": "What is the capital of France?", "answer": "Paris", "variants": [], "format": "name"}`,
			// Self-validation response (wrong answer)
			"London",
		},
	}

	g := NewGenerator(WithLLMClient(mock))

	_, err := g.GenerateQuestionWithLLM(context.Background(), "match-2", "geography", 1)
	if err == nil {
		t.Fatal("expected self-validation error")
	}
	if !contains(err.Error(), "self-validation") {
		t.Errorf("error should mention self-validation: %v", err)
	}
}

func TestGenerateQuestionWithLLM_VariantMatch(t *testing.T) {
	// Mock LLM that returns a variant during self-validation
	mock := &mockLLMProvider{
		name: "test",
		responses: []string{
			// Generation response
			`{"question": "Who invented the telephone?", "answer": "Alexander Graham Bell", "variants": ["bell", "graham bell"], "format": "name"}`,
			// Self-validation response (matches variant)
			"Bell",
		},
	}

	g := NewGenerator(WithLLMClient(mock))

	resp, err := g.GenerateQuestionWithLLM(context.Background(), "match-3", "history", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Question != "Who invented the telephone?" {
		t.Errorf("question = %q", resp.Question)
	}
}

func TestGenerateQuestionWithLLM_InvalidJSON(t *testing.T) {
	mock := &mockLLMProvider{
		name: "test",
		responses: []string{
			"I cannot generate questions.",
		},
	}

	g := NewGenerator(WithLLMClient(mock))

	_, err := g.GenerateQuestionWithLLM(context.Background(), "match-4", "math", 1)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGenerateQuestionWithLLM_LLMError(t *testing.T) {
	mock := &mockLLMProvider{
		name:      "test",
		responses: []string{""},
		errors:    []error{errors.New("API rate limit exceeded")},
	}

	g := NewGenerator(WithLLMClient(mock))

	_, err := g.GenerateQuestionWithLLM(context.Background(), "match-5", "math", 1)
	if err == nil {
		t.Fatal("expected error for LLM failure")
	}
}

func TestGenerateQuestion_LLMFailure_RetriesUntilCancel(t *testing.T) {
	// Mock LLM that always fails — with retry, this loops until context cancelled
	mock := &mockLLMProvider{
		name:      "test",
		responses: []string{"", "", "", ""},
		errors:    []error{errors.New("LLM unavailable"), errors.New("LLM unavailable"), errors.New("LLM unavailable"), errors.New("LLM unavailable")},
	}

	g := NewGenerator(WithLLMClient(mock))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := g.GenerateQuestionWithContext(ctx, "match-6", "math", 1, nil)
	if err == nil {
		t.Fatal("expected error when LLM fails and context cancelled")
	}
	if !contains(err.Error(), "cancelled") {
		t.Errorf("expected cancellation error, got: %v", err)
	}
}

func TestGenerateQuestion_NoLLM_ReturnsError(t *testing.T) {
	// Generator without LLM client — should error immediately
	g := NewGenerator()

	_, err := g.GenerateQuestion("match-7", "science", 2, nil)
	if err == nil {
		t.Fatal("expected error when no LLM configured")
	}
}

// contains checks if substr is in s.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
