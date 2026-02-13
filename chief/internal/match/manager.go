// Package match provides match state management for axon-chief.
package match

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrMatchNotFound     = errors.New("match not found")
	ErrMatchAlreadyExists = errors.New("match already exists")
	ErrMatchLocked       = errors.New("match is locked")
	ErrInvalidPhase      = errors.New("invalid phase for operation")
)

// State represents the in-memory state of a match.
type State struct {
	mu sync.RWMutex

	ID             *big.Int
	Phase          Phase
	Pool           *big.Int
	Players        []common.Address
	PlayerCount    int
	QueueDeadline  time.Time
	AnswerDeadline time.Time
	Question       *QuestionData
	Winner         common.Address
	CreatedAt      time.Time
	UpdatedAt      time.Time

	// Internal state
	locked bool

	// Registration grace period
	GraceTimerStarted bool         // Whether the grace timer has been started (minPlayers reached)
	GraceDone         chan struct{} // Closed when maxPlayers reached during grace to trigger early start

	// Wallet → ERC-8004 agentId mapping (populated on agent join)
	AgentIDs map[string]*big.Int

	// Personality-based judging
	Personalities []Personality                        // 3 unique judge personalities for this match
	JudgePanel    []int                                // assigned judge indices for this match
	Evaluations   map[string]*ParticipantEvaluation   // participant address -> evaluation results
}

// Manager manages all active matches.
type Manager struct {
	mu       sync.RWMutex
	matches  map[string]*State // matchID -> state
	config   *Config
	lastCreate time.Time
}

// NewManager creates a new match manager.
func NewManager(cfg *Config) *Manager {
	return &Manager{
		matches: make(map[string]*State),
		config:  cfg,
	}
}

// GetConfig returns the match configuration.
func (m *Manager) GetConfig() *Config {
	return m.config
}

// SetConfig updates the match configuration.
func (m *Manager) SetConfig(cfg *Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = cfg
}

// CreateMatch creates a new match state.
func (m *Manager) CreateMatch(id *big.Int, queueDeadline time.Time) (*State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := id.String()
	if _, exists := m.matches[key]; exists {
		return nil, ErrMatchAlreadyExists
	}

	state := &State{
		ID:            id,
		Phase:         PhaseQueue,
		Pool:          big.NewInt(0),
		Players:       make([]common.Address, 0),
		QueueDeadline: queueDeadline,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		AgentIDs:      make(map[string]*big.Int),
		Evaluations:   make(map[string]*ParticipantEvaluation),
		GraceDone:     make(chan struct{}),
	}

	m.matches[key] = state
	m.lastCreate = time.Now()
	return state, nil
}

// GetMatch returns a match state by ID.
func (m *Manager) GetMatch(id *big.Int) (*State, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.matches[id.String()]
	if !ok {
		return nil, ErrMatchNotFound
	}
	return state, nil
}

// GetActiveMatches returns all active matches (not in terminal state).
func (m *Manager) GetActiveMatches() []*State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var active []*State
	for _, state := range m.matches {
		state.mu.RLock()
		phase := state.Phase
		state.mu.RUnlock()

		if phase != PhaseSettled && phase != PhaseRefunded && phase != PhaseCancelled {
			active = append(active, state)
		}
	}
	return active
}

// GetMatchesInPhase returns all matches in a specific phase.
func (m *Manager) GetMatchesInPhase(phase Phase) []*State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matches []*State
	for _, state := range m.matches {
		state.mu.RLock()
		if state.Phase == phase {
			matches = append(matches, state)
		}
		state.mu.RUnlock()
	}
	return matches
}

// RemoveMatch removes a match from memory.
func (m *Manager) RemoveMatch(id *big.Int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.matches, id.String())
}

// TransitionPhase validates and applies a phase transition for a match.
// Returns ErrMatchNotFound if the match doesn't exist.
// Returns ErrInvalidTransition if the transition is not allowed.
func (m *Manager) TransitionPhase(id *big.Int, to Phase, trigger string) error {
	state, err := m.GetMatch(id)
	if err != nil {
		return err
	}
	return state.TransitionPhase(to, trigger)
}

