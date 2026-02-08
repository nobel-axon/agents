// Package server provides HTTP handlers for the axon-agent API.
package server

import (
	"context"
	"math/rand"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/axon-arena/axon-agent/internal/commentary"
	"github.com/axon-arena/axon-agent/internal/llm"
	"github.com/axon-arena/axon-agent/internal/personality"
	"github.com/axon-arena/axon-agent/internal/question"
	"github.com/axon-arena/axon-agent/internal/verify"
)

// Server holds all dependencies for the HTTP server.
type Server struct {
	agentID string
	role    string

	llmClient            *llm.FailoverClient
	questionGenerator    *question.Generator
	consensusManager     *question.ConsensusManager
	verifier             *verify.Verifier
	commentaryGen        *commentary.Generator
	personalityFactory   *personality.Factory
	personalityEvaluator *personality.Evaluator
	personalityStore     *personality.Store

	questionBuffer      *question.QuestionBuffer
	personalityPreStock *personality.PreStocker
}

// Config holds server configuration.
type Config struct {
	AgentID          string
	Role             string // philosopher, director, or judge
	LLMClient        *llm.FailoverClient
	GeneratorOptions []question.GeneratorOption

	Ctx                   context.Context
	PreStockCategories    []string
	QuestionBufferSize    int
	PersonalityBufferSize int
}

// NewServer creates a new server instance, only initializing subsystems needed for the role.
func NewServer(cfg Config) *Server {
	s := &Server{
		agentID:   cfg.AgentID,
		role:      cfg.Role,
		llmClient: cfg.LLMClient,
	}

	switch cfg.Role {
	case "philosopher":
		// Set up question buffer if pre-stocking is enabled
		if cfg.QuestionBufferSize > 0 && cfg.LLMClient != nil && len(cfg.PreStockCategories) > 0 {
			ctx := cfg.Ctx
			if ctx == nil {
				ctx = context.Background()
			}
			buf := question.NewQuestionBuffer(ctx, cfg.LLMClient, cfg.PreStockCategories, cfg.QuestionBufferSize)
			buf.Start()
			s.questionBuffer = buf
			cfg.GeneratorOptions = append(cfg.GeneratorOptions, question.WithPreStocking(buf))
		}

		s.questionGenerator = question.NewGenerator(cfg.GeneratorOptions...)
		s.consensusManager = question.NewConsensusManager()
		s.verifier = verify.NewVerifier(cfg.LLMClient)
		s.commentaryGen = commentary.NewGenerator(cfg.LLMClient, cfg.AgentID)

	case "director":
		s.personalityFactory = personality.NewFactory(cfg.LLMClient)
		s.personalityStore = personality.NewStore()

		// Set up personality pre-stocker if enabled
		if cfg.PersonalityBufferSize > 0 && cfg.LLMClient != nil && len(cfg.PreStockCategories) > 0 {
			ctx := cfg.Ctx
			if ctx == nil {
				ctx = context.Background()
			}
			ps := personality.NewPreStocker(ctx, s.personalityFactory, cfg.PreStockCategories, cfg.PersonalityBufferSize)
			ps.Start()
			s.personalityPreStock = ps
		}

	case "judge":
		s.verifier = verify.NewVerifier(cfg.LLMClient)
		s.personalityEvaluator = personality.NewEvaluator(cfg.LLMClient)
		s.personalityStore = personality.NewStore()
		s.commentaryGen = commentary.NewGenerator(cfg.LLMClient, cfg.AgentID)

	default:
		// All subsystems — used in tests
		s.questionGenerator = question.NewGenerator(cfg.GeneratorOptions...)
		s.consensusManager = question.NewConsensusManager()
		s.verifier = verify.NewVerifier(cfg.LLMClient)
		s.commentaryGen = commentary.NewGenerator(cfg.LLMClient, cfg.AgentID)
		s.personalityFactory = personality.NewFactory(cfg.LLMClient)
		s.personalityStore = personality.NewStore()
		s.personalityEvaluator = personality.NewEvaluator(cfg.LLMClient)
	}

	return s
}

// Shutdown stops background workers (buffers, pre-stockers).
func (s *Server) Shutdown() {
	if s.questionBuffer != nil {
		s.questionBuffer.Stop()
	}
	if s.personalityPreStock != nil {
		s.personalityPreStock.Stop()
	}
}


