package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/axon-arena/axon-agent/internal/llm"
	"github.com/axon-arena/axon-agent/internal/question"
)

// mockProvider implements llm.Provider for testing.
type mockProvider struct {
	name      string
	responses []string
	callIndex int
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Generate(ctx context.Context, prompt string) (string, error) {
	if m.callIndex >= len(m.responses) {
		// Cycle back to last response
		return m.responses[len(m.responses)-1], nil
	}
	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp, nil
}

func (m *mockProvider) GenerateWithSystem(ctx context.Context, system, prompt string) (string, error) {
	return m.Generate(ctx, prompt)
}

func setupTestServer() (*Server, *gin.Engine) {
	gin.SetMode(gin.TestMode)

	// Create a mock LLM that returns debate questions (no self-validation needed)
	mock := &mockProvider{
		name: "mock",
		responses: []string{
			`{"question": "What is the most likely explanation for the Fermi Paradox?", "answer": "The Great Filter hypothesis suggests civilizations destroy themselves.", "variants": ["Zoo hypothesis", "Dark forest theory"], "format": "debate"}`,
			`{"question": "Is mathematics discovered or invented?", "answer": "Mathematics is discovered.", "variants": ["invented", "both"], "format": "debate"}`,
			`{"question": "What caused the Bronze Age Collapse?", "answer": "Systems collapse from cascading failures.", "variants": ["Sea Peoples", "climate change"], "format": "debate"}`,
			`{"question": "Does free will exist?", "answer": "Compatibilism offers the best framework.", "variants": ["determinism", "libertarian free will"], "format": "debate"}`,
		},
	}
	failoverClient := llm.NewFailoverClient("test-agent", mock, nil)

	srv := NewServer(Config{
		AgentID:          "agent-1",
		LLMClient:        failoverClient,
		GeneratorOptions: []question.GeneratorOption{question.WithLLMClient(mock)},
	})

	r := gin.New()
	srv.SetupRoutes(r)

	return srv, r
}

func TestHealthEndpoint(t *testing.T) {
	_, r := setupTestServer()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "mock", resp.Provider)
	assert.Equal(t, 0, resp.MatchesActive)
}

func TestGenerateQuestionEndpoint(t *testing.T) {
	_, r := setupTestServer()

	body := `{"matchId": "test-match", "category": "philosophy", "difficulty": 2}`
	req := httptest.NewRequest("POST", "/generate-question", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp question.GenerateResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotEmpty(t, resp.Question)
	assert.NotEmpty(t, resp.AnswerHash)
	assert.Equal(t, "debate", resp.FormatHint)
}

func TestGenerateQuestionEndpoint_BadRequest(t *testing.T) {
	_, r := setupTestServer()

	// Missing required matchId field
	body := `{"category": "crypto", "difficulty": 2}`
	req := httptest.NewRequest("POST", "/generate-question", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestVerifyAnswerEndpoint(t *testing.T) {
	_, r := setupTestServer()

	// First generate a question to store the answer
	genBody := `{"matchId": "verify-match", "category": "philosophy", "difficulty": 2}`
	genReq := httptest.NewRequest("POST", "/generate-question", strings.NewReader(genBody))
	genReq.Header.Set("Content-Type", "application/json")
	genW := httptest.NewRecorder()
	r.ServeHTTP(genW, genReq)
	require.Equal(t, http.StatusOK, genW.Code)

	// Now verify — the answer is a debate response, so we check the flow works
	verifyBody := `{
		"matchId": "verify-match",
		"submitted": "The Great Filter hypothesis suggests civilizations destroy themselves.",
		"question": "What is the most likely explanation for the Fermi Paradox?",
		"formatHint": "debate"
	}`
	verifyReq := httptest.NewRequest("POST", "/verify-answer", strings.NewReader(verifyBody))
	verifyReq.Header.Set("Content-Type", "application/json")
	verifyW := httptest.NewRecorder()
	r.ServeHTTP(verifyW, verifyReq)

	assert.Equal(t, http.StatusOK, verifyW.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(verifyW.Body.Bytes(), &resp)
	require.NoError(t, err)
}

func TestRevealEndpoint(t *testing.T) {
	_, r := setupTestServer()

	// First generate a question
	genBody := `{"matchId": "reveal-match", "category": "philosophy", "difficulty": 2}`
	genReq := httptest.NewRequest("POST", "/generate-question", strings.NewReader(genBody))
	genReq.Header.Set("Content-Type", "application/json")
	genW := httptest.NewRecorder()
	r.ServeHTTP(genW, genReq)
	require.Equal(t, http.StatusOK, genW.Code)

	// Now reveal the answer
	revealBody := `{"matchId": "reveal-match"}`
	revealReq := httptest.NewRequest("POST", "/reveal", strings.NewReader(revealBody))
	revealReq.Header.Set("Content-Type", "application/json")
	revealW := httptest.NewRecorder()
	r.ServeHTTP(revealW, revealReq)

	assert.Equal(t, http.StatusOK, revealW.Code)

	var resp question.RevealResponse
	err := json.Unmarshal(revealW.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotEmpty(t, resp.Answer)
	assert.True(t, strings.HasPrefix(resp.Salt, "0x"))
}

func TestRevealEndpoint_NotFound(t *testing.T) {
	_, r := setupTestServer()

	body := `{"matchId": "nonexistent-match"}`
	req := httptest.NewRequest("POST", "/reveal", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
