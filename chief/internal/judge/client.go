// Package judge provides judge agent coordination for axon-chief.
package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an HTTP client for communicating with a judge agent.
type Client struct {
	endpoint   string
	httpClient *http.Client
	agentID    string
}

// NewClient creates a new judge client.
func NewClient(endpoint string, timeout time.Duration) *Client {
	return &Client{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// HealthResponse is the response from the health endpoint.
type HealthResponse struct {
	Status        string `json:"status"`
	Provider      string `json:"provider"`
	MatchesActive int    `json:"matchesActive"`
}

// Health checks if the judge agent is healthy.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.endpoint+"/health", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}

	var health HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, err
	}

	return &health, nil
}

// GenerateQuestionRequest is the request for question generation.
type GenerateQuestionRequest struct {
	MatchID       string   `json:"matchId"`
	Category      string   `json:"category"`
	Difficulty    int      `json:"difficulty"`
	ExcludeRecent []string `json:"excludeRecent,omitempty"`
}

// GenerateQuestionResponse is the response from question generation.
type GenerateQuestionResponse struct {
	Question   string `json:"question"`
	AnswerHash string `json:"answerHash"`
	FormatHint string `json:"formatHint"`
	Category   string `json:"category"`
	Difficulty int    `json:"difficulty"`
}

