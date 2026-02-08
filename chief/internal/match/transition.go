// Package match provides match state management for axon-chief.
package match

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidTransition is returned when an invalid phase transition is attempted.
	ErrInvalidTransition = errors.New("invalid phase transition")
)

// ValidTransitions defines the allowed state transitions for matches.
// Key is the current phase, value is a slice of valid next phases.
var ValidTransitions = map[Phase][]Phase{
	PhaseNone:         {PhaseQueue},
	PhaseQueue:        {PhaseActive, PhaseCancelled, PhaseRefunded},
	PhaseActive:       {PhaseAnswerPeriod, PhaseRefunded},
	PhaseAnswerPeriod: {PhaseSettled, PhaseRefunded},
	// Terminal states - no transitions allowed
	PhaseSettled:   {},
	PhaseRefunded:  {},
	PhaseCancelled: {},
}

// CanTransition checks if a transition from one phase to another is valid.
func CanTransition(from, to Phase) bool {
	validTargets, ok := ValidTransitions[from]
	if !ok {
		return false
	}
	for _, target := range validTargets {
		if target == to {
			return true
		}
	}
	return false
}

// ValidateTransition validates a phase transition and returns an error if invalid.
func ValidateTransition(from, to Phase) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, from.String(), to.String())
	}
	return nil
}

// IsTerminalPhase checks if a phase is a terminal state (no further transitions possible).
func IsTerminalPhase(phase Phase) bool {
	validTargets, ok := ValidTransitions[phase]
	if !ok {
		return true
	}
	return len(validTargets) == 0
}

// GetValidTransitions returns the list of valid next phases from the current phase.
func GetValidTransitions(from Phase) []Phase {
	if targets, ok := ValidTransitions[from]; ok {
		result := make([]Phase, len(targets))
		copy(result, targets)
		return result
	}
	return nil
}