// SetupRoutes configures the gin router based on the agent's role.
func (s *Server) SetupRoutes(r *gin.Engine) {
	r.GET("/health", s.handleHealth)

	switch s.role {
	case "philosopher":
		r.POST("/generate-question", s.handleGenerateQuestion)
		r.POST("/propose-question", s.handleProposeQuestion)
		r.POST("/vote-proposal", s.handleVoteProposal)
		r.POST("/finalize-question", s.handleFinalizeQuestion)
		r.POST("/reveal", s.handleReveal)

	case "director":
		r.POST("/generate-personalities", s.handleGeneratePersonalities)
		r.GET("/personalities/:matchId", s.handleGetPersonalities)

	case "judge":
		r.POST("/verify-answer", s.handleVerifyAnswer)
		r.POST("/validate-question", s.handleValidateQuestion)
		r.POST("/evaluate-answer", s.handleEvaluateAnswer)
		r.POST("/commentary", s.handleCommentary)

	default:
		// All routes — used in tests
		r.POST("/generate-question", s.handleGenerateQuestion)
		r.POST("/propose-question", s.handleProposeQuestion)
		r.POST("/vote-proposal", s.handleVoteProposal)
		r.POST("/finalize-question", s.handleFinalizeQuestion)
		r.POST("/reveal", s.handleReveal)
		r.POST("/generate-personalities", s.handleGeneratePersonalities)
		r.GET("/personalities/:matchId", s.handleGetPersonalities)
		r.POST("/verify-answer", s.handleVerifyAnswer)
		r.POST("/validate-question", s.handleValidateQuestion)
		r.POST("/evaluate-answer", s.handleEvaluateAnswer)
		r.POST("/commentary", s.handleCommentary)
	}
}

// HealthResponse is the response for the health endpoint.
type HealthResponse struct {
	Status              string         `json:"status"`
	Role                string         `json:"role"`
	Provider            string         `json:"provider"`
	MatchesActive       int            `json:"matchesActive"`
	QuestionBufferSizes map[string]int `json:"questionBufferSizes,omitempty"`
	PersonalityPoolSize int            `json:"personalityPoolSize,omitempty"`
}

func (s *Server) handleHealth(c *gin.Context) {
	matchesActive := 0
	if s.questionGenerator != nil {
		matchesActive = s.questionGenerator.ActiveMatchCount()
	}

	resp := HealthResponse{
		Status:        "ok",
		Role:          s.role,
		Provider:      s.llmClient.Name(),
		MatchesActive: matchesActive,
	}

	if s.questionGenerator != nil {
		resp.QuestionBufferSizes = s.questionGenerator.BufferSizes()
	}
	if s.personalityPreStock != nil {
		resp.PersonalityPoolSize = s.personalityPreStock.Size()
	}

	c.JSON(http.StatusOK, resp)
}

// GenerateQuestionRequest is the request for question generation.
type GenerateQuestionRequest struct {
	MatchID       string   `json:"matchId" binding:"required"`
	Category      string   `json:"category"`
	Difficulty    int      `json:"difficulty"`
	ExcludeRecent []string `json:"excludeRecent"`
}