// CanCreateNewMatch checks if a new match can be created.
// Single-match mode: blocks creation if any match is in a non-terminal phase.
func (m *Manager) CanCreateNewMatch() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.config.AutoCreate {
		return false
	}

	// Single-match mode: block if any active match exists
	for _, state := range m.matches {
		state.mu.RLock()
		phase := state.Phase
		state.mu.RUnlock()
		if phase != PhaseSettled && phase != PhaseRefunded && phase != PhaseCancelled {
			return false
		}
	}

	return time.Since(m.lastCreate) >= m.config.Cooldown
}

// State methods

// Lock attempts to lock the match for exclusive access.
// Returns error if already locked.
func (s *State) Lock() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.locked {
		return ErrMatchLocked
	}
	s.locked = true
	return nil
}

// Unlock releases the match lock.
func (s *State) Unlock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.locked = false
}

// IsLocked returns whether the match is locked.
func (s *State) IsLocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.locked
}

// SetPhase sets the match phase without transition validation.
// Prefer TransitionPhase() for validated phase changes.
func (s *State) SetPhase(phase Phase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Phase = phase
	s.UpdatedAt = time.Now()
}

// TransitionPhase validates and applies a phase transition.
// Returns ErrInvalidTransition if the transition is not allowed.
// The trigger parameter describes what caused the transition (for logging).
func (s *State) TransitionPhase(to Phase, trigger string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	from := s.Phase
	if err := ValidateTransition(from, to); err != nil {
		return err
	}

	s.Phase = to
	s.UpdatedAt = time.Now()

	// Log the transition (import log is already available via time package context)
	return nil
}

// GetPhase returns the match phase.
func (s *State) GetPhase() Phase {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Phase
}

// AddPlayer adds a player to the match.
func (s *State) AddPlayer(player common.Address, fee *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Players = append(s.Players, player)
	s.PlayerCount = len(s.Players)
	s.Pool = new(big.Int).Add(s.Pool, fee)
	s.UpdatedAt = time.Now()
}

// SetQuestion sets the question for the match.
func (s *State) SetQuestion(q *QuestionData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Question = q
	s.UpdatedAt = time.Now()
}

// GetQuestion returns the question for the match.
func (s *State) GetQuestion() *QuestionData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Question
}

// SetAnswerDeadline sets the answer deadline.
func (s *State) SetAnswerDeadline(deadline time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AnswerDeadline = deadline
	s.UpdatedAt = time.Now()
}

// GetAnswerDeadline returns the answer deadline.
func (s *State) GetAnswerDeadline() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.AnswerDeadline
}

// SetWinner sets the winner of the match.
func (s *State) SetWinner(winner common.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Winner = winner
	s.UpdatedAt = time.Now()
}

// GetWinner returns the winner of the match.
func (s *State) GetWinner() common.Address {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Winner
}

// SetQueueDeadline sets the queue deadline for recovery.
func (s *State) SetQueueDeadline(deadline time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.QueueDeadline = deadline
}

// SyncFromChain updates state from chain data.
func (s *State) SyncFromChain(pool *big.Int, phase Phase, answerDeadline uint64, winner common.Address, players []common.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Pool = pool
	s.Phase = phase
	s.Players = players
	s.PlayerCount = len(players)
	if answerDeadline > 0 {
		s.AnswerDeadline = time.Unix(int64(answerDeadline), 0)
	}
	if winner != (common.Address{}) {
		s.Winner = winner
	}
	s.UpdatedAt = time.Now()
}

