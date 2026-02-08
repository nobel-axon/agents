package personality

import (
	"sync"
)

// Store provides in-memory storage for personalities.
// For production, this should be replaced with a database-backed implementation.
type Store struct {
	mu            sync.RWMutex
	personalities map[string]*Personality       // id -> personality
	matchAssignments map[string]*MatchPersonalities // matchId -> assigned personalities
	pool          []*Personality                // Pre-generated personality pool
}

// NewStore creates a new personality store.
func NewStore() *Store {
	return &Store{
		personalities:    make(map[string]*Personality),
		matchAssignments: make(map[string]*MatchPersonalities),
		pool:             make([]*Personality, 0),
	}
}

// Save stores a personality.
func (s *Store) Save(p *Personality) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.personalities[p.ID] = p
}

// Get retrieves a personality by ID.
func (s *Store) Get(id string) (*Personality, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.personalities[id]
	return p, ok
}

// Delete removes a personality.
func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.personalities, id)
}

// List returns all stored personalities.
func (s *Store) List() []*Personality {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Personality, 0, len(s.personalities))
	for _, p := range s.personalities {
		result = append(result, p)
	}
	return result
}

// AddToPool adds personalities to the pre-generated pool.
func (s *Store) AddToPool(personalities []*Personality) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pool = append(s.pool, personalities...)
}

// TakeFromPool takes up to n personalities from the pool.
// Returns fewer if the pool doesn't have enough.
func (s *Store) TakeFromPool(n int) []*Personality {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pool) == 0 {
		return nil
	}

	if n > len(s.pool) {
		n = len(s.pool)
	}

	taken := s.pool[:n]
	s.pool = s.pool[n:]
	return taken
}

// PoolSize returns the current pool size.
func (s *Store) PoolSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.pool)
}

// AssignToMatch stores the personality assignment for a match.
func (s *Store) AssignToMatch(mp *MatchPersonalities) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.matchAssignments[mp.MatchID] = mp
}

// GetMatchAssignment retrieves the personality assignment for a match.
func (s *Store) GetMatchAssignment(matchID string) (*MatchPersonalities, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	mp, ok := s.matchAssignments[matchID]
	return mp, ok
}

// ClearMatchAssignment removes the personality assignment for a match.
func (s *Store) ClearMatchAssignment(matchID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.matchAssignments, matchID)
}

// GetAllAssignments returns all current match assignments.
func (s *Store) GetAllAssignments() map[string]*MatchPersonalities {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*MatchPersonalities, len(s.matchAssignments))
	for k, v := range s.matchAssignments {
		result[k] = v
	}
	return result
}