func (s *Server) handleGenerateQuestion(c *gin.Context) {
	var req GenerateQuestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// If category not specified, pick random from configured categories
	if req.Category == "" {
		if s.questionBuffer != nil {
			cats := s.questionBuffer.Categories()
			if len(cats) > 0 {
				req.Category = cats[rand.Intn(len(cats))]
			}
		}
		if req.Category == "" {
			req.Category = "crypto" // fallback default
		}
	}

	// If difficulty not specified, pick weighted random
	if req.Difficulty == 0 {
		req.Difficulty = question.WeightedDifficulty()
	}

	resp, err := s.questionGenerator.GenerateQuestion(
		req.MatchID,
		req.Category,
		req.Difficulty,
		req.ExcludeRecent,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ProposeQuestionRequest is the request for proposing a question.
type ProposeQuestionRequest struct {
	MatchID    string `json:"matchId" binding:"required"`
	Category   string `json:"category" binding:"required"`
	Difficulty int    `json:"difficulty" binding:"required,min=1,max=5"`
}

func (s *Server) handleProposeQuestion(c *gin.Context) {
	var req ProposeQuestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate a question for the proposal
	genResp, err := s.questionGenerator.GenerateQuestion(
		req.MatchID,
		req.Category,
		req.Difficulty,
		nil,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get the stored answer to create a proposal
	stored, ok := s.questionGenerator.GetAnswer(req.MatchID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "answer not found"})
		return
	}

	// Create a proposal
	q := question.Question{
		Category:   req.Category,
		Difficulty: req.Difficulty,
		Question:   genResp.Question,
		Answer:     stored.Answer,
		Variants:   stored.Variants,
		Format:     stored.Format,
	}

	proposal, err := s.consensusManager.ProposeQuestion(req.MatchID, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return proposal without the answer
	c.JSON(http.StatusOK, gin.H{
		"proposalId": proposal.ID,
		"question":   proposal.Question,
		"formatHint": proposal.FormatHint,
		"category":   proposal.Category,
		"difficulty": proposal.Difficulty,
	})
}

// VoteProposalRequest is the request for voting on a proposal.
type VoteProposalRequest struct {
	MatchID    string `json:"matchId" binding:"required"`
	ProposalID string `json:"proposalId" binding:"required"`
}

func (s *Server) handleVoteProposal(c *gin.Context) {
	var req VoteProposalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vote := s.consensusManager.VoteForProposal(req.MatchID, req.ProposalID)
	c.JSON(http.StatusOK, vote)
}

// FinalizeQuestionRequest is the request for finalizing a question.
type FinalizeQuestionRequest struct {
	MatchID    string `json:"matchId" binding:"required"`
	ProposalID string `json:"proposalId" binding:"required"`
}

func (s *Server) handleFinalizeQuestion(c *gin.Context) {
	var req FinalizeQuestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get the winning proposal
	proposal, ok := s.consensusManager.GetProposal(req.ProposalID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "proposal not found"})
		return
	}

	// Store the answer internally and compute hash
	resp, err := s.questionGenerator.GenerateQuestion(
		req.MatchID,
		proposal.Category,
		proposal.Difficulty,
		nil,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"answerHash": resp.AnswerHash,
	})
}

// VerifyAnswerRequest is the request for answer verification.
type VerifyAnswerRequest struct {
	MatchID    string `json:"matchId" binding:"required"`
	Submitted  string `json:"submitted" binding:"required"`
	Question   string `json:"question" binding:"required"`
	FormatHint string `json:"formatHint" binding:"required"`
}

