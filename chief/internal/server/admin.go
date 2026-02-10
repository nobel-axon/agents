// Package server provides HTTP handlers for the axon-chief API.
package server

import (
	"context"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"

	"github.com/axon-arena/axon-chief/internal/match"
)

// adminAuth is middleware for admin endpoints.
func (s *Server) adminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.cfg.AdminToken == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin authentication not configured"})
			c.Abort()
			return
		}

		token := c.GetHeader("X-Admin-Token")
		if token != s.cfg.AdminToken {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid admin token"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// PauseResponse is the response for pause/resume endpoints.
type PauseResponse struct {
	Paused bool `json:"paused"`
}

func (s *Server) handleAdminPause(c *gin.Context) {
	s.SetPaused(true)
	c.JSON(http.StatusOK, PauseResponse{Paused: true})
}

func (s *Server) handleAdminResume(c *gin.Context) {
	s.SetPaused(false)
	c.JSON(http.StatusOK, PauseResponse{Paused: false})
}

// AdminSettleRequest is the request for manual settlement.
type AdminSettleRequest struct {
	Winner string `json:"winner" binding:"required"`
}

func (s *Server) handleAdminSettle(c *gin.Context) {
	matchIDStr := c.Param("matchId")
	matchID, err := strconv.ParseUint(matchIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid match ID"})
		return
	}

	var req AdminSettleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	winner := common.HexToAddress(req.Winner)
	if winner == (common.Address{}) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid winner address"})
		return
	}

	bigMatchID := new(big.Int).SetUint64(matchID)

	// Get match state first
	state, err := s.matchManager.GetMatch(bigMatchID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "match not found"})
		return
	}

	// Acquire lock to prevent race with timeout watcher
	if err := state.Lock(); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "match is being processed"})
		return
	}
	defer state.Unlock()

	// Validate phase - cannot settle terminal matches
	phase := state.GetPhase()
	if match.IsTerminalPhase(phase) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "match already in terminal state", "phase": phase.String()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()

	// Cancel timeout
	s.timeoutWatcher.CancelMatch(bigMatchID)

	// Settle on chain
	if _, err := s.chainClient.SettleWinner(ctx, bigMatchID, winner); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update match state
	state.SetPhase(match.PhaseSettled)
	state.SetWinner(winner)

	// Reveal answer (same pattern as onSettleMatch)
	q := state.GetQuestion()
	if q != nil && q.Answer != "" {
		if err := s.chainClient.RevealAnswer(ctx, bigMatchID, q.Answer, q.Salt); err != nil {
			log.Printf("Warning: failed to reveal answer for match %s: %v", bigMatchID, err)
		}
	} else {
		// Get answer from philosopher
		reveal, err := s.judgeCoordinator.RequestReveal(ctx, bigMatchID.String())
		if err != nil {
			log.Printf("Warning: failed to get reveal from philosopher: %v", err)
		} else {
			var salt [32]byte
			copy(salt[:], common.FromHex(reveal.Salt))
			if err := s.chainClient.RevealAnswer(ctx, bigMatchID, reveal.Answer, salt); err != nil {
				log.Printf("Warning: failed to reveal answer for match %s: %v", bigMatchID, err)
			}
		}
	}

	// Clean up
	s.verificationMgr.RemoveQueue(bigMatchID)

	c.JSON(http.StatusOK, gin.H{
		"message": "match settled",
		"matchId": matchID,
		"winner":  winner.Hex(),
	})
}

func (s *Server) handleAdminRefund(c *gin.Context) {
	matchIDStr := c.Param("matchId")
	matchID, err := strconv.ParseUint(matchIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid match ID"})
		return
	}

	bigMatchID := new(big.Int).SetUint64(matchID)

	// Get match state first
	state, err := s.matchManager.GetMatch(bigMatchID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "match not found"})
		return
	}

	// Acquire lock to prevent race with timeout watcher
	if err := state.Lock(); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "match is being processed"})
		return
	}
	defer state.Unlock()

	// Validate phase - cannot refund terminal matches
	phase := state.GetPhase()
	if match.IsTerminalPhase(phase) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "match already in terminal state", "phase": phase.String()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()

	// Cancel timeout
	s.timeoutWatcher.CancelMatch(bigMatchID)

	// Refund on chain
	if err := s.chainClient.RefundMatch(ctx, bigMatchID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update match state
	state.SetPhase(match.PhaseRefunded)

	// Clean up
	s.verificationMgr.RemoveQueue(bigMatchID)

	c.JSON(http.StatusOK, gin.H{
		"message": "match refunded",
		"matchId": matchID,
	})
}

// AdminCreateMatchRequest is the request for creating a match.
type AdminCreateMatchRequest struct {
	EntryFee       string `json:"entryFee"`       // Wei
	BaseAnswerFee  string `json:"baseAnswerFee"`  // NEURON wei
	QueueDuration  uint64 `json:"queueDuration"`  // Seconds
	AnswerDuration uint64 `json:"answerDuration"` // Seconds
	MinPlayers     uint8  `json:"minPlayers"`
	MaxPlayers     uint8  `json:"maxPlayers"`
}

func (s *Server) handleAdminCreateMatch(c *gin.Context) {
	var req AdminCreateMatchRequest

	// Only bind if there's a body
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()

	var entryFee *big.Int
	if req.EntryFee != "" {
		var ok bool
		entryFee, ok = new(big.Int).SetString(req.EntryFee, 10)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entryFee: must be a valid integer"})
			return
		}
	} else {
		entryFee = s.matchManager.GetConfig().EntryFee
	}

	var baseAnswerFee *big.Int
	if req.BaseAnswerFee != "" {
		var ok bool
		baseAnswerFee, ok = new(big.Int).SetString(req.BaseAnswerFee, 10)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid baseAnswerFee: must be a valid integer"})
			return
		}
	} else {
		baseAnswerFee = s.matchManager.GetConfig().BaseAnswerFee
	}

	// Apply defaults
	if req.QueueDuration == 0 {
		req.QueueDuration = uint64(s.cfg.Match.QueueDuration.Seconds())
	}
	if req.AnswerDuration == 0 {
		req.AnswerDuration = uint64(s.cfg.Match.AnswerDuration.Seconds())
	}
	if req.MinPlayers == 0 {
		req.MinPlayers = s.cfg.Match.MinPlayers
	}
	if req.MaxPlayers == 0 {
		req.MaxPlayers = s.cfg.Match.MaxPlayers
	}

	// Create match on chain
	matchID, err := s.chainClient.CreateMatch(
		ctx,
		entryFee,
		baseAnswerFee,
		req.QueueDuration,
		req.AnswerDuration,
		req.MinPlayers,
		req.MaxPlayers,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Calculate queue deadline
	queueDeadline := time.Now().Add(time.Duration(req.QueueDuration) * time.Second)

	// Track in memory
	s.matchManager.CreateMatch(matchID, queueDeadline)

	c.JSON(http.StatusOK, gin.H{
		"message":       "match created",
		"matchId":       matchID.String(),
		"entryFee":      entryFee.String(),
		"baseAnswerFee": baseAnswerFee.String(),
		"queueDeadline": queueDeadline.Format(time.RFC3339),
	})
}
