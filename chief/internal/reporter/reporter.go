// Package reporter provides an HTTP client for pushing match lifecycle data to axon-server.
package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/axon-arena/axon-chief/internal/match"
)

// Reporter pushes match lifecycle data to the axon-server internal API.
type Reporter struct {
	baseURL    string
	secret     string
	httpClient *http.Client
}

// NewReporter creates a new Reporter.
func NewReporter(serverURL, secret string) *Reporter {
	return &Reporter{
		baseURL: serverURL,
		secret:  secret,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// MatchUpdateRequest is the request for flexible match field updates.
type MatchUpdateRequest struct {
	MatchID         int64  `json:"matchId"`
	Phase           string `json:"phase,omitempty"`
	QuestionText    string `json:"questionText,omitempty"`
	Category        string `json:"category,omitempty"`
	Difficulty      int    `json:"difficulty,omitempty"`
	FormatHint      string `json:"formatHint,omitempty"`
	AnswerHash      string `json:"answerHash,omitempty"`
	EntryFee        string `json:"entryFee,omitempty"`
	AnswerFee       string `json:"answerFee,omitempty"`
	RegistrationEnd string `json:"registrationEnd,omitempty"`
	AnswerTimeout   string `json:"answerTimeout,omitempty"`
}

// AnswerResultRequest is the request for recording answer evaluation results.
type AnswerResultRequest struct {
	MatchID       int64           `json:"matchId"`
	AgentAddr     string          `json:"agentAddr"`
	AttemptNumber int             `json:"attemptNumber"`
	IsCorrect     bool            `json:"isCorrect"`
	Consensus     string          `json:"consensus"`
	Confidence    float64         `json:"confidence"`
	TotalScore    int             `json:"totalScore,omitempty"`
	Agreement     string          `json:"agreement,omitempty"`
	Reasoning     string          `json:"reasoning,omitempty"`
	Evaluations   json.RawMessage `json:"evaluations,omitempty"`
}

// SettleMatchRequest is the request for settlement reporting.
type SettleMatchRequest struct {
	MatchID      int64  `json:"matchId"`
	WinnerAddr   string `json:"winnerAddr"`
	PrizeMON     string `json:"prizeMon"`
	PrizeNeuron  string `json:"prizeNeuron"`
	SettleTxHash string `json:"settleTxHash,omitempty"`
}

// CommentaryRequest is the request for posting commentary.
type CommentaryRequest struct {
	MatchID   int64  `json:"matchId"`
	AgentID   string `json:"agentId"`
	EventType string `json:"eventType"`
	Text      string `json:"text"`
}

// AnswerSubmittedRequest is the request for recording an answer submission.
type AnswerSubmittedRequest struct {
	MatchID       int64  `json:"matchId"`
	Agent         string `json:"agent"`
	Answer        string `json:"answer"`
	Reasoning     string `json:"reasoning,omitempty"`
	AttemptNumber int    `json:"attemptNumber"`
	NeuronBurned  string `json:"neuronBurned"`
}

// PersonalityPayload is the payload for reporting personalities.
type PersonalityPayload struct {
	MatchID       int64           `json:"matchId"`
	Personalities json.RawMessage `json:"personalities"`
	JudgePanel    json.RawMessage `json:"judgePanel"`
}

// AnswerRevealedPayload is the payload for reporting answer reveals.
type AnswerRevealedPayload struct {
	MatchID int64  `json:"matchId"`
	Answer  string `json:"answer"`
	Salt    string `json:"salt"`
}

// ReportMatchUpdate sends a match update to the server.
func (r *Reporter) ReportMatchUpdate(ctx context.Context, req MatchUpdateRequest) error {
	return r.post(ctx, "/internal/match-update", req)
}

// ReportPersonalities sends personality data to the server's personality endpoint.
func (r *Reporter) ReportPersonalities(ctx context.Context, matchID string, personalities []match.Personality, judgePanel []int) error {
	personalitiesJSON, err := json.Marshal(personalities)
	if err != nil {
		return fmt.Errorf("marshal personalities: %w", err)
	}
	judgePanelJSON, err := json.Marshal(judgePanel)
	if err != nil {
		return fmt.Errorf("marshal judge panel: %w", err)
	}
	return r.post(ctx, "/internal/match-personalities", PersonalityPayload{
		MatchID:       parseMatchID(matchID),
		Personalities: personalitiesJSON,
		JudgePanel:    judgePanelJSON,
	})
}

// ReportAnswerResult sends an answer evaluation result to the server.
func (r *Reporter) ReportAnswerResult(ctx context.Context, req AnswerResultRequest) error {
	return r.post(ctx, "/internal/answer-result", req)
}

// ReportAnswerRevealed sends an answer reveal to the server.
func (r *Reporter) ReportAnswerRevealed(ctx context.Context, matchID string, answer string, salt string) error {
	return r.post(ctx, "/internal/answer-revealed", AnswerRevealedPayload{
		MatchID: parseMatchID(matchID),
		Answer:  answer,
		Salt:    salt,
	})
}

// ReportSettlement sends a settlement result to the server.
func (r *Reporter) ReportSettlement(ctx context.Context, req SettleMatchRequest) error {
	return r.post(ctx, "/internal/match-settled", req)
}

// ReportCommentary sends commentary to the server.
func (r *Reporter) ReportCommentary(ctx context.Context, req CommentaryRequest) error {
	return r.post(ctx, "/internal/commentary", req)
}

// ReportAnswerSubmitted sends an answer submission to the server.
func (r *Reporter) ReportAnswerSubmitted(ctx context.Context, req AnswerSubmittedRequest) error {
	return r.post(ctx, "/internal/answer-submitted", req)
}

// BountyAnswerResultRequest is the request for recording bounty answer evaluation results.
type BountyAnswerResultRequest struct {
	BountyID    int64           `json:"bountyId"`
	AgentAddr   string          `json:"agentAddr"`
	TotalScore  int             `json:"totalScore"`
	Agreement   string          `json:"agreement"`
	Evaluations json.RawMessage `json:"evaluations,omitempty"`
}

// ReportBountyAnswerResult sends a bounty answer evaluation result to the server.
func (r *Reporter) ReportBountyAnswerResult(ctx context.Context, req BountyAnswerResultRequest) error {
	return r.post(ctx, "/internal/bounty-answer-result", req)
}

// ReputationUpdateRequest is the request for reporting reputation updates.
type ReputationUpdateRequest struct {
	AgentAddr string `json:"agentAddr"`
	Score     int    `json:"score"`
}

// ReportReputationUpdate sends a reputation update to the server.
func (r *Reporter) ReportReputationUpdate(ctx context.Context, agentAddr string, score int) error {
	return r.post(ctx, "/internal/reputation-updated", ReputationUpdateRequest{
		AgentAddr: agentAddr,
		Score:     score,
	})
}

const maxRetries = 3

// post is the shared HTTP POST helper with retry logic.
func (r *Reporter) post(ctx context.Context, path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("reporter: marshal body: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", r.baseURL+path, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("reporter: create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if r.secret != "" {
			req.Header.Set("X-Internal-Secret", r.secret)
		}

		resp, err := r.httpClient.Do(req)
		if err != nil {
			lastErr = err
			log.Printf("Reporter: POST %s attempt %d failed: %v", path, attempt+1, err)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("Reporter: POST %s -> %d", path, resp.StatusCode)
			return nil
		}

		lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
		log.Printf("Reporter: POST %s attempt %d: %v", path, attempt+1, lastErr)
	}

	return fmt.Errorf("reporter: POST %s failed after %d attempts: %w", path, maxRetries, lastErr)
}

// parseMatchID converts a string match ID to int64.
func parseMatchID(s string) int64 {
	var id int64
	fmt.Sscanf(s, "%d", &id)
	return id
}