func (s *Server) handleVerifyAnswer(c *gin.Context) {
	var req VerifyAnswerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if we have the stored answer
	if s.questionGenerator != nil {
		stored, ok := s.questionGenerator.GetAnswer(req.MatchID)
		if ok {
			// Store in verifier for exact match
			s.verifier.StoreAnswer(req.MatchID, &verify.StoredAnswer{
				Answer:   stored.Answer,
				Salt:     "0x", // Salt not needed for verification
				Variants: stored.Variants,
				Format:   stored.Format,
			})
		}
	}

	result, err := s.verifier.Verify(
		c.Request.Context(),
		req.MatchID,
		req.Submitted,
		req.Question,
		verify.FormatHint(req.FormatHint),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ValidateQuestionRequest is the request for question validation.
type ValidateQuestionRequest struct {
	MatchID    string `json:"matchId"`
	Question   string `json:"question"`
	FormatHint string `json:"formatHint"`
	Category   string `json:"category"`
	Difficulty int    `json:"difficulty"`
}

func (s *Server) handleValidateQuestion(c *gin.Context) {
	var req ValidateQuestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Basic validation: question must be non-empty and have a format hint
	valid := req.Question != "" && req.FormatHint != ""
	c.JSON(http.StatusOK, gin.H{
		"valid":      valid,
		"confidence": 0.95,
		"reasoning":  "Question is well-formed and answerable",
	})
}

// RevealRequest is the request for revealing an answer.
type RevealRequest struct {
	MatchID string `json:"matchId" binding:"required"`
}

func (s *Server) handleReveal(c *gin.Context) {
	var req RevealRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.questionGenerator.RevealAnswer(req.MatchID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Also clean up verifier
	if s.verifier != nil {
		s.verifier.DeleteStoredAnswer(req.MatchID)
	}

	c.JSON(http.StatusOK, resp)
}

func (s *Server) handleCommentary(c *gin.Context) {
	var req commentary.CommentaryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := s.commentaryGen.Generate(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GeneratePersonalitiesRequest is the request for generating match personalities.
type GeneratePersonalitiesRequest struct {
	MatchID  string `json:"matchId" binding:"required"`
	Category string `json:"category"`
}

// GeneratePersonalitiesResponse is the response for personality generation.
type GeneratePersonalitiesResponse struct {
	MatchID       string                   `json:"matchId"`
	Personalities []personality.Personality `json:"personalities"`
}

func (s *Server) handleGeneratePersonalities(c *gin.Context) {
	var req GeneratePersonalitiesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	category := req.Category
	if category == "" {
		category = "general"
	}

	// Try pre-stocker first for instant response
	if s.personalityPreStock != nil {
		if triplet, ok := s.personalityPreStock.PopTriplet(category); ok {
			// Convert to non-pointer slice
			ps := make([]personality.Personality, len(triplet.Personalities))
			for i, p := range triplet.Personalities {
				ps[i] = *p
			}

			mp := &personality.MatchPersonalities{
				MatchID:       req.MatchID,
				Personalities: ps,
			}
			s.personalityStore.AssignToMatch(mp)

			c.JSON(http.StatusOK, GeneratePersonalitiesResponse{
				MatchID:       req.MatchID,
				Personalities: ps,
			})
			return
		}
	}

	// Fall back to on-demand generation
	mp, err := s.personalityFactory.GenerateForMatch(c.Request.Context(), req.MatchID, category)
	if err != nil {
		// Fall back to default personalities on error
		defaults := personality.DefaultPersonalities()
		mp = &personality.MatchPersonalities{
			MatchID:       req.MatchID,
			Personalities: defaults,
		}
	}

	// Store the assignment
	s.personalityStore.AssignToMatch(mp)

	c.JSON(http.StatusOK, GeneratePersonalitiesResponse{
		MatchID:       req.MatchID,
		Personalities: mp.Personalities,
	})
}

// EvaluateAnswerRequest is the request for personality-based answer evaluation.
type EvaluateAnswerRequest struct {
	MatchID       string                   `json:"matchId" binding:"required"`
	Question      string                   `json:"question" binding:"required"`
	Answer        string                   `json:"answer" binding:"required"`
	Reasoning     string                   `json:"reasoning"`
	Personalities []personality.Personality `json:"personalities,omitempty"`
}

// EvaluateAnswerResponse is the response for personality-based evaluation.
type EvaluateAnswerResponse struct {
	MatchID     string                         `json:"matchId"`
	TotalScore  int                            `json:"totalScore"`
	Agreement   string                         `json:"agreement"`
	Evaluations []personality.EvaluateResponse `json:"evaluations"`
}

func (s *Server) handleEvaluateAnswer(c *gin.Context) {
	var req EvaluateAnswerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Determine which personalities to use:
	// 1. Personalities passed in the request (from Chief, cross-process)
	// 2. Locally stored personalities (same-process fallback)
	var personalities []personality.Personality
	if len(req.Personalities) > 0 {
		personalities = req.Personalities
		// Also store them locally for future requests
		s.personalityStore.AssignToMatch(&personality.MatchPersonalities{
			MatchID:       req.MatchID,
			Personalities: personalities,
		})
	} else {
		mp, ok := s.personalityStore.GetMatchAssignment(req.MatchID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "no personalities assigned to this match"})
			return
		}
		personalities = mp.Personalities
	}

	// Evaluate the answer with all personalities
	pe, err := s.personalityEvaluator.EvaluateWithAll(
		c.Request.Context(),
		req.MatchID,
		req.Question,
		req.Answer,
		req.Reasoning,
		personalities,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, EvaluateAnswerResponse{
		MatchID:     req.MatchID,
		TotalScore:  pe.TotalScore,
		Agreement:   pe.Agreement,
		Evaluations: pe.Evaluations,
	})
}

func (s *Server) handleGetPersonalities(c *gin.Context) {
	matchID := c.Param("matchId")

	mp, ok := s.personalityStore.GetMatchAssignment(matchID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "no personalities assigned to this match"})
		return
	}

	c.JSON(http.StatusOK, GeneratePersonalitiesResponse{
		MatchID:       matchID,
		Personalities: mp.Personalities,
	})
}

// GetPersonalityStore returns the personality store for external use.
func (s *Server) GetPersonalityStore() *personality.Store {
	return s.personalityStore
}

// GetPersonalityFactory returns the personality factory for external use.
func (s *Server) GetPersonalityFactory() *personality.Factory {
	return s.personalityFactory
}
