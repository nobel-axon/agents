// Package bounty provides in-memory tracking of active bounties for the chief service.
package bounty

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/axon-arena/axon-chief/internal/match"
)

// Phase represents the lifecycle phase of a bounty.
type Phase int

const (
	PhasePending Phase = iota
	PhaseActive
	PhaseSettled
)

func (p Phase) String() string {
	switch p {
	case PhasePending:
		return "pending"
	case PhaseActive:
		return "active"
	case PhaseSettled:
		return "settled"
	default:
		return "unknown"
	}
}

// BountyState holds the in-memory state of a single bounty.
type BountyState struct {
	mu sync.RWMutex

	ID              *big.Int
	Creator         common.Address
	Question        string
	Category        string
	Difficulty      uint8
	EntryFee        *big.Int
	Pool            *big.Int
	Deadline        time.Time
	MaxParticipants uint8
	MinRating       *big.Int
	Phase           Phase
	PlayerCount     int
	Players         []common.Address
	Winner          common.Address

	// Wallet → ERC-8004 agentId mapping (populated on agent join)
	AgentIDs map[string]*big.Int

	// Judge evaluation state
	Personalities []match.Personality
	JudgePanel    []int
	Evaluations   map[string]*match.ParticipantEvaluation
}

// NewBountyState creates a new bounty state.
func NewBountyState(id *big.Int, creator common.Address, question, category string, difficulty uint8, entryFee *big.Int, deadline time.Time, maxParticipants uint8, minRating *big.Int) *BountyState {
	return &BountyState{
		ID:              id,
		Creator:         creator,
		Question:        question,
		Category:        category,
		Difficulty:      difficulty,
		EntryFee:        entryFee,
		Pool:            big.NewInt(0),
		Deadline:        deadline,
		MaxParticipants: maxParticipants,
		MinRating:       minRating,
		Phase:           PhasePending,
		Players:         make([]common.Address, 0),
		AgentIDs:        make(map[string]*big.Int),
		Evaluations:     make(map[string]*match.ParticipantEvaluation),
	}
}

// AddPlayer adds a player to the bounty.
func (b *BountyState) AddPlayer(addr common.Address) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Players = append(b.Players, addr)
	b.PlayerCount = len(b.Players)
}

// SetAgentID maps a wallet address to its ERC-8004 agentId.
func (b *BountyState) SetAgentID(wallet string, agentID *big.Int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.AgentIDs[wallet] = agentID
}

// GetAgentID returns the ERC-8004 agentId for a wallet address.
func (b *BountyState) GetAgentID(wallet string) (*big.Int, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	id, ok := b.AgentIDs[wallet]
	return id, ok
}

// SetPersonalities stores the judge personalities for this bounty.
func (b *BountyState) SetPersonalities(personalities []match.Personality) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Personalities = personalities
}

// GetPersonalities returns the judge personalities for this bounty.
func (b *BountyState) GetPersonalities() []match.Personality {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Personalities
}

// SetJudgePanel stores the judge panel indices.
func (b *BountyState) SetJudgePanel(panel []int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.JudgePanel = panel
}

// GetJudgePanel returns the judge panel indices.
func (b *BountyState) GetJudgePanel() []int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.JudgePanel
}

// AddEvaluation stores an evaluation for a participant.
func (b *BountyState) AddEvaluation(participant string, eval *match.ParticipantEvaluation) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Evaluations[participant] = eval
}

// GetAllEvaluations returns all evaluations for the bounty.
func (b *BountyState) GetAllEvaluations() map[string]*match.ParticipantEvaluation {
	b.mu.RLock()
	defer b.mu.RUnlock()
	// Return a copy
	result := make(map[string]*match.ParticipantEvaluation, len(b.Evaluations))
	for k, v := range b.Evaluations {
		result[k] = v
	}
	return result
}

// Manager manages active bounties in memory.
type Manager struct {
	mu      sync.RWMutex
	bounties map[string]*BountyState // bountyID string -> state
}

// NewManager creates a new bounty manager.
func NewManager() *Manager {
	return &Manager{
		bounties: make(map[string]*BountyState),
	}
}

// CreateBounty registers a new bounty in memory.
func (m *Manager) CreateBounty(state *BountyState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := state.ID.String()
	if _, exists := m.bounties[key]; exists {
		return fmt.Errorf("bounty %s already exists", key)
	}
	m.bounties[key] = state
	return nil
}

// GetBounty returns the state of a bounty.
func (m *Manager) GetBounty(bountyID *big.Int) (*BountyState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.bounties[bountyID.String()]
	if !ok {
		return nil, fmt.Errorf("bounty %s not found", bountyID)
	}
	return state, nil
}

// RemoveBounty removes a bounty from tracking.
func (m *Manager) RemoveBounty(bountyID *big.Int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.bounties, bountyID.String())
}

// GetActiveBounties returns all non-terminal bounties (Pending or Active).
func (m *Manager) GetActiveBounties() []*BountyState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var active []*BountyState
	for _, b := range m.bounties {
		if b.Phase == PhasePending || b.Phase == PhaseActive {
			active = append(active, b)
		}
	}
	return active
}