// RecoverActiveMatches recovers active match states from the chain.
func (m *Manager) RecoverActiveMatches(ctx context.Context, getMatchState func(ctx context.Context, id *big.Int) (Phase, *big.Int, uint64, common.Address, []common.Address, error), nextMatchID *big.Int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Iterate through all matches up to nextMatchID
	for i := int64(1); i < nextMatchID.Int64(); i++ {
		matchID := big.NewInt(i)
		phase, pool, answerDeadline, winner, players, err := getMatchState(ctx, matchID)
		if err != nil {
			continue // Skip invalid matches
		}

		// Only recover active matches
		if phase == PhaseSettled || phase == PhaseRefunded || phase == PhaseCancelled || phase == PhaseNone {
			continue
		}

		state := &State{
			ID:            matchID,
			Phase:         phase,
			Pool:          pool,
			Players:       players,
			PlayerCount:   len(players),
			Winner:        winner,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			AgentIDs:      make(map[string]*big.Int),
			Evaluations:   make(map[string]*ParticipantEvaluation),
		}

		if answerDeadline > 0 {
			state.AnswerDeadline = time.Unix(int64(answerDeadline), 0)
		}

		m.matches[matchID.String()] = state
	}

	return nil
}

// SetAgentID maps a wallet address to its ERC-8004 agentId.
func (s *State) SetAgentID(wallet string, agentID *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.AgentIDs == nil {
		s.AgentIDs = make(map[string]*big.Int)
	}
	s.AgentIDs[wallet] = agentID
}

// GetAgentID returns the ERC-8004 agentId for a wallet address.
func (s *State) GetAgentID(wallet string) (*big.Int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.AgentIDs == nil {
		return nil, false
	}
	id, ok := s.AgentIDs[wallet]
	return id, ok
}

// SetPersonalities sets the judge personalities for this match.
func (s *State) SetPersonalities(personalities []Personality) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Personalities = personalities
	s.UpdatedAt = time.Now()
}

// GetPersonalities returns the judge personalities for this match.
func (s *State) GetPersonalities() []Personality {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Personalities
}

// HasPersonalities returns true if personalities have been assigned.
func (s *State) HasPersonalities() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Personalities) > 0
}

// SetJudgePanel sets the judge panel indices for this match.
func (s *State) SetJudgePanel(panel []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.JudgePanel = panel
	s.UpdatedAt = time.Now()
}

// GetJudgePanel returns the judge panel indices for this match.
func (s *State) GetJudgePanel() []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.JudgePanel
}

// HasJudgePanel returns true if a judge panel has been assigned.
func (s *State) HasJudgePanel() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.JudgePanel) > 0
}

// AddEvaluation adds or updates an evaluation for a participant.
func (s *State) AddEvaluation(participant string, eval *ParticipantEvaluation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Evaluations == nil {
		s.Evaluations = make(map[string]*ParticipantEvaluation)
	}
	s.Evaluations[participant] = eval
	s.UpdatedAt = time.Now()
}

// GetEvaluation returns the evaluation for a participant.
func (s *State) GetEvaluation(participant string) (*ParticipantEvaluation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Evaluations == nil {
		return nil, false
	}
	eval, ok := s.Evaluations[participant]
	return eval, ok
}

// GetAllEvaluations returns all evaluations for this match.
func (s *State) GetAllEvaluations() map[string]*ParticipantEvaluation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Evaluations == nil {
		return make(map[string]*ParticipantEvaluation)
	}
	result := make(map[string]*ParticipantEvaluation, len(s.Evaluations))
	for k, v := range s.Evaluations {
		result[k] = v
	}
	return result
}

// DetermineWinnerByScore finds the participant with the highest total score.
// Returns the winner address and their evaluation, or empty if no evaluations.
func (s *State) DetermineWinnerByScore() (string, *ParticipantEvaluation) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Evaluations == nil || len(s.Evaluations) == 0 {
		return "", nil
	}

	var highestScore int
	var winner string
	var winnerEval *ParticipantEvaluation

	for participant, eval := range s.Evaluations {
		if eval.TotalScore > highestScore ||
			(eval.TotalScore == highestScore && winnerEval != nil && eval.SubmittedAt.Before(winnerEval.SubmittedAt)) {
			highestScore = eval.TotalScore
			winner = participant
			winnerEval = eval
		}
	}

	return winner, winnerEval
}