// GenerateQuestion requests the judge to generate a question.
func (c *Client) GenerateQuestion(ctx context.Context, matchID, category string, difficulty int, excludeRecent []string) (*GenerateQuestionResponse, error) {
	reqBody := GenerateQuestionRequest{
		MatchID:       matchID,
		Category:      category,
		Difficulty:    difficulty,
		ExcludeRecent: excludeRecent,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/generate-question", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("generate question failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result GenerateQuestionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// VerifyAnswerRequest is the request for answer verification.
type VerifyAnswerRequest struct {
	MatchID    string `json:"matchId"`
	Submitted  string `json:"submitted"`
	Question   string `json:"question"`
	FormatHint string `json:"formatHint"`
}

// VerifyAnswerResponse is the response from answer verification.
type VerifyAnswerResponse struct {
	Correct    bool    `json:"correct"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
	Method     string  `json:"method"` // "exact_match" or "llm_reasoning"
}

// VerifyAnswer requests the judge to verify an answer.
func (c *Client) VerifyAnswer(ctx context.Context, matchID, submitted, question, formatHint string) (*VerifyAnswerResponse, error) {
	reqBody := VerifyAnswerRequest{
		MatchID:    matchID,
		Submitted:  submitted,
		Question:   question,
		FormatHint: formatHint,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/verify-answer", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("verify answer failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result VerifyAnswerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// RevealRequest is the request for revealing an answer.
type RevealRequest struct {
	MatchID string `json:"matchId"`
}

// RevealResponse is the response from answer reveal.
type RevealResponse struct {
	Answer string `json:"answer"`
	Salt   string `json:"salt"`
}

// Reveal requests the judge to reveal the answer.
func (c *Client) Reveal(ctx context.Context, matchID string) (*RevealResponse, error) {
	reqBody := RevealRequest{
		MatchID: matchID,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/reveal", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("reveal failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result RevealResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ValidateQuestionRequest is the request for question validation.
type ValidateQuestionRequest struct {
	MatchID    string `json:"matchId"`
	Question   string `json:"question"`
	FormatHint string `json:"formatHint"`
	Category   string `json:"category"`
	Difficulty int    `json:"difficulty"`
	// Note: No answer or answerHash - verifiers don't see the answer
}

// ValidateQuestionResponse is the response from question validation.
type ValidateQuestionResponse struct {
	Valid      bool     `json:"valid"`
	Confidence float64  `json:"confidence"`
	Issues     []string `json:"issues,omitempty"`
	Reasoning  string   `json:"reasoning,omitempty"`
}

// ValidateQuestion requests the judge to validate a question.
// This is used by verifier judges to check if a question is suitable.
func (c *Client) ValidateQuestion(ctx context.Context, req *ValidateQuestionRequest) (*ValidateQuestionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/validate-question", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("validate question failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result ValidateQuestionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// CommentaryRequest is the request for generating commentary.
type CommentaryRequest struct {
	MatchID    string `json:"matchId"`
	EventType  string `json:"eventType"`
	Agent      string `json:"agent,omitempty"`
	Answer     string `json:"answer,omitempty"`
	IsCorrect  bool   `json:"isCorrect,omitempty"`
	Winner     string `json:"winner,omitempty"`
	Prize      string `json:"prize,omitempty"`
}

// CommentaryResponse is the response from commentary generation.
type CommentaryResponse struct {
	Commentary string `json:"commentary"`
	Tone       string `json:"tone"`
}

// Commentary requests the judge to generate commentary.
func (c *Client) Commentary(ctx context.Context, req *CommentaryRequest) (*CommentaryResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/commentary", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("commentary failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result CommentaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Personality represents a judge personality for answer evaluation.
type Personality struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Perspective string   `json:"perspective"`
	Values      []string `json:"values"`
	Style       string   `json:"style"`
}

// GeneratePersonalitiesRequest is the request for generating match personalities.
type GeneratePersonalitiesRequest struct {
	MatchID  string `json:"matchId"`
	Category string `json:"category"`
}

// GeneratePersonalitiesResponse is the response from personality generation.
type GeneratePersonalitiesResponse struct {
	MatchID       string        `json:"matchId"`
	Personalities []Personality `json:"personalities"`
}

// GeneratePersonalities requests the judge to generate personalities for a match.
func (c *Client) GeneratePersonalities(ctx context.Context, matchID, category string) (*GeneratePersonalitiesResponse, error) {
	reqBody := GeneratePersonalitiesRequest{
		MatchID:  matchID,
		Category: category,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/generate-personalities", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("generate personalities failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result GeneratePersonalitiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetPersonalities retrieves the personalities assigned to a match.
func (c *Client) GetPersonalities(ctx context.Context, matchID string) (*GeneratePersonalitiesResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.endpoint+"/personalities/"+matchID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get personalities failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result GeneratePersonalitiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// EvaluateAnswerRequest is the request for personality-based answer evaluation.
type EvaluateAnswerRequest struct {
	MatchID       string        `json:"matchId"`
	Question      string        `json:"question"`
	Answer        string        `json:"answer"`
	Reasoning     string        `json:"reasoning,omitempty"`
	Personalities []Personality `json:"personalities,omitempty"`
}

// PersonalityEvaluation is a single personality's evaluation of an answer.
type PersonalityEvaluation struct {
	PersonalityID   string `json:"personalityId"`
	PersonalityName string `json:"personalityName"`
	Score           int    `json:"score"`
	Convinced       string `json:"convinced"`
	Concerns        string `json:"concerns"`
	Verdict         string `json:"verdict"`
	AgreeLevel      string `json:"agreeLevel"`
}

// EvaluateAnswerResponse is the response from personality-based evaluation.
type EvaluateAnswerResponse struct {
	MatchID     string                  `json:"matchId"`
	TotalScore  int                     `json:"totalScore"`
	Agreement   string                  `json:"agreement"`
	Evaluations []PersonalityEvaluation `json:"evaluations"`
}

// EvaluateAnswer requests personality-based evaluation of an answer.
func (c *Client) EvaluateAnswer(ctx context.Context, matchID, question, answer, reasoning string, personalities []Personality) (*EvaluateAnswerResponse, error) {
	reqBody := EvaluateAnswerRequest{
		MatchID:       matchID,
		Question:      question,
		Answer:        answer,
		Reasoning:     reasoning,
		Personalities: personalities,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/evaluate-answer", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("evaluate answer failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result EvaluateAnswerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}
