package question

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// Proposal represents a question proposal for consensus voting.
type Proposal struct {
	ID         string `json:"proposalId"`
	Question   string `json:"question"`
	FormatHint string `json:"formatHint"`
	Category   string `json:"category"`
	Difficulty int    `json:"difficulty"`
	Answer     string `json:"-"` // Not exposed in API
	Variants   []string `json:"-"`
}

// Vote represents a vote on a proposal.
type Vote struct {
	ProposalID string `json:"votedFor"`
}

// ConsensusManager handles proposal and voting for question consensus.
type ConsensusManager struct {
	proposals map[string]*Proposal // proposalId -> proposal
	votes     map[string][]Vote    // matchId -> votes
	mu        sync.RWMutex
}

// NewConsensusManager creates a new consensus manager.
func NewConsensusManager() *ConsensusManager {
	return &ConsensusManager{
		proposals: make(map[string]*Proposal),
		votes:     make(map[string][]Vote),
	}
}

// ProposeQuestion creates a new proposal for a question.
func (cm *ConsensusManager) ProposeQuestion(matchID string, q Question) (*Proposal, error) {
	// Generate proposal ID
	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, err
	}
	proposalID := hex.EncodeToString(idBytes)

	proposal := &Proposal{
		ID:         proposalID,
		Question:   q.Question,
		FormatHint: q.Format,
		Category:   q.Category,
		Difficulty: q.Difficulty,
		Answer:     q.Answer,
		Variants:   q.Variants,
	}

	cm.mu.Lock()
	cm.proposals[proposalID] = proposal
	cm.mu.Unlock()

	return proposal, nil
}

// VoteForProposal records a vote for a proposal.
func (cm *ConsensusManager) VoteForProposal(matchID, proposalID string) *Vote {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	vote := Vote{ProposalID: proposalID}
	cm.votes[matchID] = append(cm.votes[matchID], vote)
	return &vote
}

// GetProposal retrieves a proposal by ID.
func (cm *ConsensusManager) GetProposal(proposalID string) (*Proposal, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	p, ok := cm.proposals[proposalID]
	return p, ok
}

// GetVotes retrieves votes for a match.
func (cm *ConsensusManager) GetVotes(matchID string) []Vote {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.votes[matchID]
}

// ClearMatch clears proposals and votes for a match.
func (cm *ConsensusManager) ClearMatch(matchID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.votes, matchID)
	// Note: proposals are kept until explicitly cleared
}
